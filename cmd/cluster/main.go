// cluster 命令:事件语义去重聚类(迭代 1)。
// 规则直合 + 便宜档 LLM 批量确认 → event_clusters,canonical 保留、其余 merged。
// 预算:复用 PIKS_AI_DAILY_TOKEN_BUDGET(与抽取同池)。
//
// 收敛(issue #75):正常 pass 只扫「未扫描」的事件(cluster_scanned_at IS NULL),
// 扫完无对端的盖上水位戳,下轮不再进池;重审视 Pass 收窗到 -window-days。
// 两条合起来把池钉成常数(≈ 窗口天数 × 日入库量),而非「全量存量,且每日递增」。
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
	windowDays := flag.Int("window-days", 7,
		"重审视 Pass 的时间窗(天,0=不限)。只处理「窗口内仍活跃的簇」与「窗口内扫描过的事件」;\n"+
			"⚠️ 相隔超过该窗口才到达的重复事件不会被配对(召回代价,实测生产全部多源簇跨度 ≤1 天,默认 7 有余量)")
	dryRun := flag.Bool("dry-run", false, "只生成候选并落 meta,不调 LLM、不写库(验证候选池是否收敛)")
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
	if !*dryRun {
		if err := provider.HealthCheck(ctx); err != nil {
			finishFail(ctx, s, runID, err)
		}
	}

	// issue #53:默认不限(取全部未聚类事件)。若调用方显式传了 -limit>0,先探测是否被截断,
	// 把结果写进 task_runs.meta,不再让「取不下的事件」静默留在池外。
	// ⚠️ issue #75:截断时**不得**给池外事件盖扫描水位(它们本轮根本没被比对),见落盘处。
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

	// ⚠️ 早退**不能**放在这里(issue #75 潜伏 bug):重审视 Pass 在下方,
	// 「今日无新事件」收敛后是常态,若在此 return,重审视会静默停跑
	// (`task_runs.meta` 里 reexam_* 全部消失)。故拆出 var 供末尾判断。

	maxTokens := int64(0)
	budgetOff := cfg.AIDailyTokenBudget <= 0
	if !budgetOff {
		midnight := config.BeijingMidnight(time.Now())
		today, _ := s.TokensSince(ctx, midnight)
		if today < cfg.AIDailyTokenBudget {
			maxTokens = cfg.AIDailyTokenBudget - today
		}
	} else {
		// 0 = 无护栏,不是「不限预算」——显式告警,别让它静默放行(issue #75 护栏被关的根因)。
		fmt.Fprintln(os.Stderr, "WARN: ai_daily_token_budget=0 ⇒ 预算护栏**关闭**,本轮不会拦截任何 LLM 调用;请在 /settings 设为非 0(建议 1000000)")
	}

	var cand cluster.Candidate
	var verdicts []cluster.PairVerdict
	tokens := int64(0)
	comps := [][]int(nil)
	merged := 0
	if !*dryRun && len(events) > 0 {
		cand = cluster.GenCandidates(events)
		if len(cand.LLM) > 0 {
			verdicts, tokens, err = cluster.ConfirmPairs(ctx, provider, events, cand.LLM, *batch, maxTokens)
			if err != nil {
				finishFail(ctx, s, runID, err)
			}
		}
		comps = cluster.BuildComponents(len(events), cand.Auto, verdicts, cand.LLM)
		merged, err = cluster.ApplyClusters(ctx, s, events, comps)
		if err != nil {
			finishFail(ctx, s, runID, err)
		}
		// 收敛落盘:给「本轮比对过、最终无对端」的事件盖水位戳。
		// 截断时**跳过**(池外事件没被比对,标记它们 = 永久漏召回)。
		if !truncated {
			unmatched := cluster.UnmatchedIndices(len(events), comps)
			ids := make([]string, 0, len(unmatched))
			for _, i := range unmatched {
				ids = append(ids, events[i].ID)
			}
			if _, err := s.MarkEventsScanned(ctx, ids); err != nil {
				finishFail(ctx, s, runID, err)
			}
		}
	} else if *dryRun {
		// dry-run:只算候选,不调 LLM、不写库。用于验证「连续两次跑第二次显著变小」。
		cand = cluster.GenCandidates(events)
		comps = cluster.BuildComponents(len(events), cand.Auto, nil, cand.LLM)
	}

	// 重审视 Pass(design cluster-quality):既有 canonical 跨簇互检 + 新事件↔既有簇。
	// 预算沿用本命令剩余额度;预算耗尽则跳过并如实记 meta,不报错。
	var since time.Time
	if *windowDays > 0 {
		since = time.Now().Add(-time.Duration(*windowDays) * 24 * time.Hour)
	}
	reexamPairs, reexamMerged, reexamTokens := 0, 0, int64(0)
	skippedBudget := false
	if *reexamine && !*dryRun {
		remaining := int64(0) // maxTokens==0(未配预算)→ 0 == 不限(与 ConfirmPairs 语义一致)
		if maxTokens > 0 {
			remaining = maxTokens - tokens
			if remaining <= 0 {
				fmt.Println("cluster: reexamine skipped (daily token budget exhausted)")
				skippedBudget = true
				remaining = -1 // 跳过哨兵
			}
		}
		if remaining >= 0 {
			reexamMerged, reexamTokens, reexamPairs, err = cluster.ReexamineClusters(ctx, s, provider, *batch, remaining, since)
			if err != nil {
				finishFail(ctx, s, runID, err)
			}
			merged += reexamMerged
			tokens += reexamTokens // ⚠️ 必须在组装 meta 之前加(issue #75 账本盲区:旧代码 map 已拷贝完才加)
		}
	}

	// ⚠️ 组装 meta 放在**所有** token 累加之后 —— 旧代码在 reexamine 之前就把
	// "ai_tokens": tokens 写进 map,而 Go 的 map 存的是值拷贝,后面 tokens += 加不进去,
	// 于是 reexamine 花费(实测单轮 170~290 万)完全不进账本 ⇒ TokensSince 少算近一半,
	// 手动填的预算也就拦不住真实花费。这是 issue #75 的 4 处漏账里最大的一处。
	meta := map[string]any{
		"events":         len(events),
		"auto_groups":    len(cand.Auto),
		"llm_pairs":      len(cand.LLM),
		"llm_batches":    (len(cand.LLM) + *batch - 1) / *batch,
		"clusters":       len(comps),
		"merged":         merged,
		"ai_tokens":      tokens,
		"budget_checked": !budgetOff,
		// issue #53:显式记录截断状态,别让「取不下的事件」静默留在池外。
		"limit":     *limit, // <=0 = 不限
		"truncated": truncated,
		// issue #75:窗口与护栏状态如实落账,便于事后核对花费口径。
		"window_days":      *windowDays,
		"guard_disabled":   budgetOff,
		"budget_exhausted": budgetExhausted(maxTokens, tokens),
		"dry_run":          *dryRun,
	}
	if *reexamine {
		meta["reexam_pairs"] = reexamPairs
		meta["reexam_merged"] = reexamMerged
		meta["reexam_tokens"] = reexamTokens
		if skippedBudget {
			meta["reexam_skipped_budget"] = true
		}
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fatal("finish task run:", err)
	}
	fmt.Printf("cluster: events=%d auto_groups=%d llm_pairs=%d clusters=%d merged=%d tokens=%d\n",
		len(events), len(cand.Auto), len(cand.LLM), len(comps), merged, tokens)
}

// budgetExhausted 报告正常 pass 是否因预算被截停(maxTokens>0 且已用满/超出)。
// 主 pass 与 reexamine 分列,便于区分「谁把钱花完了」(issue #75:预算耗尽不得静默降级)。
func budgetExhausted(maxTokens, used int64) bool {
	return maxTokens > 0 && used >= maxTokens
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
