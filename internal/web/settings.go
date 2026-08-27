package web

import (
	"net/http"
	"os"
	"strings"
)

// SettingRow 配置项视图行。
type SettingRow struct {
	Key    string // 配置名(中文)
	Env    string // 环境变量名
	Value  string // 生效值(敏感字段已掩码)
	Source string // 默认 | 环境变量
}

// SettingsPage 系统配置页。只读展示;key 等敏感字段绝不落明文。
type SettingsPage struct {
	Common
	AI     []SettingRow
	System []SettingRow
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	c := s.cfg
	row := func(key, envName, value, def string) SettingRow {
		src := "默认"
		if os.Getenv(envName) != "" {
			src = "环境变量"
		}
		if value == "" && def != "" {
			value = def
		}
		return SettingRow{Key: key, Env: envName, Value: value, Source: src}
	}

	ai := []SettingRow{
		row("AI 服务地址", "PIKS_AI_BASE_URL", c.AIServiceBaseURL, "https://api.deepseek.com"),
		row("API Key", "PIKS_AI_API_KEY", maskSecret(c.AIAPIKey), ""),
		row("抽取模型(便宜档)", "PIKS_AI_MODEL_EXTRACT", c.AIModelExtract, "deepseek-chat"),
		row("推理模型(贵档)", "PIKS_AI_MODEL_REASONING", c.AIModelReasoning, "deepseek-reasoner"),
		row("日 token 预算(护栏)", "PIKS_AI_DAILY_TOKEN_BUDGET", budgetStr(c.AIDailyTokenBudget), ""),
	}
	sys := []SettingRow{
		row("数据库", "PIKS_DATABASE_URL", maskDBURL(c.DatabaseURL), ""),
		row("vault 路径(待下线)", "PIKS_VAULT_PATH", c.VaultPath, "./PIKS-Vault"),
	}

	s.render(w, "settings", SettingsPage{
		Common: Common{Title: "系统配置 · PIKS", Active: "settings"},
		AI:     ai, System: sys,
	})
}

// maskSecret API key 只显示首尾 4 位,绝不落明文。
func maskSecret(k string) string {
	if k == "" {
		return "未配置"
	}
	if len(k) <= 8 {
		return "已配置(***)"
	}
	return "已配置(" + k[:4] + "···" + k[len(k)-4:] + ")"
}

// maskDBURL 掩码连接串里的密码部分。
func maskDBURL(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		rest := u[i+3:]
		if at := strings.Index(rest, "@"); at >= 0 {
			auth := rest[:at]
			if colon := strings.Index(auth, ":"); colon >= 0 {
				return u[:i+3] + auth[:colon+1] + "***" + rest[at:]
			}
		}
	}
	return u
}

func budgetStr(b int64) string {
	if b <= 0 {
		return "未设置(护栏关闭)"
	}
	return itoa(b) + " tokens/日"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
