package research

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ==================== 产物契约(§4.10 G2 / §5.6 最小版本测试) ====================
//
// 这些用例只用「固定产物 fixture」验证 Go 侧的契约读取与版本保护,
// 不 exec Python、不连数据库 —— CI 上 `go test ./internal/research/...` 直接可跑。
// 覆盖:
//   1. contract:1 → 正常打开(降版/同版兼容)
//   2. contract:2 → 明确报错,不崩溃、不硬解(升版保护)
//   3. run_meta.json 缺失/损坏 → 如实报错
//   4. 文件名契约:五类产物路径与 research/README.md 冻结表一致
//   5. lint/gate 的 passed 语义(缺失不当作通过)

// writeRunMeta 在 dir 落一份 run_meta.json,contract 可调。
func writeRunMeta(t *testing.T, dir string, contract int) {
	t.Helper()
	meta := map[string]any{
		"run_id":         "sz000560_short-term_20260912_120000",
		"symbol":         "sz000560",
		"profile":        "short-term",
		"mode":           "complete-stock",
		"as_of":          "2026-09-12",
		"sections":       []string{"price", "volume", "events"},
		"provider_calls": map[string]int{"akshare": 3},
		"contract":       contract,
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "run_meta.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenArtifactsContractSameVersion(t *testing.T) {
	dir := t.TempDir()
	writeRunMeta(t, dir, 1)

	a, err := openArtifacts(dir, "000560")
	if err != nil {
		t.Fatalf("contract:1 应可打开,却报错: %v", err)
	}
	if a.symbol != "sz000560" {
		t.Errorf("symbol = %q, want sz000560(应从 run_meta 回填)", a.symbol)
	}
	if a.meta.Contract != 1 {
		t.Errorf("Contract = %d, want 1", a.meta.Contract)
	}
}

// TestOpenArtifactsContractMissing 缺 contract 字段(旧产物/上游忘写)按 0 处理:
// 0 ≤ 本端支持版本 → 放行(「只做加法」契约下缺失即最老版本,不应拒绝)。
func TestOpenArtifactsContractMissing(t *testing.T) {
	dir := t.TempDir()
	meta := `{"run_id":"r1","symbol":"sz000560","as_of":"2026-09-12"}`
	if err := os.WriteFile(filepath.Join(dir, "run_meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openArtifacts(dir, "000560"); err != nil {
		t.Fatalf("缺 contract 字段应放行(视作最老版本),却报错: %v", err)
	}
}

// TestOpenArtifactsContractTooNew 升版保护(§5.6 硬验收):
// research 侧 contract 高于本端支持 → 明确报错,不崩溃不硬解。
func TestOpenArtifactsContractTooNew(t *testing.T) {
	dir := t.TempDir()
	writeRunMeta(t, dir, 2)

	_, err := openArtifacts(dir, "000560")
	if err == nil {
		t.Fatal("contract:2 应报错(本端仅支持 v1),却放行了")
	}
	msg := err.Error()
	if !strings.Contains(msg, "v2") || !strings.Contains(msg, "v1") {
		t.Errorf("错误信息应同时点明产物版本与本端版本,实际: %s", msg)
	}
	if !strings.Contains(msg, "升级 PIKS") {
		t.Errorf("应给出可操作指引(升级 PIKS),实际: %s", msg)
	}
}

// TestOpenArtifactsMissingMeta run_meta.json 不存在 → 如实报错(采集未产出)。
func TestOpenArtifactsMissingMeta(t *testing.T) {
	dir := t.TempDir()
	_, err := openArtifacts(dir, "000560")
	if err == nil {
		t.Fatal("run_meta.json 缺失应报错,却放行了")
	}
	if !strings.Contains(err.Error(), "run_meta.json") {
		t.Errorf("错误应点名缺失文件,实际: %s", err.Error())
	}
}

// TestOpenArtifactsCorruptMeta 损坏 JSON → 报错而非 panic。
func TestOpenArtifactsCorruptMeta(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run_meta.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openArtifacts(dir, "000560"); err == nil {
		t.Fatal("损坏的 run_meta.json 应报错,却放行了")
	}
}

// TestArtifactPathContract 文件名契约冻结(§5.6「产物契约冻结」):
// Go 侧拼出的路径必须与 research/README.md 的契约表逐一对应。
func TestArtifactPathContract(t *testing.T) {
	a := &artifacts{dir: "/d", code: "000560"}
	cases := []struct {
		got, want string
	}{
		{a.promptPath(), "/d/000560_synthesis_prompt.txt"},
		{a.metricsPath(), "/d/000560_metrics.json"},
		{a.skeletonPath(), "/d/000560_skeleton.md"},
		{a.finalPath(), "/d/000560_final.md"},
		{a.lintPath(), "/d/000560_lint.json"},
		{a.synthPath(), "/d/000560_synthesis.json"},
		{a.gatePath(), "/d/000560_gate.json"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("产物路径 = %q, want %q(契约冻结,改名即破坏独立迭代)", c.got, c.want)
		}
	}
}

// TestExistsIgnoresEmptyFile 空文件不算已产出(断点重跑判据):避免半截文件被当成果。
func TestExistsIgnoresEmptyFile(t *testing.T) {
	dir := t.TempDir()
	a := &artifacts{dir: dir, code: "000560"}
	if a.exists(a.metricsPath()) {
		t.Error("文件不存在时 exists 应为 false")
	}
	if err := os.WriteFile(a.metricsPath(), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if a.exists(a.metricsPath()) {
		t.Error("空文件不应视为已产出(否则半截产物会被当作可复用)")
	}
	if err := os.WriteFile(a.metricsPath(), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !a.exists(a.metricsPath()) {
		t.Error("非空文件应视为已产出")
	}
}

// ==================== 机检 passed 语义 ====================

// TestJSONPassed 缺失/损坏一律不当作通过(不可判定 ≠ 通过,防「静默放行」)。
func TestJSONPassed(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"passed true", `{"passed":true}`, true},
		{"passed false", `{"passed":false}`, false},
		{"缺字段", `{"scanned":10}`, false},
		{"空输入", ``, false},
		{"损坏", `{oops`, false},
	}
	for _, c := range cases {
		if got := JSONPassed(json.RawMessage(c.in)); got != c.want {
			t.Errorf("%s: JSONPassed(%q) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

// TestLintFailed 只有「有 lint 产物且 passed=false」才算未过;
// 无 lint 产物(len==0)不算未过 —— 此时 markdown 不回落骨架(交给后续步骤报错)。
func TestLintFailed(t *testing.T) {
	if !lintFailed(json.RawMessage(`{"passed":false}`)) {
		t.Error("passed:false 应判为未过")
	}
	if lintFailed(json.RawMessage(`{"passed":true}`)) {
		t.Error("passed:true 不应判为未过")
	}
	if lintFailed(nil) {
		t.Error("无 lint 产物不应判为未过(未执行 ≠ 未通过)")
	}
}

// TestToFullCode 交易所前缀映射(与 research/src/models/symbol.py 同规则)。
func TestToFullCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"600519", "sh600519"},
		{"688981", "sh688981"},
		{"000560", "sz000560"},
		{"300750", "sz300750"},
		{"430047", "bj430047"},
		{"sz000560", "sz000560"}, // 已带前缀不重复加
	}
	for _, c := range cases {
		if got := ToFullCode(c.in); got != c.want {
			t.Errorf("ToFullCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ==================== 编排状态机(注入假 CLI,不依赖 Python/DB) ====================

// fakeCLI 用固定产物模拟三步 CLI 的产出,验证编排按契约读产物。
// files: 键为产物文件名,值为内容;写入 opt 的产物目录。
type fakeCLI struct {
	files map[string]string
	// gatherErr 非空则 gather 返回该错误(模拟采集失败)。
	gatherErr error
	// priorMetricsPath 记录 synthesize 收到的 --prior-metrics 路径(验证签名贯通)。
	priorMetricsPath string
}

func (f *fakeCLI) materialize(dir, code string) {
	for name, body := range f.files {
		// fixture 里的 "{code}" 占位替换成真实代码(文件名含代码)。
		n := strings.ReplaceAll(name, "{code}", code)
		_ = os.WriteFile(filepath.Join(dir, n), []byte(body), 0o644)
	}
}

func (f *fakeCLI) gather(_ context.Context, code, _ string, _ int, outDir, _ string) (string, error) {
	if f.gatherErr != nil {
		return "", f.gatherErr
	}
	f.materialize(outDir, code)
	return "ok", nil
}

func (f *fakeCLI) synthesize(_ context.Context, dir, code, synthFile, priorMetricsPath string) (string, error) {
	// 模拟 research:读 Go 写好的 synthesis.json,渲染 final.md + lint.json。
	raw, err := os.ReadFile(synthFile)
	if err != nil {
		return "", err
	}
	// 记录 priorMetricsPath 供用例断言(夹具不真做 lint,只验证签名贯通与传参)。
	f.priorMetricsPath = priorMetricsPath
	_ = os.WriteFile(filepath.Join(dir, code+"_final.md"), append([]byte("# 报告\n\n"), raw...), 0o644)
	_ = os.WriteFile(filepath.Join(dir, code+"_lint.json"), []byte(`{"scanned":9,"matched":9,"ignored":0,"passed":true,"issues":[]}`), 0o644)
	return "ok", nil
}

func (f *fakeCLI) gate(_ context.Context, dir, code string) (string, error) {
	_ = os.WriteFile(filepath.Join(dir, code+"_gate.json"),
		[]byte(`{"passed":true,"checks":[{"name":"data_completeness","passed":true,"detail":"ok"}]}`), 0o644)
	return "ok", nil
}
