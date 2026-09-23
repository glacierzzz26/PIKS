package cluster

import (
	"reflect"
	"testing"
)

// 剥转载判定(issue #83 分期 P-1)。
//
// 用例取自 **docs/phase11/design/reprint-stripping.md §3 的实测语料**
// (lab 生产库 2026-09-20~21,398 个多机构簇 / 1369 篇成员 / 2735 个跨机构对),
// 关键样本**逐字复制原文**(可与设计文档的取证 SQL 对齐复现,括号内为实测 Jaccard)。
//
// 三类:
//   - 真转载(必须合并):逐字相同、电头差异、【标题】 vs 标题+正文、HTML 标签残留;
//   - 改写(必须不合并):同事件不同措辞 —— 这是**误判的唯一来源**,也是最要紧的一类;
//   - 边界(如实钉住):空正文、单成员、短稿。

// ── 真转载:归一化后近逐字 ────────────────────────────────────────────────

func TestGroupReprintsTrueReprint(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{
			"逐字相同(仅财联社电头差异,J=1.000)",
			"【长鑫科技：第五代工艺技术平台实现量产】财联社9月20日电，长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产，并同步展出基于该平台打造的大容量LPDDR5X产品。",
			"【长鑫科技：第五代工艺技术平台实现量产】长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产，并同步展出基于该平台打造的大容量LPDDR5X产品。",
		},
		{
			"裸标题 vs【同标题】正文 + 金十电头(J=0.897)",
			"诺和诺德：计划到2030年将产能扩大至可服务10倍使用口服GLP-1的肥胖患者",
			"财联社9月21日电，诺和诺德表示，计划到2030年将产能扩大至可服务10倍使用口服GLP-1的肥胖患者。",
		},
		{
			"【标题】 vs 标题+正文 + 尾部括注((外交部),J=0.982)",
			"【中国政府中东问题特使翟隽会见瑞士外交部中东北非司司长基尔格兹】金十数据9月21日讯，2026年9月21日，中国政府中东问题特使翟隽应约会见瑞士外交部中东北非司司长基尔格兹，双方就中东地区热点问题交换意见。",
			"中国政府中东问题特使翟隽会见瑞士外交部中东北非司司长基尔格兹 2026年9月21日，中国政府中东问题特使翟隽应约会见瑞士外交部中东北非司司长基尔格兹，双方就中东地区热点问题交换意见。(外交部)",
		},
		{
			"HTML 标签残留(<b>…</b>,J=1.000;含原文错字「基础设施设施」)",
			"<b>州长表示，乌克兰波尔塔瓦地区的关键基础设施设施遭到俄罗斯无人机袭击。</b>",
			"州长表示，乌克兰波尔塔瓦地区的关键基础设施设施遭到俄罗斯无人机袭击。",
		},
		{
			"实体名一字之差(卡塔尔能源 vs 卡塔尔能源公司,J=0.879)",
			"卡塔尔能源首席执行官：卡塔尔能源正认真考虑与合作伙伴共同进入委内瑞拉市场。",
			"卡塔尔能源公司首席执行官：卡塔尔能源正认真考虑与合作伙伴共同进入委内瑞拉市场。",
		},
		{
			"前缀不同(「市场消息：」 vs 财联社电头,J=0.857)",
			"财联社9月21日电，数据显示，印度近40%的燃煤电厂燃料库存已降至危险低位。",
			"市场消息：数据显示，印度近40%的燃煤电厂燃料库存已降至危险低位。",
		},
	}
	for _, c := range cases {
		got := GroupReprints([]string{c.a, c.b})
		if len(got) != 1 {
			t.Errorf("%s: 应合成 1 组(转载), got %d 组 %v", c.name, len(got), got)
		}
	}
}

// ── 改写:同事件、不同措辞 —— 必须**不**合并(误判只可能出在这里) ────────────

