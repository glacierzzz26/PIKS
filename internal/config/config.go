package config

import "os"

// Config 由环境变量驱动(库/vault),大模型配置例外:AI 字段权威源 = 数据库
// app_config 表(config.ApplyAppConfig),不再读 PIKS_AI_* 环境变量。
type Config struct {
	// 数据库连接串。默认指向 docker-compose 的独立 postgres(宿主端口 5433)。
	DatabaseURL string
	// 生成的 Obsidian vault 仓库路径。迭代 5-2 已废弃:Web 直读 PG,
	// vault/GitHub 停更。空 = vault 禁用(daily-review/reconcile 跳过写盘+git)。
	VaultPath string
	// AI(OpenAI 兼容协议)。由 app_config 表提供,Load() 零值,ApplyAppConfig 合并。
	AIServiceBaseURL   string
	AIAPIKey           string
	AIModelExtract     string
	AIModelReasoning   string
	AIDailyTokenBudget int64
}

func Load() Config {
	return Config{
		DatabaseURL: getenv("PIKS_DATABASE_URL", "postgres://piks:piks_dev_password@localhost:5433/piks?sslmode=disable"),
		VaultPath:   getenv("PIKS_VAULT_PATH", ""), // 迭代 5-2:默认禁用 vault(弃 Obsidian/GitHub)
	}
}

// ApplyAppConfig 从 app_config 表合并 AI 配置。DB 值缺失/为空 → 保留零值(密钥=未配置);
// 迁移已种子默认值,页面编辑为唯一修改途径。
func (c *Config) ApplyAppConfig(m map[string]string) {
	if v := m["ai_service_base_url"]; v != "" {
		c.AIServiceBaseURL = v
	}
	if v := m["ai_api_key"]; v != "" {
		c.AIAPIKey = v
	}
	if v := m["ai_model_extract"]; v != "" {
		c.AIModelExtract = v
	}
	if v := m["ai_model_reasoning"]; v != "" {
		c.AIModelReasoning = v
	}
	if v := m["ai_daily_token_budget"]; v != "" {
		c.AIDailyTokenBudget = atoi64(v)
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi64(s string) int64 {
	if s == "" {
		return 0
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
