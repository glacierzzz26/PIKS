package cluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
)

func mkEvent(id, title, etype string, affected []string, at time.Time, conf float64) model.Event {
	af, _ := json.Marshal(affected)
	return model.Event{
		ID: id, Title: title, EventType: etype,
		Affected: af, OccurredAt: &at, Confidence: conf, CreatedAt: at,
	}
}

// 规则直合:归一化标题全同 + 同类型。
func TestGenCandidatesAuto(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.8),
		mkEvent("c", "星河新能源发布固态电池", "tech", []string{"新能源"}, now, 0.9),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 1 || len(c.Auto[0]) != 2 {
		t.Fatalf("expected 1 auto group of 2, got %v", c.Auto)
	}
	if len(c.LLM) != 0 {
		t.Fatalf("auto-merged events must not enter LLM pool, got %d pairs", len(c.LLM))
	}
}

// #17 真实验证回归:实体措辞不一致("银行" vs "银行板块")→ 包含关系视为重叠 → LLM 候选。
func TestEntityContainmentCandidate(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调金融机构存款准备金率0.25个百分点", "policy", []string{"金融机构", "银行"}, now, 0.9),
		mkEvent("b", "央行降准0.25个百分点 释放约5000亿流动性", "policy", []string{"银行板块", "LPR"}, now.Add(45*time.Minute), 0.8),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 0 {
		t.Fatalf("different titles should not auto-merge: %v", c.Auto)
	}
	if len(c.LLM) != 1 {
		t.Fatalf("containment entity overlap must produce 1 LLM pair, got %d", len(c.LLM))
	}
}

// 中等置信:标题不同但实体重叠+时间近 → LLM 候选 → mock 确认同事件。
func TestLLMPath(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "降准靴子落地 央行释放流动性", "policy", []string{"银行", "房地产"}, now.Add(1*time.Hour), 0.8),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 0 {
		t.Fatalf("different titles should not auto-merge: %v", c.Auto)
	}
	if len(c.LLM) != 1 {
		t.Fatalf("expected 1 LLM pair, got %d", len(c.LLM))
	}
	verds, tokens, err := ConfirmPairs(context.Background(), ai.NewMock(), events, c.LLM, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tokens == 0 {
		t.Fatal("expected token usage recorded")
	}
	if !verds[0].IsSame {
		t.Fatal("mock should confirm 降准 pair as same event")
	}
	comps := BuildComponents(len(events), c.Auto, verds, c.LLM)
	if len(comps) != 1 || len(comps[0]) != 2 {
		t.Fatalf("expected 1 component of 2, got %v", comps)
	}
}

// 跨类型事件不构成 LLM 候选(防知识库污染)。
func TestNoCrossTypeCandidate(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "降准靴子落地 央行释放流动性", "tech", []string{"银行"}, now.Add(1*time.Hour), 0.8),
	}
	c := GenCandidates(events)
	if len(c.LLM) != 0 {
		t.Fatalf("cross-type events must not be LLM candidates, got %d", len(c.LLM))
	}
}

// 聚类质量回归:跨簇重复(design cluster-quality)。两条近同标题 canonical 分属两簇,
// 重审视候选池(既有 canonical ∪ 未聚类)会生成 LLM 对 → 触发确认并并簇。
// 这正是不加重审视时因 ListUnclusteredEvents 只聚未聚类而永远漏掉的对。
func TestCrossClusterCandidate(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率0.25个百分点", "policy", []string{"银行", "房地产"}, now, 0.9),
		mkEvent("b", "央行宣布下调金融机构存款准备金率0.25个百分点", "policy", []string{"金融机构", "银行"}, now, 1.0),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 0 {
		t.Fatalf("different titles should not auto-merge: %v", c.Auto)
	}
	if len(c.LLM) != 1 {
		t.Fatalf("cross-cluster duplicate titles must produce 1 LLM pair, got %d", len(c.LLM))
	}
}

