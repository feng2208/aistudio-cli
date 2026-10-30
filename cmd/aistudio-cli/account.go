package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"aistudio-cli/internal/aistudio"
)

// accountSummary 用于列表展示的账户信息
type accountSummary struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Enabled     bool   `json:"enabled"`
	State       string `json:"state"`
	BenefitTier string `json:"benefit_tier"`
	Proxy       string `json:"proxy,omitempty"`
	ModelsCount int    `json:"models_count"`
	Locale      string `json:"locale,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
	AuthUser    int    `json:"auth_user,omitempty"`
}

func runAccountList(authDirs []string, asJSON bool) error {
	if len(authDirs) == 0 {
		authDirs = []string{"auth"}
	}
	store := aistudio.NewAccountStore(authDirs...)
	accounts, err := store.Load()
	if err != nil {
		return fmt.Errorf("读取账户列表失败: %w", err)
	}

	summaries := make([]accountSummary, 0, len(accounts))
	for _, acc := range accounts {
		summaries = append(summaries, accountSummary{
			ID:          acc.ID,
			Label:       acc.Config.Label,
			Enabled:     acc.Config.Enabled,
			State:       string(acc.State),
			BenefitTier: acc.BenefitTier.String(),
			Proxy:       acc.Config.Proxy,
			ModelsCount: len(acc.Models),
			Locale:      acc.Config.Locale,
			Timezone:    acc.Config.Timezone,
			AuthUser:    acc.Config.AuthUser,
		})
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summaries)
	}

	if len(summaries) == 0 {
		fmt.Println("未找到任何账户。")
		return nil
	}

	fmt.Printf("%-30s %-8s %-12s %-10s %-8s %s\n", "ID/邮箱", "状态", "权益", "启用", "模型数", "代理")
	fmt.Println(strings.Repeat("-", 80))
	for _, s := range summaries {
		enabledStr := "是"
		if !s.Enabled {
			enabledStr = "否"
		}
		proxyStr := s.Proxy
		if proxyStr == "" {
			proxyStr = "(继承全局)"
		}
		fmt.Printf("%-30s %-8s %-12s %-10s %-8d %s\n", s.ID, s.State, s.BenefitTier, enabledStr, s.ModelsCount, proxyStr)
	}
	return nil
}

type importAccountOptions struct {
	AuthDir   string
	CookieSrc string
	Email     string
	Proxy     string
	Locale    string
	Timezone  string
	AuthUser  int
}

func runAccountImport(opts importAccountOptions) error {
	if strings.TrimSpace(opts.AuthDir) == "" {
		opts.AuthDir = "auth"
	}

	var cookieText string
	if opts.CookieSrc == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("从标准输入读取 Cookie 失败: %w", err)
		}
		cookieText = string(data)
	} else {
		data, err := os.ReadFile(opts.CookieSrc)
		if err != nil {
			return fmt.Errorf("读取 Cookie 文件 %s 失败: %w", opts.CookieSrc, err)
		}
		cookieText = string(data)
	}

	state, err := aistudio.ParseCookieText(cookieText)
	if err != nil {
		return fmt.Errorf("解析 Cookie 失败: %w", err)
	}

	email := strings.TrimSpace(opts.Email)
	if email == "" {
		if ext, exists, _ := state.AuthExtension(); exists && strings.TrimSpace(ext.Source.Email) != "" {
			email = strings.TrimSpace(ext.Source.Email)
		}
	}
	if email == "" {
		return fmt.Errorf("Cookie 数据未包含邮箱信息，请通过 --email <your-email@gmail.com> 显式指定")
	}

	cfg := aistudio.DefaultAccountConfig(email)
	if opts.Proxy != "" {
		cfg.Proxy = strings.TrimSpace(opts.Proxy)
	}
	if opts.Locale != "" {
		cfg.Locale = strings.TrimSpace(opts.Locale)
	}
	if opts.Timezone != "" {
		cfg.Timezone = strings.TrimSpace(opts.Timezone)
	}
	if opts.AuthUser >= 0 {
		cfg.AuthUser = opts.AuthUser
	}

	store := aistudio.NewAccountStore(opts.AuthDir)
	acc, lease, err := store.Create(cfg, state)
	if err != nil {
		return fmt.Errorf("保存账户失败: %w", err)
	}
	defer lease.Release()

	fmt.Printf("账户导入成功！\nID/邮箱: %s\n目录: %s\n", acc.ID, acc.Directory)
	return nil
}
