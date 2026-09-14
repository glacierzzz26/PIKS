package research

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"piks/internal/ai"
	"piks/internal/store"
)

// TestOrchestratorStateMachine 完整编排状态机(§5.6 最小版本测试):
// 注入 fakeCLI(固定产物 fixture)+ mock provider,跑 pending→gathering→
// synthesizing→verifying→done,**不 exec Python**。
//
// 需要数据库(写 research_runs/task_runs),故与 store 集成测试同开关:
//
//	PIKS_TEST_INTEGRATION=1 PIKS_DATABASE_URL=... go test ./internal/research/...
func TestOrchestratorStateMachine(t *testing.T) {
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
	// LIFO 清理:先注册关池(最后跑),再注册删行(先跑)。
	t.Cleanup(func() { pool.Close() })

	dir := t.TempDir()
	runID := "fixture-test-" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE run_id=$1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM task_runs WHERE command LIKE 'research-run:%' AND created_at > now() - interval '1 minute'`)
	})

	// 固定产物:run_meta + metrics + prompt + skeleton(采集步的产出)。
	cli := &fakeCLI{files: map[string]string{
		"run_meta.json": `{"run_id":"` + runID + `","symbol":"sz000560","profile":"complete-stock",` +
			`"as_of":"2026-09-12","sections":["price"],"contract":1}`,
		"{code}_metrics.json": `{"meta":{"symbol":"sz000560","as_of":"2026-09-12"},` +
			`"price":{"end_price":2.74,"period_return_pct":26.2},` +
			`"evidence":[{"id":"e1","type":"fact","tier":"structured","section":"price",` +
			`"statement":"period_return_pct = 26.2"}]}`,
		// 与 research 真实合成提示同前缀(ai.Mock 凭 "A 股研究分析师" 命中深研分支)。
		"{code}_synthesis_prompt.txt": "你是一名 A 股研究分析师，正在撰写 000560 的个股研究报告。\n指标卡见下…",
		"{code}_skeleton.md":          "# 骨架报告\n\n确定性结论",
	}}

	o := New(s, ai.NewMock(), 0)
	o.runner = cli // 注入假 CLI:不 exec Python(独立迭代测试的关键)

	// 先建行(状态机要求 run 已存在,RunID 非空即续跑它)。
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: "000560", Symbol: "sz000560",
		Profile: "complete-stock", AsOf: time.Now(), Status: StatusPending,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := o.Run(ctx, Options{RunID: runID, Code: "000560", OutDir: dir})
	if err != nil {
		t.Fatalf("编排报错(应自行收口为 status): %v", err)
	}
	if res.Status != StatusDone {
		t.Fatalf("status = %s, want done(error=%s)", res.Status, res.Error)
	}
	if !res.Lint || !res.Gate {
		t.Errorf("lint=%v gate=%v, fixture 均为通过,应都 true", res.Lint, res.Gate)
	}
	if res.Model != "mock" {
		t.Errorf("model = %q, want mock(provider.Name())", res.Model)
	}

	// 落库校验:各阶段产物齐全。
	row, err := s.GetResearchRun(ctx, runID)
	if err != nil || row == nil {
		t.Fatalf("读回落库行失败: %v", err)
	}
	if row.Status != StatusDone {
		t.Errorf("落库 status = %s, want done", row.Status)
	}
	if len(row.Metrics) == 0 || len(row.Synthesis) == 0 || row.Markdown == nil {
		t.Error("metrics/synthesis/markdown 应齐全")
	}
	if len(row.Lint) == 0 || len(row.Gate) == 0 {
		t.Error("lint/gate 应齐全")
	}
	// as_of 以指标卡为准回写(防未来函数基准)。
	if got := row.AsOf.Format("2006-01-02"); got != "2026-09-12" {
		t.Errorf("as_of = %s, want 2026-09-12(应以指标卡为准回写)", got)
	}
	// 记账:三条 task_runs(gather/synth/verify)。
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM task_runs WHERE command LIKE 'research-run:%' AND created_at > now() - interval '1 minute'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 3 {
		t.Errorf("task_runs 记录 = %d, want ≥3(gather/synth/verify 各一条)", n)
	}
}

// TestOrchestratorGatherFailure 采集失败 → status=failed + error 原文,不编造数据。
func TestOrchestratorGatherFailure(t *testing.T) {
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set")
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
	t.Cleanup(func() { pool.Close() })

	dir := t.TempDir()
	runID := "fixture-fail-" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE run_id=$1`, runID) })

	o := New(s, ai.NewMock(), 0)
	o.runner = &fakeCLI{gatherErr: errGatherBoom}

	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: "000560", Symbol: "sz000560",
		Profile: "complete-stock", AsOf: time.Now(), Status: StatusPending,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := o.Run(ctx, Options{RunID: runID, Code: "000560", OutDir: dir})
	if err != nil {
		t.Fatalf("编排应自行收口,却直接报错: %v", err)
	}
	if res.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", res.Status)
	}
	row, _ := s.GetResearchRun(ctx, runID)
	if row == nil || row.Status != StatusFailed || row.Error == nil {
		t.Fatalf("失败应如实落库(status=failed + error),实际: %+v", row)
	}
	if len(row.Metrics) > 2 { // 默认 '{}'
		t.Error("采集失败不应留下编造的 metrics")
	}
	finalMD := filepath.Join(dir, "000560_final.md")
	if _, err := os.Stat(finalMD); err == nil {
		t.Error("采集失败不应产出 final.md")
	}
}

