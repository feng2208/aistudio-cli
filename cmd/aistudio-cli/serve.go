package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"aistudio-cli/internal/aistudio"
	"aistudio-cli/internal/api"
	"aistudio-cli/internal/camoufoxnative"
)

// handleServeCommand 启动兼容 OpenAI Chat Completions 的 HTTP 服务
func handleServeCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	var authDirs stringSlice
	var listen string
	var apiKey string
	var globalProxy string
	var camoufoxPath string
	var temporaryChat bool
	var prewarm bool
	var verbose bool
	initTimeout := timeDuration{Duration: 2 * time.Minute}
	startupTimeout := timeDuration{Duration: 2 * time.Minute}
	requestTimeout := timeDuration{Duration: 30 * time.Minute}

	fs.Var(&authDirs, "auth-dir", "账户目录，可重复或逗号分隔 (默认 auth)")
	fs.StringVar(&listen, "listen", "127.0.0.1:2048", "HTTP 监听地址")
	fs.StringVar(&apiKey, "api-key", "", "可选 API key；设置后请求需携带 Authorization: Bearer <key>")
	fs.StringVar(&globalProxy, "proxy", "", "全局 HTTP/SOCKS5 代理")
	fs.StringVar(&camoufoxPath, "camoufox-path", "", "指定本地 Camoufox 可执行文件完整路径 (若不指定则自动定位或下载)")
	fs.Var(&initTimeout, "init-timeout", "Camoufox WAA 初始化超时 (默认 2m)")
	fs.Var(&startupTimeout, "startup-timeout", "启动时同步模型目录超时 (默认 2m)")
	fs.Var(&requestTimeout, "request-timeout", "单次公开 API 请求超时 (默认 30m，0 表示不限制)")
	fs.BoolVar(&temporaryChat, "temporary-chat", false, "WAA 是否使用临时对话模式")
	fs.BoolVar(&prewarm, "prewarm", false, "启动后在后台预热一个 Camoufox WAA worker")
	fs.BoolVar(&verbose, "verbose", false, "输出调试日志到 stderr")

	if err := fs.Parse(args); err != nil {
		return err
	}

	setupLogLevel(verbose)

	if len(authDirs) == 0 {
		authDirs = []string{"auth"}
	}
	if strings.TrimSpace(camoufoxPath) != "" {
		_ = os.Setenv("CAMOUFOX_PATH", camoufoxPath)
	}

	slog.Info("正在查找或准备 Camoufox 运行环境...")
	realCamoufoxPath, err := camoufoxnative.FindExecutable(ctx, globalProxy)
	if err != nil {
		return fmt.Errorf("准备 Camoufox 浏览器环境失败: %w", err)
	}
	slog.Info("Camoufox 环境就绪", "path", realCamoufoxPath)

	rt, cleanup, err := setupRuntime(ctx, runtimeOptions{
		AuthDirs:      authDirs,
		GlobalProxy:   globalProxy,
		CamoufoxPath:  realCamoufoxPath,
		InitTimeout:   initTimeout,
		NeedCamoufox:  true,
		TemporaryChat: temporaryChat,
	})
	if err != nil {
		return err
	}
	defer cleanup()

	slog.Info("正在同步可用模型目录...")
	startupCtx, cancelStartup := context.WithTimeout(ctx, startupTimeout.Duration)
	models, err := rt.Service.Models(startupCtx)
	cancelStartup()
	if err != nil {
		return fmt.Errorf("预同步模型目录失败: %w", err)
	}
	slog.Info("模型目录就绪", "models", len(models))

	if prewarm {
		go prewarmServeWorker(ctx, rt, initTimeout.Duration)
	}

	handler := newPublicAPIHandler(rt.Service, apiKey)
	if requestTimeout.Duration > 0 {
		handler = requestTimeoutMiddleware(requestTimeout.Duration, handler)
	}

	listener, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", listen, err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()

	address := displayAddress(listener.Addr().String())
	fmt.Printf("OpenAI API 已启动: http://%s/v1\n", address)
	if strings.TrimSpace(apiKey) == "" {
		fmt.Println("警告: 未设置 --api-key，任何能访问该地址的客户端都可调用")
	} else {
		fmt.Println("API key 校验已启用")
	}

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP 服务异常退出: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Warn("HTTP 服务关闭超时，强制断开剩余连接", "error", err)
		_ = server.Close()
	}
	return nil
}

// newPublicAPIHandler 只挂载公开协议路由，不暴露依赖 Admin 的管理接口
func newPublicAPIHandler(service aistudio.Service, apiKey string) http.Handler {
	handler := api.NewHandler(service, api.Config{APIKey: strings.TrimSpace(apiKey)})
	root := http.NewServeMux()
	root.Handle("/v1/", handler)
	root.Handle("/v1beta/", handler)
	root.Handle("/upload/", handler)
	root.Handle("/health", handler)
	return root
}

// requestTimeoutMiddleware 为每个公开请求设置统一超时，避免生成请求永久挂起
func requestTimeoutMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// prewarmServeWorker 在后台启动一个 Camoufox WAA worker，降低首个生成请求的冷启动延迟
func prewarmServeWorker(ctx context.Context, rt *cliRuntime, timeout time.Duration) {
	if rt == nil || rt.Workers == nil || rt.Pool == nil {
		return
	}
	for _, status := range rt.Pool.Status() {
		if !status.Enabled || len(status.Models) == 0 {
			continue
		}
		if status.State != aistudio.AccountReady && status.State != aistudio.AccountBusy {
			continue
		}
		prewarmCtx, cancel := context.WithTimeout(ctx, timeout)
		started := time.Now()
		_, err := rt.Workers.Worker(prewarmCtx, status.ID, status.Models[0])
		cancel()
		if err != nil {
			slog.Warn("Camoufox WAA 预热失败", "account", status.ID, "error", err)
			return
		}
		slog.Info("Camoufox WAA 预热完成", "account", status.ID, "elapsed", time.Since(started).Round(time.Millisecond))
		return
	}
	slog.Warn("没有可预热的账户")
}

// displayAddress 将通配监听地址转换为本机可访问地址
func displayAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