// pickSurvivorIndex:survivor 恒为既有簇代表;多成员簇优先 > 最早创建 > 高置信;无簇成员返回 -1。
func TestPickSurvivorIndex(t *testing.T) {
	now := time.Now()
	pool := []model.Event{
		mkEvent("a", "事件甲", "policy", []string{"银行"}, now.Add(-2*time.Hour), 0.8), // 簇 X(最早)
		mkEvent("b", "事件乙", "policy", []string{"银行"}, now.Add(-1*time.Hour), 0.9), // 簇 Y(次早)
		mkEvent("c", "事件丙", "policy", []string{"银行"}, now, 0.5),                   // 簇 Z(最晚)
		mkEvent("d", "事件丁", "policy", []string{"银行"}, now.Add(1*time.Hour), 0.7),  // 未聚类(更早,不参与)
		mkEvent("e", "事件戊", "policy", []string{"银行"}, now, 1.0),                   // 簇 W(与 c 同刻,更高置信)
	}
	clusterOf := []string{"X", "Y", "Z", "", "W"}
	// 全部单成员簇:选举退化为「最早创建 / 同刻高置信」,与旧行为一致。
	single := memberCounts{"X": 1, "Y": 1, "Z": 1, "W": 1}

	if got := pickSurvivorIndex(pool, clusterOf, single, []int{3}); got != -1 {
		t.Fatalf("no cluster member should return -1, got %d", got)
	}
	if got := pickSurvivorIndex(pool, clusterOf, single, []int{3, 1}); got != 1 {
		t.Fatalf("single cluster member should win, got %d", got)
	}
	if got := pickSurvivorIndex(pool, clusterOf, single, []int{0, 1, 2}); got != 0 {
		t.Fatalf("earliest created should win, got %d", got)
	}
	if got := pickSurvivorIndex(pool, clusterOf, single, []int{2, 4}); got != 4 {
		t.Fatalf("tie created_at should go to higher confidence, got %d", got)
	}

	// 🔴 issue #75 护栏:更早的**单成员簇**不得吃掉**多源簇**(否则 MergeClusters 丢弃
	// 多源簇的 LLM canonicalTitle,违反设计 D-Q3)。此处 Z 更晚但成员多,应胜出。
	multi := memberCounts{"X": 1, "Y": 3, "Z": 1, "W": 1}
	if got := pickSurvivorIndex(pool, clusterOf, multi, []int{0, 1}); got != 1 {
		t.Fatalf("multi-member cluster must beat an earlier single-member one, got %d", got)
	}
}

// ---------- 跨源同事件判定(issue #48 T2) ----------
//
// 下列标题**逐字取自 2026-09-20 dev 库实测**的 6 源快讯(T1 采集产物),勿臆造 ——
// 校准结论(见 docs/phase11/design/event-cross-source.md)全部基于这批真实数据得出。

