package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"aistudio-cli/internal/aistudio"
)

// accountHeaderProvider 为每个账户维护固定出口代理与发现的公共协议头
type accountHeaderProvider struct {
	mu          sync.RWMutex
	accounts    map[string]*accountHeaderState
	globalProxy string
}

type accountHeaderState struct {
	mu      sync.Mutex
	client  *http.Client
	headers http.Header
}

func newAccountHeaderProvider(accounts []*aistudio.Account, globalProxy string) (*accountHeaderProvider, error) {
	provider := &accountHeaderProvider{
		accounts:    make(map[string]*accountHeaderState, len(accounts)),
		globalProxy: strings.TrimSpace(globalProxy),
	}
	for _, account := range accounts {
		if err := provider.Add(account); err != nil {
			provider.Close()
			return nil, err
		}
	}
	return provider, nil
}

func (provider *accountHeaderProvider) Add(account *aistudio.Account) error {
	client, err := aistudio.NewProxyHTTPClient(account.EffectiveProxy(provider.globalProxy))
	if err != nil {
		return fmt.Errorf("创建账户 %s 的固定出口: %w", account.ID, err)
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if _, exists := provider.accounts[account.ID]; exists {
		client.CloseIdleConnections()
		return fmt.Errorf("账户固定出口已存在: %s", account.ID)
	}
	provider.accounts[account.ID] = &accountHeaderState{client: client}
	return nil
}

func (provider *accountHeaderProvider) Close() {
	provider.mu.Lock()
	accounts := provider.accounts
	provider.accounts = nil
	provider.mu.Unlock()
	for _, account := range accounts {
		account.client.CloseIdleConnections()
	}
}

// ProtocolHeaders 实现 aistudio.ProtocolHeaderProvider 接口
func (provider *accountHeaderProvider) ProtocolHeaders(ctx context.Context, accountID string) (http.Header, error) {
	provider.mu.RLock()
	account := provider.accounts[accountID]
	provider.mu.RUnlock()
	if account == nil {
		return nil, fmt.Errorf("账户不存在: %s", accountID)
	}
	account.mu.Lock()
	defer account.mu.Unlock()
	if len(account.headers) == 0 {
		headers, err := aistudio.DiscoverPublicHeaders(ctx, account.client)
		if err != nil {
			return nil, err
		}
		account.headers = headers.Clone()
	}
	return account.headers.Clone(), nil
}
