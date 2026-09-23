package web

import (
	"testing"

	"piks/internal/store"
)

// toEventItemLegacy 适配 P-4 之前的位置参数调用:只给事件层取数,raw/titles 留空。
// 语义 = 「无 raw 层数据」,正是这些用例要的(它们验证事件层的计数与转载判定;
// raw 层「取全集」与簇标题的行为见 TestToEventItemRawLayerSources / TestToEventItemClusterTitle)。
func toEventItemLegacy(ev store.EventForAPI, idx map[string]nameRef,
	clusters map[string][]store.ClusterSource, members map[string][]store.ClusterMember) apiEventItem {
	return toEventItem(ev, eventItemInput{idx: idx, clusters: clusters, members: members})
}

// toEventItem 的 cluster_sources 投影(issue #48 T2):
//   - 跨源簇(≥2 机构)→ 下发 cluster_sources;
//   - 单源簇 / 未聚类事件 → 不下发(前端不谎报「多源印证」)。
func TestToEventItemClusterSources(t *testing.T) {
	cid := "cluster-1"
	url := "https://example.com/a"
	origin := "新华社"

	base := store.EventForAPI{ID: "e1", Title: "T", EventType: "company", Status: "extracted"}
	withCluster := base
	withCluster.ClusterID = &cid

	ev := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{
		cid: {
			{EventID: "e1", Source: "东方财富", URL: &url},
			{EventID: "e2", Source: "金十数据", Origin: &origin},
		},
	}, nil)
	if len(ev.ClusterSources) != 2 {
		t.Fatalf("跨源簇应下发 2 个来源, got %+v", ev.ClusterSources)
	}
	if ev.ClusterSources[0].Source != "东方财富" || ev.ClusterSources[0].URL != url {
		t.Errorf("第 1 个来源投影错误: %+v", ev.ClusterSources[0])
	}
	if ev.ClusterSources[1].Origin != "新华社" || ev.ClusterSources[1].URL != "" {
		t.Errorf("上游一级源应带出、无 url 应留空: %+v", ev.ClusterSources[1])
	}

	// 单源簇:只有自己一个机构 → 不下发。
	single := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{
		cid: {{EventID: "e1", Source: "东方财富"}},
	}, nil)
	if single.ClusterSources != nil {
		t.Fatalf("单源簇不应下发 cluster_sources, got %+v", single.ClusterSources)
	}

	// 未聚类(cluster_id NULL)。
	if got := toEventItemLegacy(base, nil, nil, nil); got.ClusterSources != nil {
		t.Fatalf("未聚类事件不应下发 cluster_sources, got %+v", got.ClusterSources)
	}

	// 簇 id 命中不到来源(数据竞态兜底)→ 不下发,不 panic。
	if got := toEventItemLegacy(withCluster, nil, nil, nil); got.ClusterSources != nil {
		t.Fatalf("无来源映射时不应下发, got %+v", got.ClusterSources)
	}
}

// source_count 投影(issue #49 T3):「单一来源」必须**可判定** ——
// cluster_sources 是 omitempty,单源簇与未聚类都不下发,客户端据此**分不清**
// 「只有 1 家在报」和「还没聚类」。故显式给机构计数。
func TestToEventItemSourceCount(t *testing.T) {
	cid := "cluster-1"
	base := store.EventForAPI{ID: "e1", Title: "T", EventType: "company", Status: "extracted"}
	withCluster := base
	withCluster.ClusterID = &cid

	// 未聚类 → 1(确实只有自己那一家),且不谎报多源。
	if got := toEventItemLegacy(base, nil, nil, nil); got.SourceCount != 1 {
		t.Errorf("未聚类事件 source_count 应为 1, got %d", got.SourceCount)
	}
	// 单源簇 → 1。
	if got := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{
		cid: {{EventID: "e1", Source: "东方财富"}},
	}, nil); got.SourceCount != 1 {
		t.Errorf("单源簇 source_count 应为 1, got %d", got.SourceCount)
	}
	// 跨源簇 → 机构数。
	if got := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{
		cid: {
			{EventID: "e1", Source: "东方财富"},
			{EventID: "e2", Source: "金十数据"},
			{EventID: "e3", Source: "财联社"},
		},
	}, nil); got.SourceCount != 3 {
		t.Errorf("跨源簇 source_count 应为 3, got %d", got.SourceCount)
	}
	// 簇内无来源记录(竞态)→ 回落 1,不 panic、不虚报。
	if got := toEventItemLegacy(withCluster, nil, nil, nil); got.SourceCount != 1 {
		t.Errorf("无来源映射时 source_count 应回落 1, got %d", got.SourceCount)
	}
}

