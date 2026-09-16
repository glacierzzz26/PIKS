package research

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ==================== issue #8:历史研报作合成输入(纯函数,不连库不 exec Python) ====================

// TestAnnotatePriorRuns meta.prior_runs 注入(零 schema 标注"参考了几份历史")。
func TestAnnotatePriorRuns(t *testing.T) {
	in := json.RawMessage(`{"meta":{"as_of":"2026-09-15","symbol":"sh600519"},"price":{"pct_20d":3.2}}`)
	out := annotatePriorRuns(in, 2)

	var root map[string]json.RawMessage
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("注入后仍是合法 JSON: %v", err)
	}
	var meta struct {
		AsOf      string `json:"as_of"`
		PriorRuns int    `json:"prior_runs"`
	}
	if err := json.Unmarshal(root["meta"], &meta); err != nil {
		t.Fatalf("meta 应可解析: %v", err)
	}
	// 既有字段必须原样保留(只做加法)。
	if meta.AsOf != "2026-09-15" {
		t.Errorf("既有 meta.as_of 被破坏: %q", meta.AsOf)
	}
	if meta.PriorRuns != 2 {
		t.Errorf("prior_runs = %d, want 2", meta.PriorRuns)
	}
	// 顶层其它节也必须原样保留。
	if len(root["price"]) == 0 {
		t.Error("顶层 price 节丢失")
	}
}

// TestAnnotatePriorRunsNoop 无历史(n<=0)或 metrics 为空 → 原样返回(首次研报零回归)。
func TestAnnotatePriorRunsNoop(t *testing.T) {
	in := json.RawMessage(`{"meta":{"as_of":"2026-09-15"}}`)
	if got := annotatePriorRuns(in, 0); string(got) != string(in) {
		t.Errorf("n=0 应原样返回,实际: %s", got)
	}
	if got := annotatePriorRuns(nil, 2); len(got) != 0 {
		t.Errorf("空 metrics 应原样返回,实际: %s", got)
	}
	// 损坏 JSON 不应 panic,原样返回。
	bad := json.RawMessage(`{not json`)
	if got := annotatePriorRuns(bad, 1); string(got) != string(bad) {
		t.Errorf("损坏 JSON 应原样返回,实际: %s", got)
	}
}

// TestAnnotatePriorRunsNoMeta 无 meta 节时新建一个(不覆盖既有数据)。
func TestAnnotatePriorRunsNoMeta(t *testing.T) {
	out := annotatePriorRuns(json.RawMessage(`{"price":{"pct_20d":1.5}}`), 1)
	var root map[string]json.RawMessage
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("合法 JSON: %v", err)
	}
	if len(root["price"]) == 0 {
		t.Error("缺 meta 时不该动既有节")
	}
	var meta struct {
		PriorRuns int `json:"prior_runs"`
	}
	if err := json.Unmarshal(root["meta"], &meta); err != nil || meta.PriorRuns != 1 {
		t.Errorf("应新建 meta.prior_runs=1,实际 meta=%s err=%v", root["meta"], err)
	}
}

// TestPriorPromptBlock 历史摘要段:含硬约束、含各份 as_of、无历史时为空串。
func TestPriorPromptBlock(t *testing.T) {
	if got := priorPromptBlock(nil); got != "" {
		t.Errorf("无历史应返回空串(零回归),实际: %q", got)
	}

	block := priorPromptBlock([]priorSummary{
		{AsOf: "2026-09-15", Profile: "complete-stock",
			Score:     json.RawMessage(`{"overall":3,"overall_label":"偏正面"}`),
			Risk:      "中",
			Synthesis: json.RawMessage(`{"summary":"摘要A"}`)},
		{AsOf: "2026-09-12", Profile: "complete-stock", Risk: "低"},
	})
	for _, want := range []string{
		"2026-09-15", "2026-09-12", // 各份都出现
		"偏正面", "中", "摘要A", // 摘要字段被带上
		"不得", "必须", // 硬约束
	} {
		if !strings.Contains(block, want) {
			t.Errorf("摘要段应含 %q,实际:\n%s", want, block)
		}
	}
	// 关键约束:必须显式要求"只能引用本次指标卡数字"。
	if !strings.Contains(block, "本次") {
		t.Error("硬约束必须点名「本次」指标卡,否则 LLM 会拿历史值当当前值")
	}
}

// TestExtractScorecardAndRisk 从 metrics 摘字段;缺失/损坏一律返回空(不臆造)。
func TestExtractScorecardAndRisk(t *testing.T) {
	m := json.RawMessage(`{"scorecard":{"overall":2},"risk":{"overall_level":"高"}}`)
	if len(extractScorecard(m)) == 0 {
		t.Error("应摘到 scorecard")
	}
	if got := extractRiskLevel(m); got != "高" {
		t.Errorf("risk_level = %q, want 高", got)
	}
	// 缺字段 / 空 / 损坏 → 空(引用该份时只是少个字段,不报错)。
	if extractScorecard(json.RawMessage(`{"price":{}}`)) != nil {
		t.Error("无 scorecard 应返回 nil")
	}
	if got := extractRiskLevel(nil); got != "" {
		t.Errorf("空 metrics 风险等级应为空串,实际 %q", got)
	}
	if got := extractRiskLevel(json.RawMessage(`{bad`)); got != "" {
		t.Errorf("损坏 JSON 风险等级应为空串,实际 %q", got)
	}
}

// TestWritePriorMetrics 落 {code}_prior_metrics.json;无历史不落文件(保持逐字节一致)。
func TestWritePriorMetrics(t *testing.T) {
	dir := t.TempDir()
	// 无历史 → 不落文件
	p, err := writePriorMetrics(dir, "600519", nil)
	if err != nil || p != "" {
		t.Fatalf("无历史应返回空路径,实际 %q err=%v", p, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "600519_prior_metrics.json")); !os.IsNotExist(err) {
		t.Error("无历史不应落 prior_metrics 文件(首次研报产物须与改动前一致)")
	}

	// 有历史 → 落成数组,元素个数即引用份数
	metrics := []json.RawMessage{
		json.RawMessage(`{"meta":{"as_of":"2026-09-15"}}`),
		json.RawMessage(`{"meta":{"as_of":"2026-09-12"}}`),
	}
	p, err = writePriorMetrics(dir, "600519", metrics)
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(dir, "600519_prior_metrics.json") {
		t.Errorf("路径 = %q", p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil || len(arr) != 2 {
		t.Fatalf("应落成 2 元素 JSON 数组: err=%v len=%d", err, len(arr))
	}
}