type gatherBoom struct{}

func (gatherBoom) Error() string { return "akshare 连接超时(注入)" }

var errGatherBoom = gatherBoom{}

// TestOrchestratorQuickNoProvider 快速模式(RequireSynthesis=false)+ 无 AI provider:
// 合成步不 fail,以骨架报告收口 → status=done,synthesis 空、markdown=骨架。
// 对照深研(RequireSynthesis=true)同条件应 failed。
func TestOrchestratorQuickNoProvider(t *testing.T) {
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
	t.Cleanup(func() { pool.Close() })

	dir := t.TempDir()
	runID := "fixture-quick-" + time.Now().Format("20060102150405.000000")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE run_id=$1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM task_runs WHERE command LIKE 'research-run:%' AND created_at > now() - interval '1 minute'`)
	})

	cli := &fakeCLI{files: map[string]string{
		"run_meta.json": `{"run_id":"` + runID + `","symbol":"sz000560","profile":"prebuy",` +
			`"as_of":"2026-09-12","sections":["price"],"contract":1}`,
		"{code}_metrics.json": `{"meta":{"symbol":"sz000560","as_of":"2026-09-12"},` +
			`"price":{"end_price":2.74},` +
			`"patterns":{"series":[{"date":"2026-09-12","close":2.74,"turnover":1.0,"volume":100}],"labels":[],"divergence":{}}}`,
		"{code}_synthesis_prompt.txt": "你是一名 A 股研究分析师…",
		"{code}_skeleton.md":          "# 骨架报告\n\n确定性结论",
	}}

	// provider 为 nil:深研语义下合成必失败;快速模式应降级。
	o := New(s, nil, 0)
	o.runner = cli

	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: "000560", Symbol: "sz000560",
		Profile: "prebuy", AsOf: time.Now(), Status: StatusPending,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := o.Run(ctx, Options{RunID: runID, Code: "000560", OutDir: dir, RequireSynthesis: false})
	if err != nil {
		t.Fatalf("编排应自行收口: %v", err)
	}
	if res.Status != StatusDone {
		t.Fatalf("快速模式无 provider 应 done,实际 status=%s error=%s", res.Status, res.Error)
	}
	row, _ := s.GetResearchRun(ctx, runID)
	if row == nil || row.Status != StatusDone {
		t.Fatalf("应如实落 done,实际: %+v", row)
	}
	if row.Markdown == nil || len(*row.Markdown) == 0 {
		t.Error("应有骨架 markdown")
	}
	if len(row.Metrics) == 0 {
		t.Error("确定性指标卡应落库(不受合成降级影响)")
	}
}
