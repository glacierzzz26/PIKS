package research

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"piks/internal/ai"
	"piks/internal/store"
)

// issue #8 端到端:旧研报作合成输入,贯通到 fakeCLI 与落库标注。
//
// 验证三条:
//  1. PriorRuns 开 → 上一份 done 的 metrics 落成 {code}_prior_metrics.json,路径透传给 synthesize;
//  2. 落库 metrics.meta.prior_runs = 参考份数(零 schema 标注);
//  3. PriorRuns 关/无历史 → 不落 prior 文件、不标注(首次研报逐字节一致的零回归路径)。
//
// 与 store 集成测试同开关(PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL)。
func TestOrchestratorPriorRunsInput(t *testing.T) {
	requireIntegration(t)
	ctx := context.Background()
	pool, err := store.Open(ctx, os.Getenv("PIKS_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	t.Cleanup(func() { pool.Close() })

	code := "999995"
	stamp := time.Now().Format("20060102150405.000000")
	priorID := "prior-seed-" + stamp
	runID := "prior-run-" + stamp
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE code=$1`, code) })

	// 一份既往 done 研报:metrics 里带一个"历史才有的数字" 88.8,以及 scorecard/risk。
	priorMetrics := `{"meta":{"as_of":"2026-09-12"},"price":{"some_metric":88.8},` +
		`"scorecard":{"overall":2,"overall_label":"偏正面"},` +
		`"risk":{"overall_level":"中"}}`
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: priorID, Code: code, Symbol: "sh" + code, Profile: "complete-stock",
		AsOf: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), Status: StatusDone,
	}); err != nil {
		t.Fatal(err)
	}
	priorMD := "# 上次报告"
	if err := s.SaveResearchArtifacts(ctx, priorID, &store.ResearchRun{
		Metrics:   json.RawMessage(priorMetrics),
		Synthesis: json.RawMessage(`{"summary":"上次摘要","trend":"上次趋势","conclusion":"上次结论"}`),
		Markdown:  &priorMD,
	}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	cli := &fakeCLI{files: map[string]string{
		"run_meta.json": `{"run_id":"` + runID + `","symbol":"sh` + code + `","profile":"complete-stock",` +
			`"as_of":"2026-09-15","sections":["price"],"contract":1}`,
		"{code}_metrics.json": `{"meta":{"symbol":"sh` + code + `","as_of":"2026-09-15"},` +
			`"price":{"end_price":9.9}}`,
		"{code}_synthesis_prompt.txt": "你是一名 A 股研究分析师，正在撰写 " + code + " 的个股研究报告。\n指标卡见下…",
		"{code}_skeleton.md":          "# 骨架报告\n\n确定性结论",
	}}
	o := New(s, ai.NewMock(), 0)
	o.runner = cli

	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: code, Symbol: "sh" + code,
		Profile: "complete-stock", AsOf: time.Now(), Status: StatusPending,
	}); err != nil {
		t.Fatal(err)
	}

	// ---- 1. PriorRuns=2:应喂入 1 份历史(只有一份 done) ----
	res, err := o.Run(ctx, Options{RunID: runID, Code: code, OutDir: dir, PriorRuns: 2})
	if err != nil {
		t.Fatalf("编排报错: %v", err)
	}
	if res.Status != StatusDone {
		t.Fatalf("status = %s (err=%s), want done", res.Status, res.Error)
	}

	// prior_metrics.json 落盘,含 1 份历史 metrics
	priorPath := filepath.Join(dir, code+"_prior_metrics.json")
	b, err := os.ReadFile(priorPath)
	if err != nil {
		t.Fatalf("应落 prior_metrics 文件: %v", err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil || len(arr) != 1 {
		t.Fatalf("prior_metrics 应是 1 元素数组: len=%d err=%v", len(arr), err)
	}
	// 路径已透传给 synthesize(签名贯通)
	if cli.priorMetricsPath != priorPath {
		t.Errorf("synthesize 收到的 prior 路径 = %q, want %q", cli.priorMetricsPath, priorPath)
	}
	// 落库标注参考份数(零 schema)
	row, err := s.GetResearchRun(ctx, runID)
	if err != nil || row == nil {
		t.Fatalf("读回落库行: %v", err)
	}
	var meta struct {
		Meta struct {
			AsOf      string `json:"as_of"`
			PriorRuns int    `json:"prior_runs"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(row.Metrics, &meta); err != nil {
		t.Fatalf("落库 metrics 应可解析: %v", err)
	}
	if meta.Meta.PriorRuns != 1 {
		t.Errorf("metrics.meta.prior_runs = %d, want 1", meta.Meta.PriorRuns)
	}
	if meta.Meta.AsOf != "2026-09-15" {
		t.Errorf("本次 as_of 应被保留(只做加法),实际 %q", meta.Meta.AsOf)
	}

	// ---- 2. 零回归:PriorRuns=0 的新 run 不落 prior 文件、不标注 ----
	dir2 := t.TempDir()
	runID2 := "prior-off-" + stamp
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE run_id=$1`, runID2) })
	cli2 := &fakeCLI{files: map[string]string{
		"run_meta.json": `{"run_id":"` + runID2 + `","symbol":"sh` + code + `","profile":"complete-stock",` +
			`"as_of":"2026-09-15","sections":["price"],"contract":1}`,
		"{code}_metrics.json":         `{"meta":{"symbol":"sh` + code + `","as_of":"2026-09-15"},"price":{"end_price":9.9}}`,
		"{code}_synthesis_prompt.txt": "你是一名 A 股研究分析师，正在撰写 " + code + " 的个股研究报告。\n指标卡见下…",
		"{code}_skeleton.md":          "# 骨架报告\n\n确定性结论",
	}}
	o2 := New(s, ai.NewMock(), 0)
	o2.runner = cli2
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID2, Code: code, Symbol: "sh" + code,
		Profile: "complete-stock", AsOf: time.Now(), Status: StatusPending,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := o2.Run(ctx, Options{RunID: runID2, Code: code, OutDir: dir2}); err != nil {
		t.Fatalf("PriorRuns=0 编排报错: %v", err)
	}
	if cli2.priorMetricsPath != "" {
		t.Errorf("PriorRuns=0 不应传 prior 路径,实际 %q", cli2.priorMetricsPath)
	}
	if _, err := os.Stat(filepath.Join(dir2, code+"_prior_metrics.json")); !os.IsNotExist(err) {
		t.Error("PriorRuns=0 不应落 prior_metrics 文件")
	}
	row2, _ := s.GetResearchRun(ctx, runID2)
	var meta2 struct {
		Meta struct {
			PriorRuns *int `json:"prior_runs"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(row2.Metrics, &meta2); err != nil {
		t.Fatal(err)
	}
	if meta2.Meta.PriorRuns != nil {
		t.Errorf("PriorRuns=0 不应标注 prior_runs,实际 %d", *meta2.Meta.PriorRuns)
	}
	t.Logf("prior wiring ok: on(prior_runs=1,file+passthrough) off(no file,no annotate)")
}

// requireIntegration 集成测试双守卫(与既有 *_integration_test.go 同约定)。
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set (integration off by default)")
	}
	if os.Getenv("PIKS_DATABASE_URL") == "" {
		t.Skip("PIKS_DATABASE_URL not set")
	}
}
