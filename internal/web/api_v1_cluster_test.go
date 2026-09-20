package web

import (
	"testing"

	"piks/internal/store"
)

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

	ev := toEventItem(withCluster, nil, map[string][]store.ClusterSource{
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
	single := toEventItem(withCluster, nil, map[string][]store.ClusterSource{
		cid: {{EventID: "e1", Source: "东方财富"}},
	}, nil)
	if single.ClusterSources != nil {
		t.Fatalf("单源簇不应下发 cluster_sources, got %+v", single.ClusterSources)
	}

	// 未聚类(cluster_id NULL)。
	if got := toEventItem(base, nil, nil, nil); got.ClusterSources != nil {
		t.Fatalf("未聚类事件不应下发 cluster_sources, got %+v", got.ClusterSources)
	}

	// 簇 id 命中不到来源(数据竞态兜底)→ 不下发,不 panic。
	if got := toEventItem(withCluster, nil, nil, nil); got.ClusterSources != nil {
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
	if got := toEventItem(base, nil, nil, nil); got.SourceCount != 1 {
		t.Errorf("未聚类事件 source_count 应为 1, got %d", got.SourceCount)
	}
	// 单源簇 → 1。
	if got := toEventItem(withCluster, nil, map[string][]store.ClusterSource{
		cid: {{EventID: "e1", Source: "东方财富"}},
	}, nil); got.SourceCount != 1 {
		t.Errorf("单源簇 source_count 应为 1, got %d", got.SourceCount)
	}
	// 跨源簇 → 机构数。
	if got := toEventItem(withCluster, nil, map[string][]store.ClusterSource{
		cid: {
			{EventID: "e1", Source: "东方财富"},
			{EventID: "e2", Source: "金十数据"},
			{EventID: "e3", Source: "财联社"},
		},
	}, nil); got.SourceCount != 3 {
		t.Errorf("跨源簇 source_count 应为 3, got %d", got.SourceCount)
	}
	// 簇内无来源记录(竞态)→ 回落 1,不 panic、不虚报。
	if got := toEventItem(withCluster, nil, nil, nil); got.SourceCount != 1 {
		t.Errorf("无来源映射时 source_count 应回落 1, got %d", got.SourceCount)
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
	got := toEventItem(withCluster, nil, srcs2, map[string][]store.ClusterMember{
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
	noisy := toEventItem(withCluster, nil, srcs2, map[string][]store.ClusterMember{
		cid: {
			facts(`["公司股东拟减持不超过3%的股份"]`),
			facts(`["公司股东计划减持不超过3%的股份"]`),
		},
	})
	if len(noisy.EventConflicts) != 0 {
		t.Errorf("同数改写不得报冲突, got %+v", noisy.EventConflicts)
	}

	// 单源事件:无跨源比对对象 → 不报,且不 panic。
	single := toEventItem(withCluster, nil, map[string][]store.ClusterSource{
		cid: {{EventID: "e1", Source: "东方财富"}},
	}, map[string][]store.ClusterMember{
		cid: {facts(`["公司股东拟减持不超过3%的股份"]`)},
	})
	if len(single.EventConflicts) != 0 {
		t.Errorf("单源事件不得报跨源冲突, got %+v", single.EventConflicts)
	}
}
