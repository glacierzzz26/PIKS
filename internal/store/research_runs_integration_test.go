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

// TestListPriorDoneResearchRuns issue #8 的查询:同 code 既往 done 报告,as_of DESC。
// 关键语义:只取 done(失败/半成品不能当"上次怎么看"喂给 LLM)、排除本次 run、limit 生效。
func TestListPriorDoneResearchRuns(t *testing.T) {
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

	// 专用 code,与真实数据隔离;用时间戳保证可重复运行。
	code := "999997"
	stamp := time.Now().Format("20060102150405.000000")
	ids := []string{
		"test_" + code + "_" + stamp + "_a", // 最新 done
		"test_" + code + "_" + stamp + "_b", // 次新 done
		"test_" + code + "_" + stamp + "_c", // 失败(不该被取)
	}
	t.Cleanup(func() { pool.Close() })
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE code=$1`, code)
	})

	rows := []struct {
		runID  string
		asOf   time.Time
		status string
	}{
		{ids[0], time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), "done"},
		{ids[1], time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), "done"},
		{ids[2], time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), "failed"},
	}
	for _, r := range rows {
		if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
			RunID: r.runID, Code: code, Symbol: "sh" + code,
			Profile: "complete-stock", AsOf: r.asOf, Status: r.status,
		}); err != nil {
			t.Fatalf("seed %s: %v", r.runID, err)
		}
	}

	// 1. 只取 done 且 as_of DESC:最新在前,失败记录不在结果里
	got, err := s.ListPriorDoneResearchRuns(ctx, code, "", 0)
	if err != nil {
		t.Fatalf("list prior: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("应取到 2 份 done(失败的不算),实际 %d 份", len(got))
	}
	if got[0].RunID != ids[0] || got[1].RunID != ids[1] {
		t.Fatalf("应按 as_of DESC 排序,实际: %s, %s", got[0].RunID, got[1].RunID)
	}

	// 2. exclude 生效:排掉最新一份
	got2, err := s.ListPriorDoneResearchRuns(ctx, code, ids[0], 0)
	if err != nil {
		t.Fatalf("list prior exclude: %v", err)
	}
	if len(got2) != 1 || got2[0].RunID != ids[1] {
		t.Fatalf("exclude 未生效,实际 %d 份", len(got2))
	}

	// 3. limit 生效
	got3, err := s.ListPriorDoneResearchRuns(ctx, code, "", 1)
	if err != nil {
		t.Fatalf("list prior limit: %v", err)
	}
	if len(got3) != 1 || got3[0].RunID != ids[0] {
		t.Fatalf("limit 未生效,实际 %d 份", len(got3))
	}

	// 4. 无历史 → 空(不报错),首次研报路径
	got4, err := s.ListPriorDoneResearchRuns(ctx, "999996", "", 0)
	if err != nil {
		t.Fatalf("无历史应返回空而非报错: %v", err)
	}
	if len(got4) != 0 {
		t.Fatalf("无历史应得 0 份,实际 %d", len(got4))
	}
	t.Logf("ListPriorDoneResearchRuns ok: done=%d exclude=%d limit=%d empty=%d",
		len(got), len(got2), len(got3), len(got4))
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
