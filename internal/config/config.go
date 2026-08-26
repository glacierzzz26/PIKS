package config

import "os"

// Config 由环境变量驱动(换 AI 源/模型/库只需改 env,不动代码)。
type Config struct {
	// 数据库连接串。默认指向 docker-compose 的独立 postgres(宿主端口 5433)。
	DatabaseURL string
	// AI(OpenAI 兼容协议)
	AIServiceBaseURL string
	AIAPIKey         string
	AIModelExtract   string
	AIModelReasoning string
	// 日 token 预算,0 = 关闭护栏
	AIDailyTokenBudget int64
	// 生成的 Obsidian vault 仓库路径(设计 §7:独立 git 仓库,与代码仓库分离)
	VaultPath string
}

func Load() Config {
	return Config{
		DatabaseURL:        getenv("PIKS_DATABASE_URL", "postgres://piks:piks_dev_password@localhost:5433/piks?sslmode=disable"),
		AIServiceBaseURL:   getenv("PIKS_AI_BASE_URL", "https://api.deepseek.com"),
		AIAPIKey:           os.Getenv("PIKS_AI_API_KEY"),
		AIModelExtract:     getenv("PIKS_AI_MODEL_EXTRACT", "deepseek-chat"),
		AIModelReasoning:   getenv("PIKS_AI_MODEL_REASONING", "deepseek-reasoner"),
		AIDailyTokenBudget: atoi64(os.Getenv("PIKS_AI_DAILY_TOKEN_BUDGET")),
		VaultPath:          getenv("PIKS_VAULT_PATH", "./PIKS-Vault"),
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
