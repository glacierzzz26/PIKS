package cluster

import (
	"reflect"
	"testing"
)

// 跨源数值冲突检测(issue #49 / T3)。
//
// 下列用例分三类,**都对应 docs/phase11/design/event-cross-source-conflict.md §4 的实测标定**:
//   - 真阳性:同一事件、同一量、不同数 → 必须报;
//   - 真阴性(对抗集):不同动作/不同年份/不同指标 → 必须**不**报(误报的主要来源);
//   - 改写不变性:同事件、同数、措辞各异 → 必须**不**报(不能把改写当冲突)。

// ── 真阳性:同一事件同一量的数值分叉 ──────────────────────────────────────
//
// 这三条是跨源转载的**常态**:各家换个动词/补个机构名,数字照抄 → 骨架 J=1.000。
// 也是本规则唯一稳定覆盖的一类(见 TestDetectFactConflictsKnownRecallBoundary)。

func TestDetectFactConflictsTruePositive(t *testing.T) {
	cases := []struct {
		name   string
		a, b   string
		unit   string
		values []float64
	}{
		{
			"减持比例 3% vs 2%",
			"公司股东拟减持不超过3%的股份",
			"公司股东拟减持不超过2%的股份",
			"%", []float64{2, 3},
		},
		{
			"释放资金 5000亿 vs 3000亿",
			"预计释放长期资金约5000亿元",
			"预计释放长期资金约3000亿元",
			"亿元", []float64{3000, 5000},
		},
		{
			"降准幅度 0.25 vs 0.5 个百分点",
			"本次降准0.25个百分点",
			"本次降准0.5个百分点",
			"个百分点", []float64{0.25, 0.5},
		},
	}
	for _, c := range cases {
		got := DetectFactConflicts([]string{c.a}, []string{c.b})
		if len(got) != 1 {
			t.Errorf("%s: 应检出 1 条冲突, got %d (%v)", c.name, len(got), got)
			continue
		}
		if got[0].Unit != c.unit {
			t.Errorf("%s: 单位 = %q, want %q", c.name, got[0].Unit, c.unit)
		}
		if !reflect.DeepEqual(got[0].Values, c.values) {
			t.Errorf("%s: 值 = %v, want %v", c.name, got[0].Values, c.values)
		}
		// 红线:双方原文都必须在,否则前端无法「留双源原文」。
		if got[0].SentenceA != c.a || got[0].SentenceB != c.b {
			t.Errorf("%s: 必须带双方原文, got A=%q B=%q", c.name, got[0].SentenceA, got[0].SentenceB)
		}
	}
}

// ── 真阴性 · 对抗集:骨架门控必须挡住的误报源 ────────────────────────────
//
// 这四类是「同单位 + 骨架接近」最容易误报的组合,实测骨架 Jaccard:
// 回购vs增持 0.400、2026vs2025年 0.500、产能利用率vs毛利率 0.143、同比vs环比 0.500 ——
// 全部低于门控 0.6。若门控被调低,它们会立刻变成误报(见 §4 门控扫描表)。

func TestDetectFactConflictsAdversarialNegative(t *testing.T) {
	cases := []struct{ name, a, b string }{
		{"不同动作:回购 vs 增持", "公司拟回购不超过5000万股", "公司拟增持不超过3000万股"},
		{"不同年份:2026 vs 2025", "2026年营收5000亿元", "2025年营收3000亿元"},
		{"不同指标:产能利用率 vs 毛利率", "产能利用率达90%", "毛利率达30%"},
		{"不同口径:同比 vs 环比", "同比增长40%", "环比增长30%"},
	}
	for _, c := range cases {
		if got := DetectFactConflicts([]string{c.a}, []string{c.b}); len(got) != 0 {
			t.Errorf("%s: 不得报冲突(误报), got %v", c.name, got)
		}
	}
}

// ── 已知召回边界(刻意留档,勿当 bug 修)────────────────────────────────
//
// 门控取 0.6 是「误报归零」与「保住常态真阳性」的交点,代价是**远改写的同量异值漏检**:
// 改写幅度大的同量句(骨架 J≈0.47~0.50)与「不同年份营收」「回购 vs 增持」等误报
// **同处一个相似度区间**,纯骨架比对无法区分 —— 这是确定性方案的原理性上限,不是实现缺陷。
//
// 本用例把该边界**钉死**:若哪天有人调低门控想去捞这两条,这里会立刻红,提醒他
// 同时会引入对抗集的误报(门控 0.5 时对抗误报 2/4)。要覆盖远改写需 LLM 语义比对
// (网关可用后再议,见 issue #45)。
func TestDetectFactConflictsKnownRecallBoundary(t *testing.T) {
	cases := []struct{ name, a, b string }{
		{"减持·远改写", "公司股东计划减持不超过3%的股份", "股东拟减持公司不超过2%的股份"},
		{"释放资金·远改写", "预计可释放长期资金约5000亿元", "此举预计释放长期资金3000亿元"},
	}
	for _, c := range cases {
		if got := DetectFactConflicts([]string{c.a}, []string{c.b}); len(got) != 0 {
			t.Errorf("%s: 该例**预期漏检**(远改写,骨架 J≈0.5);若已被检出说明门控被调低,"+
				"须同步复验对抗集(门控 0.5 时误报 2/4):got %v", c.name, got)
		}
	}
}

