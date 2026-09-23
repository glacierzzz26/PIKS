package cluster

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// 剥转载:转载 ≠ 独立源(issue #83 分期 P-1)。
//
// 场景:同一篇通稿被两家以**近逐字**的形态各自落一行 —— 机构 A 是原发、机构 B 是转载。
// 去重键含 `source_id`(迁移 0016 的两条 partial unique index),故两行都进库;
// 印证度 `source_count` 按机构名去重(`store.ListClusterSources`),于是**转载把「几家在报」
// 这个客观计数刷高**。印证度的三级判定(单一来源 / 多家印证 / 广泛报道)**整个建立在这个计数上**,
// 计数不实则三级分级全废。
//
// ⚠️ **转载 ≠ 同事件**(这是本文件最要紧的一条边界,勿混):
//
//	转载 = 同一篇稿被**原样/近逐字**转发   → 只算一个独立来源(**本文处理**)
//	同事件 = 不同机构**各自改写**报道同一件事 → 聚成一簇,各算独立来源(cluster 负责,阈值 0.7)
//
// **不得**用同一把尺子判两者。尤其有两条实测反例:
//
//  1. **不得用标题指纹判转载**。同一事件的标题在多源间**天然收敛**(东财「【芯源微：初步确定股东
//     询价转让价格为320.97元/股】」与东财裸标题归一化后**完全相同**),用标题高相似度判转载会把
//     「一家转载」的帽子扣到两家独立报道的头上,几乎所有簇塌成「单一来源」,与分级目标背道而驰。
//  2. **不得复用 `NormalizeTitle`**。它内含 `stripBracketWrap`,对「【标题】正文」会**丢掉正文、
//     只留标题** —— 于是「裸标题」(东财)与「【同标题】+长正文」(金十)归一化后**同为标题串、
//     J=1.000**,被误判成转载(实测 156 对如此)。同理它也把「短标题 vs 全文」这类**真实改写**
//     抬到 1.0。故指纹取 **content 原文**(去电头/HTML 标签后),**不**抽【】。
//
// 归一化在 `NormalizeReprintContent`(本文件),与 `NormalizeTitle` 刻意分开 —— 后者的抽标题逻辑
// 对**聚类**是对的,对**剥转载**恰好有害。

// reprintThreshold 转载指纹门控:正文指纹 Jaccard ≥ 此值才算「同一篇稿的转载」,只计一个独立来源。
//
// 取值 0.85,依据 = 对**生产真实语料**(lab 库 2026-09-20~21,398 个多机构簇 / 1369 篇成员 /
// 2735 个跨机构对;取证脚本见 docs/phase11/design/reprint-stripping.md §3)实测的分布(正文指纹,
// 含电头剥离):
//
//	区间         对数   人工核对结论
//	0.80-0.85     28    近逐字真转载(电头/「市场消息：」/尾部括注差异)
//	0.85-0.90     30    近逐字真转载(其中 0.84~0.88 逐条人工核对**全部为真转载**)
//	0.90-0.95     28    真转载(措辞极近,仅电头或一两个虚词之差)
//	0.95-1.00     72    逐字相同(归一化后完全一致)
//	0.75-0.80     11    改写(短标题 vs 全文 / 换措辞) —— **不得**合并
//	0.70-0.75     30    改写 —— **不得**合并
//
// 分布呈**强双峰**:p75 = 0.333、p90 = 1.000,0.80 与 1.00 之间是清晰边界。取 0.85 —— 它落在
// 「下沿 0.80 全是真转载」的**内侧**,把「短标题 vs 全文」(J≈0.73~0.79)这类**改写**挡在外面,
// 宁漏勿误(误判会直接压低印证度,是更坏的错)。
//
// ⚠️ **残余精度上限**(如实登记,阈值调优无法消除):各家**改写**同一通稿(换措辞保事实)时指纹漏判
// → 仍会**高估**印证度。0.75~0.85 区间里的真转载即此边界的实测样本,不为此下调门控。
const reprintThreshold = 0.85

// reprintDatelineRe 剥电头:正文开头的「财联社9月21日电，」「金十数据9月21日讯，」等频道时间戳,
// 允许【…】内嵌标题后紧跟(如「【标普500指数涨幅扩大至1%】财联社9月21日电，…」)。
// 实测覆盖 18.3% 的成员行;剥除后 ≥0.85 的真转载由 397 增至 453(混合区 0.75~0.85 不变),
// 即**只把真转载抬过门控,不把改写抬过**。捕获组保留前导【标题】。
var reprintDatelineRe = regexp.MustCompile(`^(【[^】]{0,40}】)?\s*[^\s，。；：,、]{0,12}?\d{1,2}月\d{1,2}日\s*[电讯]\s*[，,：:]\s*`)