// independent_count 投影(issue #83 P-1):转载 ≠ 独立源。
//
// 硬验收(issue 验收原文):簇内 3 家 —— 2 家近逐字(1 原发 + 1 转载)+ 1 家独立改写
// ⇒ **机构数 3 而独立来源数 2**,且转载那家 reprint=true。
// 反过来,纯转载簇(2 家同文)⇒ 独立来源数 1 —— 转载不得把「几家在报」刷高。
func TestToEventItemIndependentCount(t *testing.T) {
	cid := "cluster-1"
	withCluster := store.EventForAPI{ID: "e1", Title: "T", EventType: "company", Status: "extracted", ClusterID: &cid}
	cs := func(src string, content *string) store.ClusterSource {
		return store.ClusterSource{EventID: "e-" + src, Source: src, Content: content}
	}
	ptr := func(s string) *string { return &s }
	reprintOf := func(srcs []apiClusterSource) map[string]bool {
		m := map[string]bool{}
		for _, s := range srcs {
			m[s.Source] = s.Reprint
		}
		return m
	}

	// 逐字原文 + 电头差异(实测 J=1.000)。
	orig := "【长鑫科技：第五代工艺技术平台实现量产】财联社9月20日电，长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产。"
	copyNoDateline := "【长鑫科技：第五代工艺技术平台实现量产】长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产。"
	independent := "宇树科技发布Dex5-S灵巧手"

	// withCluster 的事件 ID 是 "e1";把原创那家的 EventID 设成 "e1" 才是 canonical(原发不误标)。
	srcs3 := []store.ClusterSource{
		{EventID: "e1", Source: "东方财富", Content: ptr(orig)},
		cs("财联社", ptr(independent)),
		cs("金十数据", ptr(copyNoDateline)),
	}
	got := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{cid: srcs3}, nil)
	if got.SourceCount != 3 {
		t.Errorf("机构数应为 3, got %d", got.SourceCount)
	}
	if got.IndependentCount != 2 {
		t.Errorf("独立来源数应为 2(2 家近逐字算 1), got %d", got.IndependentCount)
	}
	f := reprintOf(got.ClusterSources)
	if f["东方财富"] {
		t.Errorf("原发(=canonical)不得标转载(「N 家为转载」须与标记数对得上): %v", f)
	}
	if !f["金十数据"] {
		t.Errorf("近逐字的转载那家应标转载: %v", f)
	}
	if f["财联社"] {
		t.Errorf("独立改写的一家**不得**标转载(误判只可能出在这里): %v", f)
	}

	// 纯转载簇:2 家同文 ⇒ 独立来源数 1(转载不虚增渠道数),且只有转载那家被标。
	pure := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{
		cid: {
			{EventID: "e1", Source: "东方财富", Content: ptr(copyNoDateline)},
			cs("金十数据", ptr(orig)),
		},
	}, nil)
	if pure.SourceCount != 2 || pure.IndependentCount != 1 {
		t.Errorf("纯转载簇应为机构数 2 / 独立来源数 1, got %d / %d", pure.SourceCount, pure.IndependentCount)
	}
	if pf := reprintOf(pure.ClusterSources); !pf["金十数据"] || pf["东方财富"] {
		t.Errorf("纯转载簇只应标非原发那家: %v", pf)
	}

	// 无正文(历史数据/竞态)→ 无法判转载,独立来源数=机构数,不误伤。
	noContent := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{
		cid: {cs("东方财富", nil), cs("金十数据", nil)},
	}, nil)
	if noContent.IndependentCount != 2 {
		t.Errorf("无正文时独立来源数应=机构数 2(无从判转载), got %d", noContent.IndependentCount)
	}

	// 未聚类事件 → 1。
	if got := toEventItemLegacy(store.EventForAPI{ID: "e1", Title: "T"}, nil, nil, nil); got.IndependentCount != 1 {
		t.Errorf("未聚类事件独立来源数应为 1, got %d", got.IndependentCount)
	}
}