func TestGroupReprintsRewritesNotMerged(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{
			"裸标题 vs【同标题】+ 长正文(东财 vs 金十,J=0.268 —— 抽【】标题会把这对误抬到 1.0)",
			"芯源微：初步确定股东询价转让价格为320.97元/股",
			"【芯源微：初步确定股东询价转让价格为320.97元/股】金十数据9月21日讯，芯源微公告，经向机构投资者询价后，初步确定的转让价格为320.97元/股。参与本次询价转让报价的机构投资者家数为33家，涵盖了基金管理公司、证券公司、保险公司、合格境外机构投资者、私募基金管理人、信托公司等专业机构投资者。",
		},
		{
			"短标题 vs 全文(东财 vs 财联社,J=0.213)",
			"慧博云通拟5470万元收购南京金信49%股权，实现全资控股",
			"【慧博云通：拟5470万元收购南京金信49%股权】慧博云通9月21日公告，公司拟以现金方式收购卞雯丽持有的南京慧博金信科技有限公司49.00%股权，交易对价合计为人民币5470万元。本次交易完成后，公司持有南京金信的股权比例将由51.00%上升至100.00%。",
		},
		{
			"短标题 vs【同标题】正文(富途 vs 金十,J=0.317)",
			"宇树科技发布Dex5-S灵巧手",
			"【宇树科技发布Dex5-S灵巧手】金十数据9月21日讯，宇树科技发布Dex5-S灵巧手，22个关节均可以丝滑反向驱动，且每个关节自带极限冲击力距保护。",
		},
		{
			"数字口径不同(1-8月 vs 前8个月,J=0.806 —— 0.85 边界的实测反例)",
			"【国新办发布会】1-8月，全国一般公共预算收入156824亿元，同比增长0.7%。",
			"【国新办发布会】前8个月，全国一般公共预算收入156824亿元，同比增长0.7%。",
		},
	}
	for _, c := range cases {
		got := GroupReprints([]string{c.a, c.b})
		if len(got) != 2 {
			t.Errorf("%s: 独立改写**不得**判为转载, 应 2 组, got %d 组 %v", c.name, len(got), got)
		}
	}
}

// ── 短稿/边界 ────────────────────────────────────────────────────────────

func TestGroupReprintsEdges(t *testing.T) {
	if got := GroupReprints(nil); got != nil {
		t.Errorf("空输入应返回 nil, got %v", got)
	}
	if got := IndependentCount(nil); got != 1 {
		t.Errorf("空输入独立来源数应为 1(未聚类口径), got %d", got)
	}

	// 极短同文(8 字,实测存在),应仍能判转载。
	short := []string{"纳指涨幅扩大至1%。", "纳指涨幅扩大至1%。"}
	if got := GroupReprints(short); len(got) != 1 {
		t.Errorf("极短同文应合成 1 组, got %v", got)
	}

	// 空正文不参与合并:两个空正文各自成组(不因「没内容」把两家捏成一个来源)。
	empties := []string{"", "", "有内容的一句话快讯正文示例"}
	if got := GroupReprints(empties); len(got) != 3 {
		t.Errorf("空正文应各自成组(不合并), got %d 组 %v", len(got), got)
	}

	// 三成员传递闭包:A~B、B~C ⇒ A/B/C 同组(卡塔尔能源三源近逐字)。
	tri := []string{
		"卡塔尔能源首席执行官：卡塔尔能源正认真考虑与合作伙伴共同进入委内瑞拉市场。",
		"卡塔尔能源公司首席执行官：卡塔尔能源正认真考虑与合作伙伴共同进入委内瑞拉市场。",
		"卡塔尔能源首席执行官：卡塔尔能源正认真考虑与合作伙伴共同进入委内瑞拉市场。",
	}
	if got := IndependentCount(tri); got != 1 {
		t.Errorf("三成员近逐字应合成 1 个独立来源, got %d 组 %v", got, GroupReprints(tri))
	}
}

