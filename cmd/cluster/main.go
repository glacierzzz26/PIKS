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
	limit := flag.Int("limit", 0, "max unclustered events per run (<=0 = 不限,默认取全部)")
	batch := flag.Int("batch", 20, "LLM pairs per prompt")
	reexamine := flag.Bool("reexamine", true, "run cross-cluster reexamination pass after normal clustering")
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

	runID, err := s.StartTaskRun(ctx, "cluster")
	if err != nil {
		fatal("start task run:", err)
	}

	provider := newProvider(cfg)
	if err := provider.HealthCheck(ctx); err != nil {
		finishFail(ctx, s, runID, err)
	}

	// issue #53:默认不限(取全部未聚类事件)。若调用方显式传了 -limit>0,先探测是否被截断,
	// 把结果写进 task_runs.meta,不再让「取不下的事件」静默留在池外。
	truncated := false
	if *limit > 0 {
		truncated, err = s.UnclusteredEventsTruncated(ctx, *limit)
		if err != nil {
			finishFail(ctx, s, runID, err)
		}
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
		"events":         len(events),
		"auto_groups":    len(cand.Auto),
		"llm_pairs":      len(cand.LLM),
		"llm_batches":    (len(cand.LLM) + *batch - 1) / *batch,
		"clusters":       len(comps),
		"merged":         merged,
		"ai_tokens":      tokens,
		"budget_checked": cfg.AIDailyTokenBudget > 0,
		// issue #53:显式记录截断状态,别让「取不下的事件」静默留在池外。
		"limit":          *limit, // <=0 = 不限
		"truncated":      truncated,
	}
	if truncated {
		fmt.Fprintf(os.Stderr, "WARN: cluster limit=%d 截断未聚类事件,本次仅处理最旧 %d 条;剩余未处理(见 -limit,0=不限)\n",
			*limit, len(events))
	}

	// 重审视 Pass(design cluster-quality):既有 canonical 跨簇互检 + 新事件↔既有簇。
	// 预算沿用本命令剩余额度;预算耗尽则跳过并如实记 meta,不报错。
	reexamPairs, reexamMerged, reexamTokens := 0, 0, int64(0)
	if *reexamine {
		remaining := int64(0) // maxTokens==0(未配预算)→ 0 == 不限(与 ConfirmPairs 语义一致)
		if maxTokens > 0 {
			remaining = maxTokens - tokens
			if remaining <= 0 {
				fmt.Println("cluster: reexamine skipped (daily token budget exhausted)")
				meta["reexam_skipped_budget"] = true
				remaining = -1 // 跳过哨兵
			}
		}
		if remaining >= 0 {
			reexamMerged, reexamTokens, reexamPairs, err = cluster.ReexamineClusters(ctx, s, provider, *batch, remaining)
			if err != nil {
				finishFail(ctx, s, runID, err)
			}
			merged += reexamMerged
			tokens += reexamTokens
		}
		meta["reexam_pairs"] = reexamPairs
		meta["reexam_merged"] = reexamMerged
		meta["reexam_tokens"] = reexamTokens
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