// event_conflicts 投影(issue #49 T3):
//   - 跨源簇内两家数字不一致 → 下发冲突,**带双方原文**;
//   - 单源事件不做跨源比对(无对象);
//   - 措辞不同但数字一致 → 不下发。
func TestToEventItemEventConflicts(t *testing.T) {
	cid := "cluster-1"
	withCluster := store.EventForAPI{ID: "e1", Title: "T", EventType: "company", Status: "extracted", ClusterID: &cid}
	srcs2 := map[string][]store.ClusterSource{
		cid: {{EventID: "e1", Source: "东方财富"}, {EventID: "e2", Source: "财联社"}},
	}
	facts := func(js string) store.ClusterMember {
		return store.ClusterMember{EventID: "x", Source: "s", Facts: []byte(js)}
	}

	// 冲突:减持 3% vs 2% → 检出 1 条,双方原文都在。
	got := toEventItemLegacy(withCluster, nil, srcs2, map[string][]store.ClusterMember{
		cid: {
			facts(`["公司股东拟减持不超过3%的股份"]`),
			facts(`["公司股东拟减持不超过2%的股份"]`),
		},
	})
	if len(got.EventConflicts) != 1 {
		t.Fatalf("应检出 1 条冲突, got %+v", got.EventConflicts)
	}
	c := got.EventConflicts[0]
	if c.Unit != "%" || len(c.Values) != 2 {
		t.Errorf("冲突单位/值错误: %+v", c)
	}
	if c.SentenceA == "" || c.SentenceB == "" {
		t.Errorf("必须带双方原文(红线:禁止静默择一): %+v", c)
	}

	// 措辞不同、数字一致 → 不报。
	noisy := toEventItemLegacy(withCluster, nil, srcs2, map[string][]store.ClusterMember{
		cid: {
			facts(`["公司股东拟减持不超过3%的股份"]`),
			facts(`["公司股东计划减持不超过3%的股份"]`),
		},
	})
	if len(noisy.EventConflicts) != 0 {
		t.Errorf("同数改写不得报冲突, got %+v", noisy.EventConflicts)
	}

	// 单源事件:无跨源比对对象 → 不报,且不 panic。
	single := toEventItemLegacy(withCluster, nil, map[string][]store.ClusterSource{
		cid: {{EventID: "e1", Source: "东方财富"}},
	}, map[string][]store.ClusterMember{
		cid: {facts(`["公司股东拟减持不超过3%的股份"]`)},
	})
	if len(single.EventConflicts) != 0 {
		t.Errorf("单源事件不得报跨源冲突, got %+v", single.EventConflicts)
	}
}

// P8 来源取 raw 层全集(issue #83 P-4 硬验收):
// 「某家报了这篇稿但**没被抽成事件**」时,事件层看不见它,raw 层必须仍列出它 ——
// 这正是 issue 说的「数据层合了多家、展示层回退到一家」的修法。
func TestToEventItemRawLayerSources(t *testing.T) {
	cid := "cluster-1"
	withCluster := store.EventForAPI{ID: "e1", Title: "T", EventType: "company", Status: "extracted", ClusterID: &cid}
	ptr := func(s string) *string { return &s }

	// 事件层只见 2 家(甲=canonical、乙=merged);raw 层多出「丙」(报了但未抽成事件)。
	urlA := "https://a.example/1"
	urlB := "https://b.example/1"
	urlC := "https://c.example/1"
	in := eventItemInput{
		clusters: map[string][]store.ClusterSource{
			cid: {{EventID: "e1", Source: "甲", URL: &urlA, Content: ptr("正文甲")}, {EventID: "e2", Source: "乙", URL: &urlB, Content: ptr("正文乙独立改写")}},
		},
		raw: map[string][]store.ClusterRawSource{
			cid: {
				{Source: "甲", URL: &urlA, IsRep: true, Canonical: "rep-1"},
				{Source: "乙", URL: &urlB, IsRep: true, Canonical: "rep-2"},
				{Source: "丙", URL: &urlC, IsRep: true, Canonical: "rep-1"}, // 事件层看不到的第三家
			},
		},
	}
	got := toEventItem(withCluster, in)

	// 计数仍走事件层:机构数 2(丙 未抽成事件,不计入印证度)。
	if got.SourceCount != 2 {
		t.Errorf("机构数应仍为事件层的 2(丙 未抽成事件,不计印证度), got %d", got.SourceCount)
	}
	// 但来源列表必须含 丙(P8 的核心)。
	bySrc := map[string]apiClusterSource{}
	for _, cs := range got.ClusterSources {
		bySrc[cs.Source] = cs
	}
	if _, ok := bySrc["丙"]; !ok {
		t.Fatalf("raw 层多出的机构「丙」必须在来源列表里(P8 硬验收): %+v", got.ClusterSources)
	}
	if bySrc["丙"].URL != urlC || len(bySrc["丙"].URLs) != 1 || bySrc["丙"].URLs[0] != urlC {
		t.Errorf("丙 的链接投影错误: %+v", bySrc["丙"])
	}
	// 无链接的源:如实空数组,绝不拼假链接。
	in2 := eventItemInput{
		clusters: map[string][]store.ClusterSource{cid: {{EventID: "e1", Source: "甲"}, {EventID: "e2", Source: "乙"}}},
		raw: map[string][]store.ClusterRawSource{cid: {
			{Source: "甲", Canonical: "r1", IsRep: true}, {Source: "乙", Canonical: "r2", IsRep: true},
		}},
	}
	got2 := toEventItem(withCluster, in2)
	for _, cs := range got2.ClusterSources {
		if len(cs.URLs) != 0 || cs.URL != "" {
			t.Errorf("无外链的源必须留空(urls=[], url=\"\"),不得造链接: %+v", cs)
		}
	}
}

