package cluster

import "sort"

// raw 层转载组(issue #83 分期 P-4 / 原 P-8):把「同一篇稿」的 raw 文档归到一组,
// 选一个代表行落 `raw_documents.canonical_id`(NULL = 自己是代表)。
//
// 为什么在 raw 层再做一次:P-1 的指纹分组是**读路径派生**(簇内、事件层);P8 要的是
// 「簇 canonical 名下**全部** raw_documents 的 (机构名, url)」—— 因为**某家报了但没被抽出
// 事件时,事件层看不到它**(issue P8 红线:「链接取 raw 层全集,事件层是子集」)。
// 故需要把「哪些 raw 行是同一篇稿」**持久化**到 raw 层。
//
// ⚠️ **判据唯一**:本文件**直接复用** `GroupReprints`(同一指纹、同一阈值 0.85),
// 不新增第二把尺子。P-1 登记的残余上限(改写型转载漏判 → 高估印证度)在 raw 层一并继承。
//
// ⚠️ **成组条件比 P-1 严格**:P-1 判「簇内两成员是否近逐字」(与机构无关);
// 本处要求组内**至少 2 个不同机构**(`source_id`)—— 因为「同一机构发了两条相同内容」
// 不是 issue 意义上的「多家转载同一篇稿」,并成一个来源无意义(反而会把同机构多条 URL 藏掉)。
// 这与 P-1「转载 ≠ 独立源」的目标**正交**(那是簇层,这是 raw 层),故条件不同是对的。

// RawDocEntry raw 层分组所需的**最小投影**(避免 cluster 包依赖 store/model)。
// 与 `store.RawDocForGrouping` 字段一一对应,由调用方组装。
type RawDocEntry struct {
	ID          string // raw_documents.id
	SourceID    string // 机构(去重按此,不按机构名 —— id 才是真源)
	Content     string // 正文(指纹取原文,见 NormalizeReprintContent)
	RetrievedAt string // NOT NULL;用字符串 RFC3339 只为可比较/可序列化,不参与语义
	// CanonicalID 既有代表(空串 = NULL = 自己就是代表)。**非空表示该行已冻结**,
	// 分组时**以已冻结的代表为准**(见 RawGroups 的代表选取)—— 保证「冻结优先于重选」。
	CanonicalID string
}

// RawGroup 一组「同一篇稿」的 raw 行。
// RepID = 代表行 id(落 canonical_id 的值);Members = 组内**仍为 NULL、需要写回**的行 id
// (不含代表,**不含已冻结行**;可能为空 —— 此时该组已收敛,无需写库)。
type RawGroup struct {
	RepID   string
	Members []string
}

// RawGroups 把一批 raw 文档按 P-1 正文指纹聚成**转载组**,仅返回「组内 ≥2 个不同机构」的组。
//
// 单成员组、同机构多行组**不返回**(调用方据此:未出现在结果里的行 canonical_id 保持 NULL
// = 自己是代表,这既省写库也更少漂移)。
//
// 代表选取(**冻结优先**,两段):
//  1. 组内**已有冻结代表**(某成员 `CanonicalID` 非空)→ 取它;若多个且不一致,取字典序最小者
//     (确定性;理论上不该出现,出现即异常,取最小只为可复现)。
//  2. 否则取 **`RetrievedAt` 最早,同则 `ID` 字典序最小** —— 确定性(与 P-1 `ReprintFlags`
//     的「组内最小下标」同精神:只求可复现)。
//
// Members 只含 `CanonicalID == ""` 的行(冻结行**不重写**)。⚠️ 代表**冻结**:调用方落库时
// 还套 `AND canonical_id IS NULL` 二重保险(见 `store.SetRawCanonicalIDs`)。
func RawGroups(docs []RawDocEntry) []RawGroup {
	n := len(docs)
	if n < 2 {
		return nil
	}
	contents := make([]string, n)
	for i, d := range docs {
		contents[i] = d.Content
	}
	// 复用 P-1 指纹分组:空/极短正文各自成组(不参与合并),组间按下标升序 —— 输出稳定。
	groups := GroupReprints(contents)

	// 按组内**不同机构数 ≥2** 过滤,并选代表。
	var out []RawGroup
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		srcs := make(map[string]struct{}, len(g))
		for _, i := range g {
			srcs[docs[i].SourceID] = struct{}{}
		}
		if len(srcs) < 2 {
			continue // 同机构多发同内容:不是「多家转载同一篇稿」
		}
		repID := rawGroupRep(docs, g)
		// 写回集:仍为 NULL 的成员(代表本身 NULL 且 id == repID,天然被排除)。
		members := make([]string, 0, len(g))
		for _, i := range g {
			if docs[i].CanonicalID == "" && docs[i].ID != repID {
				members = append(members, docs[i].ID)
			}
		}
		sort.Strings(members)
		out = append(out, RawGroup{RepID: repID, Members: members})
	}
	// 组间按代表 id 排序 —— 输出确定,便于测试与幂等核对。
	sort.Slice(out, func(a, b int) bool { return out[a].RepID < out[b].RepID })
	return out
}

// rawGroupRep 在组 g 内选代表:已冻结代表优先(取字典序最小者),否则最早入库者,同则 id 最小。
func rawGroupRep(docs []RawDocEntry, g []int) string {
	frozen := ""
	for _, i := range g {
		if c := docs[i].CanonicalID; c != "" && (frozen == "" || c < frozen) {
			frozen = c
		}
	}
	if frozen != "" {
		return frozen
	}
	rep := g[0]
	for _, i := range g[1:] {
		if rawRepLess(docs[i], docs[rep]) {
			rep = i
		}
	}
	return docs[rep].ID
}

// rawRepLess 报告 a 是否比 b 更适合当组代表:入库更早者胜,同则 id 字典序更小者胜。
func rawRepLess(a, b RawDocEntry) bool {
	if a.RetrievedAt != b.RetrievedAt {
		return a.RetrievedAt < b.RetrievedAt
	}
	return a.ID < b.ID
}