// TestNormalizeReprintContentPreservesTitleAfterBracket 钉住最关键的一条:
// 「【标题】正文」**不得**被抽成只剩标题(抽了就与裸标题 J=1.0,误判转载)。
// 这是与 `NormalizeTitle` 的行为分岔点,回归防线。
func TestNormalizeReprintContentPreservesTitleAfterBracket(t *testing.T) {
	withBracket := NormalizeReprintContent("【芯源微：初步确定股东询价转让价格为320.97元/股】金十数据9月21日讯，芯源微公告，经向机构投资者询价后，初步确定的转让价格为320.97元/股。")
	bare := NormalizeReprintContent("芯源微：初步确定股东询价转让价格为320.97元/股")
	if withBracket == bare {
		t.Fatal("正文归一化不得抽【】内标题只剩标题串(否则裸标题与【标题】正文会 J=1.0 误判转载)")
	}
	if Jaccard(Bigrams(withBracket), Bigrams(bare)) >= reprintThreshold {
		t.Errorf("【标题】正文 与 裸标题 的 Jaccard 不得 ≥ 转载门控")
	}
}

// ── 计数与标记 ───────────────────────────────────────────────────────────

func TestIndependentCountAndFlags(t *testing.T) {
	// 构造成簇场景:1 条原创 + 1 条同文转载 + 1 条独立改写 ⇒ 机构数 3、独立来源数 2。
	// canonicalIdx = 0(原创那条),故转载标记落在下标 1,原发(下标 0)不标。
	svc := []string{
		"【长鑫科技：第五代工艺技术平台实现量产】财联社9月20日电，长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产。",
		"【长鑫科技：第五代工艺技术平台实现量产】长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产。",
		"宇树科技发布Dex5-S灵巧手",
	}
	if got := IndependentCount(svc); got != 2 {
		t.Errorf("独立来源数应为 2, got %d", got)
	}
	flags := ReprintFlags(svc, 0)
	want := []bool{false, true, false} // 原发(0)不标,转载(1)标,独立改写(2)不标
	if !reflect.DeepEqual(flags, want) {
		t.Errorf("转载标记应为 %v, got %v", want, flags)
	}
	// 每组恰好留一个不标 —— 转载标记数 = 机构数 − 独立来源数(前端「N 家为转载」据此显示)。
	if n := countTrue(flags); n != len(svc)-IndependentCount(svc) {
		t.Errorf("转载标记数 %d 应等于 机构数−独立来源数 %d", n, len(svc)-IndependentCount(svc))
	}
	// canonicalIdx 不在转载组内(该组全是被并入成员)时,退化为组内最小下标当原发。
	flagsAlt := ReprintFlags(svc, 2)
	if countTrue(flagsAlt) != 1 {
		t.Errorf("canonical 落在别组时,转载组仍应只标 1 个(不把原发标成转载), got %v", flagsAlt)
	}
}

func countTrue(bs []bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}

// TestIndependentCountMonotonic 组数 ≤ 成员数(转载只会**减少**独立来源,永不虚增)。
func TestIndependentCountMonotonic(t *testing.T) {
	svc := []string{"a b c 这是一条新闻", "a b c 这是一条新闻", "完全不同的一条独立报道内容"}
	if n, m := IndependentCount(svc), len(svc); n > m {
		t.Errorf("独立来源数 %d 不得超过成员数 %d", n, m)
	}
}

// TestGroupReprintsDeterministic 输出顺序稳定(前端渲染不抖动、测试可复现)。
func TestGroupReprintsDeterministic(t *testing.T) {
	svc := []string{"第三条独立正文内容啊", "第一条独立正文内容啊", "第二条独立正文内容啊"}
	want := [][]int{{0}, {1}, {2}}
	for i := 0; i < 5; i++ {
		got := GroupReprints(svc)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("输出应稳定升序, got %v", got)
		}
	}
}
