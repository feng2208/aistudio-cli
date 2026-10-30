package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"aistudio-cli/internal/aistudio"
	"aistudio-cli/internal/camoufoxnative"
)

type timeDuration struct {
	time.Duration
}

func (d *timeDuration) String() string {
	return d.Duration.String()
}

func (d *timeDuration) Set(v string) error {
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return err
	}
	d.Duration = parsed
	return nil
}

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	subcommand := os.Args[1]
	args := os.Args[2:]

	var err error
	switch subcommand {
	case "account":
		err = handleAccountCommand(ctx, args)
	case "models":
		err = handleModelsCommand(ctx, args)
	case "generate":
		err = handleGenerateCommand(ctx, args)
	case "serve":
		err = handleServeCommand(ctx, args)
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n\n", subcommand)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`AIStudio2API 独立命令行工具 (Playground Private API)

用法:
  aistudio-cli <子命令> [参数...]

子命令:
  account list      列出所有已配置的账户及其状态
  account import    从 Cookie 文本/导出文件导入新账户
  models            查询 AI Studio 可用模型目录
  generate          调用 Playground 私有 GenerateContent 接口生成文本
  serve             启动兼容 OpenAI Chat Completions 的 HTTP 服务

运行 'aistudio-cli <子命令> -h' 查看各子命令详细参数。`)
}

func handleAccountCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Println("用法: aistudio-cli account [list|import] [参数...]")
		return nil
	}
	action := args[0]
	actionArgs := args[1:]

	switch action {
	case "list":
		fs := flag.NewFlagSet("account list", flag.ExitOnError)
		var authDirs stringSlice
		var asJSON bool
		fs.Var(&authDirs, "auth-dir", "账户状态目录，可重复或以逗号分隔 (默认 auth)")
		fs.BoolVar(&asJSON, "json", false, "以 JSON 格式输出")
		if err := fs.Parse(actionArgs); err != nil {
			return err
		}
		if len(authDirs) == 0 {
			authDirs = []string{"auth"}
		}
		return runAccountList(authDirs, asJSON)

	case "import":
		fs := flag.NewFlagSet("account import", flag.ExitOnError)
		var authDir string
		var cookieSrc string
		var email string
		var proxy string
		var locale string
		var timezone string
		var authUser int
		fs.StringVar(&authDir, "auth-dir", "auth", "保存新账户的目录根路径")
		fs.StringVar(&cookieSrc, "cookies", "", "Cookie 文件路径，或 '-' 从标准输入读取 (必须)")
		fs.StringVar(&email, "email", "", "Google 邮箱地址 (Cookie 数据中无邮箱时必须指定)")
		fs.StringVar(&proxy, "proxy", "", "此账户专用 HTTP/SOCKS5 代理")
		fs.StringVar(&locale, "locale", "", "浏览器语言区域 (默认根据系统检测)")
		fs.StringVar(&timezone, "timezone", "", "浏览器时区 (默认根据系统检测)")
		fs.IntVar(&authUser, "auth-user", 0, "Google AuthUser 索引 (0-9)")
		if err := fs.Parse(actionArgs); err != nil {
			return err
		}
		if cookieSrc == "" {
			return fmt.Errorf("必须通过 --cookies <path|-> 指定 Cookie 输入")
		}
		return runAccountImport(importAccountOptions{
			AuthDir:   authDir,
			CookieSrc: cookieSrc,
			Email:     email,
			Proxy:     proxy,
			Locale:    locale,
			Timezone:  timezone,
			AuthUser:  authUser,
		})

	default:
		return fmt.Errorf("未知的 account 动作: %s (支持 list 或 import)", action)
	}
}

func handleModelsCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("models", flag.ExitOnError)
	var authDirs stringSlice
	var globalProxy string
	var asJSON bool
	var timeout timeDuration = timeDuration{Duration: 2 * time.Minute}
	var verbose bool

	fs.Var(&authDirs, "auth-dir", "账户目录，可重复或逗号分隔 (默认 auth)")
	fs.StringVar(&globalProxy, "proxy", "", "全局 HTTP/SOCKS5 代理")
	fs.BoolVar(&asJSON, "json", false, "以 JSON 格式输出模型目录")
	fs.Var(&timeout, "timeout", "请求超时时间 (默认 2m)")
	fs.BoolVar(&verbose, "verbose", false, "输出调试日志")

	if err := fs.Parse(args); err != nil {
		return err
	}

	setupLogLevel(verbose)

	if len(authDirs) == 0 {
		authDirs = []string{"auth"}
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout.Duration)
	defer cancel()

	rt, cleanup, err := setupRuntime(reqCtx, runtimeOptions{
		AuthDirs:     authDirs,
		GlobalProxy:  globalProxy,
		NeedCamoufox: false,
	})
	if err != nil {
		return err
	}
	defer cleanup()

	models, err := rt.Service.Models(reqCtx)
	if err != nil {
		return fmt.Errorf("拉取模型目录失败: %w", err)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(models)
	}

	fmt.Printf("%-35s %-30s %-12s %s\n", "模型 ID", "显示名称", "上下文上限", "支持方法")
	fmt.Println(strings.Repeat("-", 100))
	for _, m := range models {
		fmt.Printf("%-35s %-30s %-12d %s\n", m.ID, m.Name, m.InputTokenLimit, strings.Join(m.Methods, ","))
	}
	return nil
}

func handleGenerateCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	var authDirs stringSlice
	var globalProxy string
	var camoufoxPath string
	var modelID string
	var prompt string
	var promptFile string
	var systemPrompt string
	var accountID string
	var stream bool
	var asJSON bool
	var timeout timeDuration = timeDuration{Duration: 30 * time.Minute}
	var initTimeout timeDuration = timeDuration{Duration: 2 * time.Minute}
	var temporaryChat bool
	var verbose bool

	fs.Var(&authDirs, "auth-dir", "账户目录，可重复或逗号分隔 (默认 auth)")
	fs.StringVar(&globalProxy, "proxy", "", "全局 HTTP/SOCKS5 代理")
	fs.StringVar(&camoufoxPath, "camoufox-path", "", "指定本地 Camoufox 可执行文件完整路径 (若不指定则自动定位或下载)")
	fs.StringVar(&modelID, "model", "gemini-2.5-flash", "目标模型 ID")
	fs.StringVar(&prompt, "prompt", "", "提示词内容 (若为 '-' 或空且 stdin 有内容则从标准输入读取)")
	fs.StringVar(&promptFile, "prompt-file", "", "从指定文本文件读取提示词")
	fs.StringVar(&systemPrompt, "system", "", "系统指令提示词 (System Instruction)")
	fs.StringVar(&accountID, "account", "", "指定调用的账户 ID/邮箱 (默认自动轮询)")
	fs.BoolVar(&stream, "stream", true, "是否流式输出生成内容 (默认 true)")
	fs.BoolVar(&asJSON, "json", false, "输出 JSON/NDJSON 格式结果")
	fs.Var(&timeout, "timeout", "生成总超时时间 (默认 30m)")
	fs.Var(&initTimeout, "init-timeout", "Camoufox WAA 初始化超时 (默认 2m)")
	fs.BoolVar(&temporaryChat, "temporary-chat", false, "WAA 是否使用临时对话模式")
	fs.BoolVar(&verbose, "verbose", false, "输出调试日志到 stderr")

	if err := fs.Parse(args); err != nil {
		return err
	}

	setupLogLevel(verbose)

	finalPrompt, err := resolvePrompt(prompt, promptFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(finalPrompt) == "" {
		return fmt.Errorf("提示词内容不能为空")
	}

	if len(authDirs) == 0 {
		authDirs = []string{"auth"}
	}

	if camoufoxPath != "" {
		_ = os.Setenv("CAMOUFOX_PATH", camoufoxPath)
	}

	slog.Info("正在查找或准备 Camoufox 运行环境...")
	realCamoufoxPath, err := camoufoxnative.FindExecutable(ctx, globalProxy)
	if err != nil {
		return fmt.Errorf("准备 Camoufox 浏览器环境失败: %w", err)
	}
	slog.Info("Camoufox 环境就绪", "path", realCamoufoxPath)

	reqCtx, cancel := context.WithTimeout(ctx, timeout.Duration)
	defer cancel()

	rt, cleanup, err := setupRuntime(reqCtx, runtimeOptions{
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
	if accountID != "" {
		_, err = rt.Service.RefreshAccountModels(reqCtx, accountID)
	} else {
		_, err = rt.Service.Models(reqCtx)
	}
	if err != nil {
		slog.Warn("同步模型目录遇到警告", "error", err)
	}

	generateReq := aistudio.GenerateRequest{
		Model: strings.TrimPrefix(strings.TrimSpace(modelID), "models/"),
		Contents: []aistudio.Content{
			{
				Role: "user",
				Parts: []aistudio.Part{
					{Text: finalPrompt},
				},
			},
		},
		AccountID: strings.TrimSpace(accountID),
	}
	if strings.TrimSpace(systemPrompt) != "" {
		generateReq.System = []string{strings.TrimSpace(systemPrompt)}
	}

	slog.Info("发起 Playground 私有生成请求...", "model", generateReq.Model)
	eventStream, err := rt.Service.Generate(reqCtx, generateReq)
	if err != nil {
		return fmt.Errorf("生成请求失败: %w", err)
	}

	collector := newEventCollector(asJSON, stream)
	for event := range eventStream {
		if err := collector.Handle(event); err != nil {
			return fmt.Errorf("处理生成流事件失败: %w", err)
		}
	}

	return collector.Finish()
}

func resolvePrompt(prompt string, promptFile string) (string, error) {
	if promptFile != "" {
		data, err := os.ReadFile(promptFile)
		if err != nil {
			return "", fmt.Errorf("读取提示词文件 %s: %w", promptFile, err)
		}
		return string(data), nil
	}

	if prompt == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("从标准输入读取提示词: %w", err)
		}
		return string(data), nil
	}

	if prompt != "" {
		return prompt, nil
	}

	fi, err := os.Stdin.Stat()
	if err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("从管道读取提示词: %w", err)
		}
		return string(data), nil
	}

	return "", nil
}

func setupLogLevel(verbose bool) {
	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
}
