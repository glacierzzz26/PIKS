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
	if budget > 0 {
		midnight := time.Now().Truncate(24 * time.Hour)
		tokensToday, _ = s.TokensSince(ctx, midnight)
	}

	processed, events, failed, tokens := 0, 0, 0, int64(0)
	for _, doc := range docs {
		if budget > 0 && tokensToday+tokens >= budget {
			fmt.Printf("worker: daily token budget %d reached, stopping\n", budget)
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
		"processed":     processed,
		"events":        events,
		"failed":        failed,
		"ai_tokens":     tokens,
		"budget_checked": budget > 0,
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