// 同机构多条链接:全部如实列出(urls 全集),url 取首项(兼容旧契约)。
func TestToEventItemRawLayerMultipleURLs(t *testing.T) {
	cid := "cluster-1"
	withCluster := store.EventForAPI{ID: "e1", Title: "T", EventType: "company", Status: "extracted", ClusterID: &cid}
	u1, u2 := "https://a.example/1", "https://a.example/2"
	origin := "新华社"
	in := eventItemInput{
		clusters: map[string][]store.ClusterSource{cid: {
			{EventID: "e1", Source: "甲"}, {EventID: "e2", Source: "金十数据", Origin: &origin},
		}},
		raw: map[string][]store.ClusterRawSource{cid: {
			{Source: "甲", URL: &u1, IsRep: true, Canonical: "r1"},
			{Source: "金十数据", URL: &u2, Origin: &origin, IsRep: true, Canonical: "r2"},
		}},
	}
	got := toEventItem(withCluster, in)
	for _, cs := range got.ClusterSources {
		if cs.Source == "甲" && (len(cs.URLs) != 1 || cs.URL != u1) {
			t.Errorf("甲 应带 1 条链接: %+v", cs)
		}
		// 一级源名必须带出,供前端把「渠道 / 一级源」分区展示(金十的 url 就是一级源的链接)。
		if cs.Source == "金十数据" && cs.Origin != "新华社" {
			t.Errorf("金十的一级源名应带出(渠道与一级源分区展示): %+v", cs)
		}
	}
}

// 簇标题下发(issue #83 P-4 / P8):展示单元标题 = event_clusters.title,无簇/无标题则省略。
func TestToEventItemClusterTitle(t *testing.T) {
	cid := "cluster-1"
	withCluster := store.EventForAPI{ID: "e1", Title: "成员标题", EventType: "company", Status: "extracted", ClusterID: &cid}

	got := toEventItem(withCluster, eventItemInput{titles: map[string]string{cid: "簇展示标题"}})
	if got.ClusterTitle != "簇展示标题" {
		t.Errorf("簇标题应下发, got %q", got.ClusterTitle)
	}
	// 有簇但无标题 → 空(前端退回成员 title)。
	if got := toEventItem(withCluster, eventItemInput{}); got.ClusterTitle != "" {
		t.Errorf("无标题的簇,cluster_title 应为空, got %q", got.ClusterTitle)
	}
	// 未聚类 → 空。
	base := store.EventForAPI{ID: "e1", Title: "T"}
	if got := toEventItem(base, eventItemInput{titles: map[string]string{cid: "X"}}); got.ClusterTitle != "" {
		t.Errorf("未聚类事件不应有簇标题, got %q", got.ClusterTitle)
	}
}
