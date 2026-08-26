// cluster 命令:事件语义去重聚类(迭代 1)。
// 规则直合 + 便宜档 LLM 批量确认 → event_clusters,canonical 保留、其余 merged。
// 预算:复用 PIKS_AI_DAILY_TOKEN_BUDGET(与抽取同池)。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"piks/internal/ai"
	"piks/internal/cluster"
	"piks/internal/config"
	"piks/internal/store"
)

func main() {
	limit := flag.Int("limit", 100, "max unclustered events per run")
	batch := flag.Int("batch", 20, "LLM pairs per prompt")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "cluster")
	if err != nil {
		fatal("start task run:", err)
	}

	provider := newProvider(cfg)
	if err := provider.HealthCheck(ctx); err != nil {
		finishFail(ctx, s, runID, err)
	}

	events, err := s.ListUnclusteredEvents(ctx, *limit)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}
	if len(events) == 0 {
		fmt.Println("cluster: no unclustered events")
		_ = s.FinishTaskRun(ctx, runID, "success", "", map[string]any{"events": 0})
		return
	}

	cand := cluster.GenCandidates(events)

	maxTokens := int64(0)
	if cfg.AIDailyTokenBudget > 0 {
		midnight := time.Now().Truncate(24 * time.Hour)
		today, _ := s.TokensSince(ctx, midnight)
		if today < cfg.AIDailyTokenBudget {
			maxTokens = cfg.AIDailyTokenBudget - today
		}
	}
	var verdicts []cluster.PairVerdict
	tokens := int64(0)
	if len(cand.LLM) > 0 {
		verdicts, tokens, err = cluster.ConfirmPairs(ctx, provider, events, cand.LLM, *batch, maxTokens)
		if err != nil {
			finishFail(ctx, s, runID, err)
		}
	}

	comps := cluster.BuildComponents(len(events), cand.Auto, verdicts, cand.LLM)
	merged, err := cluster.ApplyClusters(ctx, s, events, comps, verdicts, cand.LLM)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}

	meta := map[string]any{
		"events":        len(events),
		"auto_groups":   len(cand.Auto),
		"llm_pairs":     len(cand.LLM),
		"llm_batches":   (len(cand.LLM) + *batch - 1) / *batch,
		"clusters":      len(comps),
		"merged":        merged,
		"ai_tokens":     tokens,
		"budget_checked": cfg.AIDailyTokenBudget > 0,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fatal("finish task run:", err)
	}
	fmt.Printf("cluster: events=%d auto_groups=%d llm_pairs=%d clusters=%d merged=%d tokens=%d\n",
		len(events), len(cand.Auto), len(cand.LLM), len(comps), merged, tokens)
}

func newProvider(cfg config.Config) ai.Provider {
	if os.Getenv("PIKS_AI_PROVIDER") == "mock" {
		return ai.NewMock()
	}
	return ai.NewOpenAICompat(cfg.AIServiceBaseURL, cfg.AIAPIKey, cfg.AIModelExtract)
}

func finishFail(ctx context.Context, s *store.Store, runID int64, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
	fatal("cluster:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