// reprintHTMLRe 剥内联 HTML 标签(金十正文常带 `<b>…</b>`,实测 2% 成员行有,剥后 J 由 0.87 抬到 1.0)。
var reprintHTMLRe = regexp.MustCompile(`<[^>]{0,32}>`)

// NormalizeReprintContent 剥转载专用正文归一化:**只**去「电头 + HTML 标签」这类**转载痕迹**,
// 再折叠空白/标点、转小写、留汉字与字母数字。与 `NormalizeTitle` 的关键区别:**绝不抽【】内嵌标题**
// (抽标题会把「短标题 vs 全文」误抬到 1.0,见 reprint.go 顶部反例 2)。
func NormalizeReprintContent(s string) string {
	s = reprintDatelineRe.ReplaceAllString(s, "$1") // 保留前导【标题】,只去时间戳
	s = reprintHTMLRe.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsSpace(r):
			continue
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case unicode.Is(unicode.Han, r):
			b.WriteRune(r)
		default: // 标点等跳过
		}
	}
	return b.String()
}

// ReprintFingerprint 正文指纹:归一化后的字符二元组集合(与 cluster 既有相似度同一把尺子)。
// 空正文 → 空集合(Jaccard 对空集合恒 0,即永不判转载 —— 宁漏勿误)。
func ReprintFingerprint(content string) map[string]struct{} {
	return Bigrams(NormalizeReprintContent(content))
}

// GroupReprints 把一组正文按指纹聚成「转载组」,返回每组的成员下标(组内按下标升序,
// 组间按各组最小下标升序 —— 输出稳定,便于前端与测试复现)。
//
// 判定:两成员指纹 Jaccard ≥ reprintThreshold 即视为同一篇稿,并查集合并(传递闭包 ——
// A~B、B~C 则 A/B/C 同组)。**单成员组 = 独立来源;多成员组 = 一篇稿的多次转载,只计一个独立来源。**
//
// 空/极短正文(指纹为空)不参与合并,自成一组成员 —— 不因「没内容」把两家捏成一个来源。
func GroupReprints(contents []string) [][]int {
	n := len(contents)
	if n == 0 {
		return nil
	}
	fp := make([]map[string]struct{}, n)
	for i, c := range contents {
		fp[i] = ReprintFingerprint(c)
	}

	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]] // 路径压缩
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	for i := 0; i < n; i++ {
		if len(fp[i]) == 0 {
			continue
		}
		for j := i + 1; j < n; j++ {
			if len(fp[j]) == 0 {
				continue
			}
			if Jaccard(fp[i], fp[j]) >= reprintThreshold {
				union(i, j)
			}
		}
	}

	groups := make(map[int][]int, n)
	for i := 0; i < n; i++ {
		r := find(i)
		groups[r] = append(groups[r], i)
	}
	out := make([][]int, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	sort.Slice(out, func(a, b int) bool { return out[a][0] < out[b][0] })
	return out
}

// IndependentCount 独立来源数 = 转载组数(便捷封装)。未聚类/空输入按 1 计 —— 确实只有自己那一家,
// 与 store 侧 `source_count` 的「未聚类 = 1」口径一致。
func IndependentCount(contents []string) int {
	if len(contents) == 0 {
		return 1
	}
	return len(GroupReprints(contents))
}

// ReprintFlags 就一组正文给出「每篇是否为转载」的布尔标记(下标与入参平行)。
//
// 判据:所属转载组含 >1 成员、该成员**不是本组的原发**、且指纹非空 —— 空正文不标转载(不冤枉)。
//
// 「原发」= `canonicalIdx`(簇的 canonical 事件下标):cluster 的 canonical 按 `created_at` 最早选,
// 即**该簇最早的报道**,拿它当原发是合理且确定性的。若 canonicalIdx 不在某转载组内(该组全是被
// 并入的成员),则取组内**下标最小**者当原发 —— 仍确定性,宁少不滥(只标「非原发」,不倒过来把原发
// 标成转载)。`canonicalIdx < 0` 表示未知(无 canonical),此时每组的原发按组内最小下标定。
//
// 前端据此逐条如实标注「(转载)」,不隐藏、不合并显示(项目红线「不静默」)。
// ⚠️ 必须**恰好每组留一个不标** —— 否则「N 家为转载」与该标记数对不上(原发被误标)。
func ReprintFlags(contents []string, canonicalIdx int) []bool {
	flags := make([]bool, len(contents))
	for _, g := range GroupReprints(contents) {
		if len(g) < 2 {
			continue
		}
		origin := g[0] // 组内最小下标 = 默认原发
		for _, i := range g {
			if i == canonicalIdx {
				origin = i
				break
			}
		}
		for _, i := range g {
			if i == origin {
				continue
			}
			// 空正文自成一组的对端已被排除在合并之外;此处再挡一次,防御未来规则变更。
			if strings.TrimSpace(contents[i]) != "" {
				flags[i] = true
			}
		}
	}
	return flags
}