// 近同改写(骨架 J=1.0)必须稳报 —— 这是判据的另一半:门控不能严到把常态也漏掉。
func TestDetectFactConflictsNearParaphraseStillTruePositive(t *testing.T) {
	cases := []struct{ name, a, b string }{
		{"减持·换动词", "公司股东拟减持不超过3%的股份", "公司股东计划减持不超过2%的股份"},
		{"释放资金·换措辞", "预计释放长期资金约5000亿元", "预计将释放长期资金约3000亿元"},
	}
	for _, c := range cases {
		if got := DetectFactConflicts([]string{c.a}, []string{c.b}); len(got) != 1 {
			t.Errorf("%s: 近同改写应稳报 1 条, got %v", c.name, got)
		}
	}
}

// ── 真阴性 · 改写不变性:同事件同数、措辞各异不得报 ──────────────────────
//
// 这是「单一来源/多源」场景的常态:各家转载同一件事、数字一致而措辞不同。
// 规则若对这些报冲突,功能即为噪音源。

func TestDetectFactConflictsParaphraseNoFalseAlarm(t *testing.T) {
	cases := []struct{ name, a, b string }{
		{"动词同义", "公司股东拟减持不超过3%的股份", "公司股东计划减持不超过3%的股份"},
		{"主动→被动", "公司股东拟减持不超过3%的股份", "公司股东拟被减持不超过3%的股份"},
		{"补充机构名与措辞", "央行表示此举旨在保持流动性合理充裕、加大对实体经济支持力度",
			"央行有关负责人表示，此举旨在保持流动性合理充裕，加大对实体经济的支持力度。"},
		{"全称 vs 简称同数", "中国人民银行决定自2026年9月1日起下调金融机构存款准备金率0.25个百分点",
			"央行宣布下调金融机构存款准备金率0.25个百分点"},
	}
	for _, c := range cases {
		if got := DetectFactConflicts([]string{c.a}, []string{c.b}); len(got) != 0 {
			t.Errorf("%s: 同数改写不得报冲突, got %v", c.name, got)
		}
	}
}

// ── 边界:单侧多值(并列口径)不报 ─────────────────────────────────────────
//
// 同单位一句里出现两个数,通常是「同比 vs 环比」并列,不是分歧 —— 宁漏不误。

func TestDetectFactConflictsMultiValueSkips(t *testing.T) {
	a := "同比增长40%，环比增长30%"
	b := "同比增长40%"
	if got := DetectFactConflicts([]string{a}, []string{b}); len(got) != 0 {
		t.Fatalf("A 侧同单位多值应跳过(不报), got %v", got)
	}
}

// ── 边界:空输入 / 无单位句 / 单位不匹配 ────────────────────────────────

func TestDetectFactConflictsEdgeCases(t *testing.T) {
	if got := DetectFactConflicts(nil, nil); got != nil {
		t.Errorf("空输入应返回 nil, got %v", got)
	}
	if got := DetectFactConflicts([]string{""}, []string{""}); len(got) != 0 {
		t.Errorf("空串不得报冲突, got %v", got)
	}
	// 无白名单单位的数字(裸数字/日期)不参与比对。
	if got := DetectFactConflicts(
		[]string{"公司于2026年9月20日发布公告"},
		[]string{"公司于2027年9月20日发布公告"}); len(got) != 0 {
		t.Errorf("裸年份不得被当成数值冲突, got %v", got)
	}
	// 同单位不同值但骨架差太远 → 门控挡下。
	if got := DetectFactConflicts(
		[]string{"产能利用率达90%"},
		[]string{"毛利率达30%"}); len(got) != 0 {
		t.Errorf("骨架过远不得报冲突, got %v", got)
	}
}

// 单位白名单必须做最长匹配:「个百分点」不得被截成「百分点」或「个」,
// 「亿元」不得被截成「元」——否则同量会因单位归属不同而被判成两回事。
func TestFactNumbersUnitLongestMatch(t *testing.T) {
	got := factNumbers("本次降准0.25个百分点")
	if _, ok := got["个百分点"]; !ok {
		t.Fatalf("应抽出单位「个百分点」, got %v", got)
	}
	if _, ok := got["百分点"]; ok {
		t.Fatalf("不得同时抽出「百分点」(重复归属), got %v", got)
	}
	got2 := factNumbers("释放资金约5000亿元")
	if _, ok := got2["亿元"]; !ok {
		t.Fatalf("应抽出单位「亿元」, got %v", got2)
	}
	if _, ok := got2["元"]; ok {
		t.Fatalf("不得同时抽出「元」, got %v", got2)
	}
}