// 内嵌前缀抽取:【】内文字是**标题**时取出来,是**栏目标签**时保留原句。
func TestStripBracketWrap(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			// 东财:【规范标题】+ 公告正文 → 取【】内
			"东财标题前缀", "【天山生物：持股5%以上股东拟减持不超3%股份】天山生物(300313.SZ)公告称，持股5%以上股东新疆畜牧业集团有限公司计划…",
			"天山生物：持股5%以上股东拟减持不超3%股份",
		},
		{
			// 新浪:同款【标题】正文
			"新浪标题前缀", "【日本奄美大岛附近海域发生5.0级地震】据日本气象厅消息，当地时间20日16时2…",
			"日本奄美大岛附近海域发生5.0级地震",
		},
		{
			// 标题【正文】 → 取【】前的标题
			"标题在括号外", "盛和资源：澄清不存在拟对外转让公司控股权情形【盛和资源公告称…】",
			"盛和资源：澄清不存在拟对外转让公司控股权情形",
		},
		{
			// 栏目标签:【】内不足 6 字,不是标题 → 原样保留(否则会与正文脱节)
			"栏目标签不抽取", "【电报解读】Anthropic据报计划年底拥有约5吉瓦可用算力！",
			"【电报解读】Anthropic据报计划年底拥有约5吉瓦可用算力！",
		},
		{
			// 栏目标签(7 字,会被抽取):实测该行与其它 162 行无任何 ≥0.5 相似对(最高 0.059),
			// 退化成短标签后同样不构成误合并 —— 长度门槛对这批真实数据是安全的。
			"栏目标签·研报(>=6字则抽取)", "【风口研报·公司】AI挤占产能导致MLCC供需趋紧",
			"风口研报·公司",
		},
		{
			// 栏目标签但不足 6 字 → 保留原句
			"栏目标签·短(保留)", "【要闻】央行今日开展逆回购操作",
			"【要闻】央行今日开展逆回购操作",
		},
		{"无括号", "比亚迪：2026年8月乘用车及皮卡海外销售188746辆 同比增长134.6%",
			"比亚迪：2026年8月乘用车及皮卡海外销售188746辆 同比增长134.6%"},
		{"只有左括号", "【未闭合的标题", "【未闭合的标题"},
		{"空串", "", ""},
	}
	for _, c := range cases {
		if got := stripBracketWrap(c.in); got != c.want {
			t.Errorf("%s: stripBracketWrap(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// 归一化后跨源标题相似度必须回到阈值之上 —— 这是「不降门槛」能成立的前提。
func TestCrossSourceSimilarityRestored(t *testing.T) {
	cases := []struct {
		name, a, b string
	}{
		{
			"东财【标题】正文 vs 财联社裸标题",
			"【天山生物：持股5%以上股东拟减持不超3%股份】天山生物(300313.SZ)公告称，持股5%以上股东新疆畜牧业集团有限公司计划自公告披露之日起15个交易日后的三个月内，以集中竞价、大宗交易方式减持公司股份合计不超过719.34万股，即不超过公司总股本的3%。减持原因为自身经营发展资金需求。",
			"天山生物：持股5%以上股东拟减持不超3%股份",
		},
		{
			"新浪【标题】正文 vs 金十裸标题(地震)",
			"【日本奄美大岛附近海域发生5.0级地震】据日本气象厅消息，当地时间20日16时2分左右，日本奄美大岛附近海域发生5.0级地震。",
			"日本奄美大岛附近海域发生5.0级地震",
		},
		{
			"东财 vs 新浪(同款内嵌前缀)",
			"【立讯精密：立讯转债即将到期及停止交易】9月20日，立讯精密公告，公司发行的立讯转债到期日为2026年11月2日…",
			"【立讯精密：立讯转债即将到期及停止交易】立讯精密公告称，公司发行的立讯转债到期日为2026年11月2日…",
		},
	}
	for _, c := range cases {
		v := Jaccard(Bigrams(NormalizeTitle(c.a)), Bigrams(NormalizeTitle(c.b)))
		if v < 0.7 {
			t.Errorf("%s: 归一化后 Jaccard=%.3f < 0.7(应>=阈值,否则漏合并)", c.name, v)
		}
	}
}

// 回归护栏:同类型 + 同股但**不同批次**的公告不得因归一化加固而变成候选对。
// 实测(163 条 6 源真实标题)这三条新华制药公告在 0.6/0.7/0.8 各阈值下 Jaccard 均 ≤0.515;
// 【】内标题抽取后仍不得 ≥0.7 —— 即「不同药品」这组判别边界不因加固而失守。
func TestNoFalseMergeSameStockDifferentAnnouncements(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "【新华制药：获得硫酸镁钠钾口服用浓溶液药品注册证书】新华制药9月20日公告…", "company", nil, now, 0.8),
		mkEvent("b", "【新华制药：获得左卡尼汀口服溶液药品注册证书】新华制药9月20日公告…", "company", nil, now, 0.8),
		mkEvent("c", "【新华制药：盐酸多奈哌齐口崩片获得药品注册证书】新华制药9月20日公告…", "company", nil, now, 0.8),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 0 {
		t.Fatalf("不同药品的注册证书不得直合: %v", c.Auto)
	}
	if len(c.LLM) != 0 {
		t.Fatalf("不同药品的注册证书不得进 LLM 候选(阈值不降), got %d pairs", len(c.LLM))
	}
}

// 跨源同一事件经内嵌前缀抽取后进入 LLM 候选(端到端:东财 vs 财联社)。
// 修正前该对 Jaccard 仅 0.165 → 漏合并;修正后 ≥0.7 → 进候选 → mock 确认同事件。
func TestCrossSourcePairEndToEnd(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "【天山生物：持股5%以上股东拟减持不超3%股份】天山生物(300313.SZ)公告称，持股5%以上股东新疆畜牧业集团有限公司计划…", "company", nil, now, 0.8),
		mkEvent("b", "天山生物：持股5%以上股东拟减持不超3%股份", "company", nil, now.Add(8*time.Minute), 0.8),
	}
	c := GenCandidates(events)
	// 内嵌前缀抽取后两侧归一化标题**全同** → 直接走高置信直合(无需 LLM,零 token)。
	if len(c.Auto) != 1 || len(c.Auto[0]) != 2 {
		t.Fatalf("跨源同一事件经抽取后应直合为 1 组 2 成员, got auto=%v", c.Auto)
	}
	if len(c.LLM) != 0 {
		t.Fatalf("已直合的对不应再进 LLM 池, got %d", len(c.LLM))
	}
	comps := BuildComponents(len(events), c.Auto, nil, nil)
	if len(comps) != 1 || len(comps[0]) != 2 {
		t.Fatalf("跨源同一事件应聚为一簇(2 成员), got %v", comps)
	}
}

// 内嵌前缀抽取**不得**把不同公司的公告拉近(实体不同 → 不应成对)。
func TestCrossSourceNoWrongCompany(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "【天山生物：持股5%以上股东拟减持不超3%股份】…", "company", nil, now, 0.8),
		mkEvent("b", "【埃夫特：股东信惟基石及一致行动人拟减持不超3%股份】…", "company", nil, now, 0.8),
	}
	c := GenCandidates(events)
	if len(c.LLM) != 0 {
		t.Fatalf("不同公司的减持公告不得成候选对, got %d", len(c.LLM))
	}
}
