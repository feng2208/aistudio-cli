package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"aistudio-cli/internal/aistudio"
	"aistudio-cli/internal/camoufoxnative"
)

// camoufoxWorkerPool 维护已启动的 Camoufox WAA worker
type camoufoxWorkerPool struct {
	mu            sync.Mutex
	workers       map[string]*aistudio.NativeWorker
	pool          *aistudio.AccountPool
	camoufoxPath  string
	globalProxy   string
	initTimeout   time.Duration
	temporaryChat bool
	closed        bool
}

func newCamoufoxWorkerPool(
	pool *aistudio.AccountPool,
	camoufoxPath string,
	globalProxy string,
	initTimeout time.Duration,
	temporaryChat bool,
) *camoufoxWorkerPool {
	return &camoufoxWorkerPool{
		workers:       make(map[string]*aistudio.NativeWorker),
		pool:          pool,
		camoufoxPath:  camoufoxPath,
		globalProxy:   strings.TrimSpace(globalProxy),
		initTimeout:   initTimeout,
		temporaryChat: temporaryChat,
	}
}

// Worker 实现 aistudio.ProtectedPreparerProvider 接口
func (m *camoufoxWorkerPool) Worker(ctx context.Context, accountID string, modelID string) (aistudio.ProtectedPreparer, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, fmt.Errorf("WAA worker pool 已关闭")
	}

	accountID = strings.TrimSpace(accountID)
	if existing, ok := m.workers[accountID]; ok {
		state := existing.State()
		if state.Phase == aistudio.WorkerReady || state.Phase == aistudio.WorkerBusy {
			return existing, nil
		}
		_ = existing.Close()
		delete(m.workers, accountID)
	}

	account, err := m.pool.Account(accountID)
	if err != nil {
		return nil, fmt.Errorf("获取账户 %s: %w", accountID, err)
	}

	bootstrapModel, err := m.pool.BootstrapModel(accountID)
	if err != nil {
		bootstrapModel = "gemini-flash-latest"
	}

	proxy := account.EffectiveProxy(m.globalProxy)
	options := camoufoxnative.Options{
		ExecutablePath:   m.camoufoxPath,
		StorageStatePath: account.StoragePath,
		Model:            bootstrapModel,
		Locale:           account.Config.Locale,
		Timezone:         account.Config.Timezone,
		Proxy:            proxy,
		Headless:         true,
		TemporaryChat:    m.temporaryChat,
		AuthUser:         account.Config.AuthUser,
		StartupProgress: func(stage camoufoxnative.StartupStage) {
			slog.Debug("Camoufox WAA worker 启动进度", "account", accountID, "stage", stage)
		},
	}

	initCtx, cancel := context.WithTimeout(ctx, m.initTimeout)
	defer cancel()

	slog.Info("正在初始化 Camoufox WAA worker", "account", accountID, "model", bootstrapModel)
	worker, err := aistudio.NewNativeWorker(initCtx, accountID, options)
	if err != nil {
		return nil, fmt.Errorf("启动 Camoufox WAA worker (%s): %w", accountID, err)
	}

	m.workers[accountID] = worker
	return worker, nil
}

func (m *camoufoxWorkerPool) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for id, worker := range m.workers {
		slog.Debug("正在关闭 Camoufox WAA worker", "account", id)
		_ = worker.Close()
	}
	m.workers = make(map[string]*aistudio.NativeWorker)
}
