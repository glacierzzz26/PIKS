// worker 抽取命令:取 status='raw' 的文档 → LLM 结构化抽取(Fact 层)→ 校验 → 入 events/evidences。
// 预算护栏:超日 token 阈值则提前停止。生产用真实 provider,测试用 PIKS_AI_PROVIDER=mock。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"piks/internal/ai"
	"piks/internal/config"
	"piks/internal/extract"
	"piks/internal/store"
)

func main() {
	limit := flag.Int("limit", 50, "max raw documents to process per run")
	retry := flag.Bool("retry", false, "also pick previously failed documents")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)
	// AI 配置权威源 = 数据库 app_config(不再读 PIKS_AI_* 环境变量)。
	if err := s.ApplyAppConfig(ctx, &cfg); err != nil {
		fatal("apply app config:", err)
	}

	runID, err := s.StartTaskRun(ctx, "worker")
	if err != nil {
		fatal("start task run:", err)
	}

	provider := newProvider(cfg)
	if err := provider.HealthCheck(ctx); err != nil {
		finishFail(ctx, s, runID, err)
	}
	extractor, err := extract.NewExtractor(provider, s, "prompts/extract.md", cfg.AIModelExtract)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}

	docs, err := s.ListRawPendingStatus(ctx, *limit, *retry)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}

	budget := cfg.AIDailyTokenBudget
	tokensToday := int64(0)
	guardDisabled := budget <= 0
	if !guardDisabled {
		midnight := config.BeijingMidnight(time.Now())
		tokensToday, _ = s.TokensSince(ctx, midnight)
	} else {
		// 0 = 无护栏,不是「不限预算」——显式告警(issue #75)。
		fmt.Fprintln(os.Stderr, "WARN: ai_daily_token_budget=0 ⇒ 预算护栏**关闭**,本轮不会拦截任何 LLM 调用;请在 /settings 设为非 0(建议 1000000)")
	}

	processed, events, failed, tokens := 0, 0, 0, int64(0)
	budgetExhausted := false
	for _, doc := range docs {
		if !guardDisabled && tokensToday+tokens >= budget {
			fmt.Printf("worker: daily token budget %d reached, stopping\n", budget)
			budgetExhausted = true
			break
		}
		n, used, err := extractor.Extract(ctx, &doc)
		tokens += used
		if err != nil {
			failed++
			_ = s.MarkRawFailed(ctx, doc.ID, err.Error())
			continue
		}
		processed++
		events += n
		_ = s.MarkRawProcessed(ctx, doc.ID, extractor.PipelineVersion())
	}

	meta := map[string]any{
		"processed":      processed,
		"events":         events,
		"failed":         failed,
		"ai_tokens":      tokens,
		"budget_checked": !guardDisabled,
		// issue #75:预算耗尽不得静默降级 —— 旧实现只 fmt.Printf 就继续记 success,
		// 前端看到绿色「已完成」却不知有一批文档因预算被跳过。
		"budget_exhausted": budgetExhausted,
		"guard_disabled":   guardDisabled,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fatal("finish task run:", err)
	}
	fmt.Printf("worker: processed=%d events=%d failed=%d tokens=%d\n", processed, events, failed, tokens)
}

func newProvider(cfg config.Config) ai.Provider {
	if os.Getenv("PIKS_AI_PROVIDER") == "mock" {
		return ai.NewMock()
	}
	return ai.NewOpenAICompat(cfg.AIServiceBaseURL, cfg.AIAPIKey, cfg.AIModelExtract)
}

func finishFail(ctx context.Context, s *store.Store, runID int64, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
	fatal("worker:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
