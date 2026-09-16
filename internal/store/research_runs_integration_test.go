package store_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"piks/internal/store"
)

// TestResearchRunCRUD research_runs 集成冒烟(设计 §5.2 T3 验收:CRUD 测试过)。
// 与 smoke_test 同开关;用唯一 run_id 保证可重复运行,结束清理不留痕。
func TestResearchRunCRUD(t *testing.T) {
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set (integration off by default)")
	}
	dsn := os.Getenv("PIKS_DATABASE_URL")
	if dsn == "" {
		t.Skip("PIKS_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	// 注意注册顺序:t.Cleanup 是 LIFO。先注册关池(最后跑),再注册删行(先跑)——
	// 若改用 `defer pool.Close()`,defer 会早于 Cleanup 执行,清理将作用在已关闭的池上而静默失败。
	runID := "test_sz000560_short-term_" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() { pool.Close() })
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE run_id=$1`, runID) })

	asOf := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	// 1. create(pending)
	created, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: "000560", Symbol: "sz000560",
		Profile: "short-term", AsOf: asOf,
	})
	if err != nil || !created {
		t.Fatalf("create: created=%v err=%v", created, err)
	}
	// 2. 幂等:同 run_id 再建不增行
	created2, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: "000560", Symbol: "sz000560", Profile: "short-term", AsOf: asOf,
	})
	if err != nil || created2 {
		t.Fatalf("idempotent create: created=%v err=%v", created2, err)
	}

	// 3. 状态推进 + 落产物
	if err := s.UpdateResearchRunStatus(ctx, runID, "gathering", ""); err != nil {
		t.Fatalf("status: %v", err)
	}
	md := "# 报告\n确定性骨架"
	if err := s.SaveResearchArtifacts(ctx, runID, &store.ResearchRun{
		Metrics:   json.RawMessage(`{"meta":{"as_of":"2026-09-11"},"price":{"pct_20d":3.2}}`),
		Synthesis: json.RawMessage(`{"summary":"摘要","trend":"趋势","conclusion":"结论"}`),
		Markdown:  &md,
		Lint:      json.RawMessage(`{"scanned":42,"matched":41,"passed":true,"issues":[]}`),
		Gate:      json.RawMessage(`{"passed":true,"checks":[]}`),
		Evidence:  json.RawMessage(`[{"id":"ev1","section":"price"}]`),
		Model:     "deepseek-chat", Tokens: 563,
	}); err != nil {
		t.Fatalf("save artifacts: %v", err)
	}

	// 4. 读回校验(metrics/synthesis/markdown/lint/gate/evidence/model/tokens 齐全)
	got, err := s.GetResearchRun(ctx, runID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "gathering" || got.Model != "deepseek-chat" || got.Tokens != 563 {
		t.Fatalf("readback mismatch: status=%s model=%s tokens=%d", got.Status, got.Model, got.Tokens)
	}
	if got.Markdown == nil || *got.Markdown != md {
		t.Fatalf("markdown mismatch: %v", got.Markdown)
	}
	var metrics map[string]any
	if err := json.Unmarshal(got.Metrics, &metrics); err != nil || metrics["price"] == nil {
		t.Fatalf("metrics mismatch: %s err=%v", got.Metrics, err)
	}
	if got.Code != "000560" || !got.AsOf.Equal(asOf) {
		t.Fatalf("code/as_of mismatch: %s %s", got.Code, got.AsOf)
	}

	// 5. 收尾为 done
	if err := s.FinishResearchRun(ctx, runID, "done", "", nil); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, _ = s.GetResearchRun(ctx, runID)
	if got.Status != "done" {
		t.Fatalf("finish status = %s", got.Status)
	}

	// 6. 列表按 code 过滤命中
	list, err := s.ListResearchRuns(ctx, "000560", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	hit := false
	for _, r := range list {
		if r.RunID == runID {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("list by code missed %s", runID)
	}
	t.Logf("research_runs CRUD ok: %s (%d rows for 000560)", runID, len(list))
}

// TestFindActiveResearchRun 触发侧防重查询(issue #7):
// 只匹配「进行中」的 run;done/failed 不算(历史版本是刻意保留的时间序列)。
// 用独立 code 保证与其它测试互不干扰,结束清理。
func TestFindActiveResearchRun(t *testing.T) {
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set (integration off by default)")
	}
	dsn := os.Getenv("PIKS_DATABASE_URL")
	if dsn == "" {
		t.Skip("PIKS_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	// 用一个测试专用 code,避免与真实数据/其它测试撞车。
	const code = "999998"
	// LIFO:先注册关池(最后跑),再注册删行(先跑)。
	t.Cleanup(func() { pool.Close() })
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE code=$1`, code) })
	_, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE code=$1`, code)

	asOf := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	// 1. 无 run → 查不到
	if got, err := s.FindActiveResearchRun(ctx, code, "short-term"); err != nil || got != nil {
		t.Fatalf("empty: got=%v err=%v", got, err)
	}

	// 2. 建一条 pending → 命中
	runID := "test_sz999998_short-term_" + time.Now().Format("20060102150405.000000")
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: code, Symbol: "sz999998", Profile: "short-term", AsOf: asOf,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.FindActiveResearchRun(ctx, code, "short-term")
	if err != nil || got == nil || got.RunID != runID {
		t.Fatalf("pending should be active: got=%v err=%v", got, err)
	}

	// 3. 不同 profile 不该命中(防重按 code+profile 成对)
	if other, err := s.FindActiveResearchRun(ctx, code, "complete-stock"); err != nil || other != nil {
		t.Fatalf("other profile should miss: got=%v err=%v", other, err)
	}

	// 4. 收尾为 done → 不再算进行中(已完成的历史版本不该被复用)
	if err := s.FinishResearchRun(ctx, runID, "done", "", nil); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if got, err := s.FindActiveResearchRun(ctx, code, "short-term"); err != nil || got != nil {
		t.Fatalf("done should not be active: got=%v err=%v", got, err)
	}

	// 5. failed 同样不算
	runID2 := "test_sz999998_short-term_" + time.Now().Format("20060102150405.000001")
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID2, Code: code, Symbol: "sz999998", Profile: "short-term", AsOf: asOf,
	}); err != nil {
		t.Fatalf("create2: %v", err)
	}
	if err := s.FinishResearchRun(ctx, runID2, "failed", "采集失败: 测试", nil); err != nil {
		t.Fatalf("finish2: %v", err)
	}
	if got, err := s.FindActiveResearchRun(ctx, code, "short-term"); err != nil || got != nil {
		t.Fatalf("failed should not be active: got=%v err=%v", got, err)
	}
	t.Logf("FindActiveResearchRun ok: pending hit, done/failed/other-profile miss")
}
