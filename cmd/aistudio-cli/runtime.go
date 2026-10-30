package main

import (
	"context"
	"fmt"

	"aistudio-cli/internal/aistudio"
)

// cliRuntime 封装 CLI 所需的服务实例及资源释放句柄
type cliRuntime struct {
	Store     *aistudio.AccountStore
	Pool      *aistudio.AccountPool
	Service   *aistudio.PooledService
	Client    *aistudio.Client
	Headers   *accountHeaderProvider
	Transport *aistudio.MakerSuiteHTTPTransport
	Workers   *camoufoxWorkerPool
}

type runtimeOptions struct {
	AuthDirs      []string
	GlobalProxy   string
	CamoufoxPath  string
	InitTimeout   timeDuration
	NeedCamoufox  bool
	TemporaryChat bool
}

func setupRuntime(ctx context.Context, opts runtimeOptions) (*cliRuntime, func(), error) {
	if len(opts.AuthDirs) == 0 {
		opts.AuthDirs = []string{"auth"}
	}

	store := aistudio.NewAccountStore(opts.AuthDirs...)
	accounts, err := store.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("载入账户目录失败: %w", err)
	}
	if len(accounts) == 0 {
		return nil, nil, fmt.Errorf("未在指定目录 %v 找到有效账户", opts.AuthDirs)
	}

	pool := aistudio.NewAccountPool(accounts, 2)
	pool.SetUpstreamChannels([]aistudio.Channel{aistudio.ChannelPlayground})

	headers, err := newAccountHeaderProvider(accounts, opts.GlobalProxy)
	if err != nil {
		return nil, nil, err
	}

	transport, err := aistudio.NewMakerSuiteHTTPTransport(aistudio.HTTPTransportOptions{
		Pool:        pool,
		Signer:      aistudio.NewSigner(),
		Headers:     headers,
		GlobalProxy: opts.GlobalProxy,
	})
	if err != nil {
		headers.Close()
		return nil, nil, fmt.Errorf("创建 HTTP 传输层失败: %w", err)
	}

	var workers *camoufoxWorkerPool
	var protected aistudio.ProtectedTransport
	if opts.NeedCamoufox {
		workers = newCamoufoxWorkerPool(
			pool,
			opts.CamoufoxPath,
			opts.GlobalProxy,
			opts.InitTimeout.Duration,
			opts.TemporaryChat,
		)
		protected = aistudio.NewWorkerProtectedTransport(aistudio.WorkerProtectedTransportOptions{
			Transport:    transport,
			Workers:      workers,
			SetupTimeout: opts.InitTimeout.Duration,
		})
	}

	client := aistudio.NewClient(aistudio.ClientOptions{
		Transport:       transport,
		Protected:       protected,
		ContextProvider: aistudio.NewPoolRequestContextProvider(pool),
	})

	service := aistudio.NewPooledService(pool, client)
	service.ContinuationRewrite = true

	rt := &cliRuntime{
		Store:     store,
		Pool:      pool,
		Service:   service,
		Client:    client,
		Headers:   headers,
		Transport: transport,
		Workers:   workers,
	}

	cleanup := func() {
		if workers != nil {
			workers.Close()
		}
		if transport != nil {
			transport.CloseIdleConnections()
		}
		if headers != nil {
			headers.Close()
		}
	}

	return rt, cleanup, nil
}
