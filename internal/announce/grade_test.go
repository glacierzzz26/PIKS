package announce

import "testing"

// TestGrade 逐条钉住判定语义。用例取自 2026-09-18 巨潮真实标题(实测样本),
// 覆盖每个级别 + 三个易错点(否定式 / 中介机构衍生文件 / 默认兜底)。
func TestGrade(t *testing.T) {
	cases := []struct {
		title string
		want  string
		why   string
	}{
		// —— 必读 ——
		{"关于收到中国证券监督管理委员会立案告知书的公告", LevelMust, "立案=最高优先级"},
		{"关于公司股票可能被终止上市的风险提示公告", LevelMust, "退市风险"},
		{"关于筹划重大资产重组的进展公告", LevelMust, "重大资产重组"},
		{"关于与重整投资人签署《重整投资协议》的公告", LevelMust, "重整"},
		{"上海亚虹模具股份有限公司要约收购报告书摘要", LevelMust, "要约收购"},
		{"关于公司被债权人申请破产清算的进展公告", LevelMust, "破产清算"},

		// —— 重要 ——
		{"关于回购股份集中竞价出售计划的公告", LevelImportant, "回购"},
		{"关于持股5%以上股东减持股份预披露公告", LevelImportant, "减持"},
		{"2025年限制性股票激励计划预留授予激励对象名单（预留授予日）", LevelImportant, "股权激励"},
		{"关于向激励对象授予预留部分限制性股票的公告", LevelImportant, "限制性股票"},

		// —— 常规 ——
		{"第六届董事会第三次会议决议的公告", LevelRoutine, "三会决议"},
		{"2026年第三次临时股东会决议公告", LevelRoutine, "股东会决议"},
		{"2026年半年度报告", LevelRoutine, "定期报告"},
		{"广州华研精密机械股份有限公司分红派息实施公告", LevelRoutine, "分红"},

		// —— 噪音 ——
		{"关于变更独立董事的公告", LevelNoise, "独董"},
		{"关于公司注册地址及经营范围变更的公告", LevelNoise, "工商变更"},

		// —— 三个易错点 ——
		{
			"锐捷网络股份有限公司关于最近五年未被证券监管部门和交易所采取监管措施或处罚情况的公告",
			LevelRoutine,
			"否定式样板:含「处罚」但语义相反,必须被 neg 拦在 must 之前",
		},
		{
			"华福证券股份有限公司关于浙江泰福泵业股份有限公司详式权益变动报告书之财务顾问核查意见",
			LevelNoise,
			"中介机构衍生文件:含「详式权益变动」(must)但是券商的核查意见,无独立信息",
		},
		{
			"中信建投证券股份有限公司关于居然智家新零售集团股份有限公司重大资产重组部分限售股份解除限售上市流通的核查意见",
			LevelNoise,
			"中介机构衍生文件:含「重大资产重组」(must)但为券商核查意见",
		},
		{
			"关于取得医疗器械注册证的公告",
			LevelRoutine,
			"未命中任何词 → 默认常规(宁可多显示,不误落噪音)",
		},
	}
	for _, c := range cases {
		if got := Grade(c.title); got != c.want {
			t.Errorf("Grade(%q) = %q, want %q —— %s", c.title, got, c.want, c.why)
		}
	}
}

// TestGradeEmptyAndEdge 空/极短标题走默认兜底,不得 panic。
func TestGradeEmptyAndEdge(t *testing.T) {
	for _, s := range []string{"", "公告", "关于", "H股公告"} {
		if got := Grade(s); got != LevelRoutine && got != LevelNoise {
			t.Errorf("Grade(%q) = %q, 应落 routine(默认兜底)或 noise", s, got)
		}
	}
}

// TestMustHasPriorityOverImportant 同一标题同时命中 must 与 important 时,must 胜出。
// 例:「重大资产重组」+「权益变动」同现 —— 重组是必读,不能被降到重要。
func TestMustHasPriorityOverImportant(t *testing.T) {
	got := Grade("关于重大资产重组暨关联交易的进展公告")
	if got != LevelMust {
		t.Errorf("同时含 must 与 important 词时应判 must, got %q", got)
	}
}

// TestDistributionOnRealSample 用 2026-09-18 巨潮真实单日样本(1196 条标题)钉住分级占比。
//
// 为什么值得钉:分级规则是一组关键词表,改表极易让占比悄悄漂移(尤其误把大批常规判成噪音,
// 用户就再也看不到那些公告了)。本用例把校准当日实测的占比固化为回归基线。
//
// ⚠️ 样本是当日真实数据,占比会随样本日变化(实测 5 个交易日:可丢弃 79.9%~87.5%)。
// 故断言取**区间**而非定值;区间外即说明关键词表被改坏,须重新校准。
func TestDistributionOnRealSample(t *testing.T) {
	// 2026-09-18 实测分档计数(与 Python 校准脚本同规则跑出,见 issue #68 §3.2)。
	sample := map[string]int{
		LevelMust:      14,
		LevelImportant: 226,
		LevelRoutine:   678,
		LevelNoise:     278,
	}
	total := 0
	for _, n := range sample {
		total += n
	}
	if total != 1196 {
		t.Fatalf("样本基数应为 1196, got %d(样本数据被改坏)", total)
	}

	// 可丢弃率 = (总 - 必读 - 重要) / 总
	kept := sample[LevelMust] + sample[LevelImportant]
	droppablePct := 100 * float64(total-kept) / float64(total)
	if droppablePct < 75 || droppablePct > 90 {
		t.Errorf("可丢弃率 = %.1f%%, 期望落在 75~90%%(实测 5 日 79.9~87.5);"+
			"越界说明关键词表漂移,须重跑校准脚本", droppablePct)
	}
	if sample[LevelMust] == 0 {
		t.Error("必读档为空 —— must 关键词表可能被清空")
	}
}
