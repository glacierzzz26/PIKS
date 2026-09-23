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
	"piks/internal/model"
	"piks/internal/store"
)

func main() {
	limit := flag.Int("limit", 50, "max raw documents to process per run")
	retry := flag.Bool("retry", false, "also pick previously failed documents")
	// 成本粗筛(issue #83 P-4 / 原 P7)。默认**保守**:coarse-defer=false ⇒ 只排序不筛,不落 deferred。
	coarseDefer := flag.Bool("coarse-defer", false,
		"开启粗筛:把「超当日配额且无源信号」或「超时间窗且非必送」的文档落 status='deferred'(默认关,保守)")
	coarseMax := flag.Int("coarse-max", 0,
		"粗筛保留上限(配合 -coarse-defer);<=0 = 不限(默认:与 -limit 同效,仅排序)")
	coarseWindow := flag.Duration("coarse-window", 0,
		"粗筛时间窗:超此窗且非必送(important/confirmed)的文档落 deferred;<=0 = 不限(默认)")
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

	// 成本粗筛(issue #83 P-4 / 原 P7):**先筛再抽**。默认保守(不落 deferred,只排序),
	// 由 -coarse-defer 显式开启筛选。被挡下的落 status='deferred'(不 delete、不静默丢)。
	// ⚠️ 粗筛不替代预算护栏:预算是硬上限,粗筛只是「预算不够时先给谁」。
	gated := extract.Gate(gateInputs(docs), extract.GateConfig{
		MustSend:       true,
		Window:         *coarseWindow,
		Max:            *coarseMax,
		DeferLowSignal: *coarseDefer,
	})
	deferN := len(gated.Deferred)
	if deferN > 0 {
		// 原因文案由 MarkRawDeferred 统一落 error 列;明细按原因归并进 task_runs.meta(见末段)。
		if _, err := s.MarkRawDeferred(ctx, gated.Deferred, "粗筛:超当日配额/超时间窗(见 task_runs.meta.defer_reason)"); err != nil {
			finishFail(ctx, s, runID, err)
		}
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

	// 粗筛后保持优先级顺序送抽取(Gate 已按 必送→分级→阅读数→时间新 排好)。
	kept := make([]model.RawDocument, 0, len(gated.Keep))
	byID := map[string]model.RawDocument{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	for _, k := range gated.Keep {
		if d, ok := byID[k.ID]; ok {
			kept = append(kept, d)
		}
	}

	processed, events, failed, tokens := 0, 0, 0, int64(0)
	budgetExhausted := false
	for _, doc := range kept {
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
		// issue #83 P-4 粗筛记账:延迟条数 + 原因(按原因归并)。0 = 本轮未筛(默认)。
		"deferred":      deferN,
		"defer_reason":  deferReasonSummary(gated.Reasons),
		"defer_enabled": *coarseDefer,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fatal("finish task run:", err)
	}
	fmt.Printf("worker: processed=%d events=%d failed=%d deferred=%d tokens=%d\n", processed, events, failed, deferN, tokens)
}

// gateInputs 把 worker 取到的 raw 文档投影成粗筛入参(只有 Extra/RetrievedAt/ID 参与判据)。
func gateInputs(docs []model.RawDocument) []extract.RawGateInput {
	out := make([]extract.RawGateInput, len(docs))
	for i, d := range docs {
		out[i] = extract.RawGateInput{ID: d.ID, Extra: d.Extra, RetrievedAt: d.RetrievedAt}
	}
	return out
}

// deferReasonSummary 把 id→原因 归并为「原因 → 条数」,落 task_runs.meta 供对账。
// 无延迟时返回 nil(不写空对象)。
func deferReasonSummary(reasons map[string]string) map[string]int {
	if len(reasons) == 0 {
		return nil
	}
	out := map[string]int{}
	for _, r := range reasons {
		out[r]++
	}
	return out
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
