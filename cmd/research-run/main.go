// research-run 个股深研编排命令(research 并入,design research-merge.md §4.4 D-6)。
//
// 一次运行 = 采集(research CLI) → LLM 合成(PIKS ai.Provider) → 渲染+Number Lint
// → 六项 Quality Gate → 归档 PG(research_runs)。任何一步失败如实落 status=failed。
//
// 用法:
//
//	bin/research-run 000560 [--profile short-term] [--days 60] [--force]
//
// 退出码:0 = done(含机检未过,产物仍可用);1 = failed(编排失败,error 已落库)。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"piks/internal/ai"
	"piks/internal/config"
	"piks/internal/research"
	"piks/internal/store"
)

func main() {
	profile := flag.String("profile", "complete-stock", "研究档案: complete-stock / short-term")
	days := flag.Int("days", 0, "研究周期(交易日),0 = 用档案默认")
	force := flag.Bool("force", false, "丢弃既有产物重开一次(保留历史 run)")
	runID := flag.String("run-id", "", "续跑指定 run(断点重试:跳过已完成步骤)")
	outDir := flag.String("out-dir", "", "产物目录(默认按 run_id 落临时根)")
	flag.Parse()

	// Go flag 遇到首个位置参数即停止解析,故 `000560 --profile x` 里 flag 会被吞。
	// 设计 §4.4 的用法是代码在前的,这里摘出代码后重解析剩余部分(两种顺序都支持)。
	rest := flag.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "用法: research-run <代码> [--profile short-term] [--days N] [--force]")
		os.Exit(2)
	}
	code := rest[0]
	if len(rest) > 1 {
		if err := flag.CommandLine.Parse(rest[1:]); err != nil {
			fatal("解析参数:", err)
		}
	}

	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)
	// AI 配置权威源 = 数据库 app_config(与 entity-build / weekly 一致)。
	if err := s.ApplyAppConfig(ctx, &cfg); err != nil {
		fatal("apply app config:", err)
	}

	o := research.New(s, newProvider(cfg), cfg.AIDailyTokenBudget)
	res, err := o.Run(ctx, research.Options{
		Code: code, Profile: *profile, Days: *days,
		RunID: *runID, Force: *force, OutDir: *outDir,
		RequireSynthesis: true, // CLI 深研:必须有 AI(快速模式仅 web 速评端点用)
	})
	if err != nil {
		fatal("research-run:", err)
	}

	if res.Status != research.StatusDone {
		fmt.Fprintf(os.Stderr, "❌ run %s status=%s: %s\n", res.RunID, res.Status, res.Error)
		os.Exit(1)
	}
	fmt.Printf("✅ run %s done: lint=%v gate=%v model=%s tokens=%d\n",
		res.RunID, res.Lint, res.Gate, res.Model, res.Tokens)
}

// newProvider 构造 AI provider(与 cmd/entity-build 同模式;PIKS_AI_PROVIDER=mock 走 mock)。
// AI 未配置时返回 nil,编排器在合成步如实失败(不降级不编造)。
func newProvider(cfg config.Config) ai.Provider {
	if os.Getenv("PIKS_AI_PROVIDER") == "mock" {
		return ai.NewMock()
	}
	if cfg.AIServiceBaseURL == "" || cfg.AIAPIKey == "" {
		return nil
	}
	// 合成用便宜档(extract),未配置回退 reasoning(与 weekly-ai-summary 同模式)。
	model := cfg.AIModelExtract
	if model == "" {
		model = cfg.AIModelReasoning
	}
	if model == "" {
		return nil
	}
	return ai.NewOpenAICompat(cfg.AIServiceBaseURL, cfg.AIAPIKey, model)
}

func fatal(v ...any) {
	_, _ = fmt.Fprintln(os.Stderr, append([]any{"research-run:", time.Now().Format(time.RFC3339)}, v...)...)
	os.Exit(1)
}
