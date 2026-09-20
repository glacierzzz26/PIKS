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
	})
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
	})
	if single.ClusterSources != nil {
		t.Fatalf("单源簇不应下发 cluster_sources, got %+v", single.ClusterSources)
	}

	// 未聚类(cluster_id NULL)。
	if got := toEventItem(base, nil, nil); got.ClusterSources != nil {
		t.Fatalf("未聚类事件不应下发 cluster_sources, got %+v", got.ClusterSources)
	}

	// 簇 id 命中不到来源(数据竞态兜底)→ 不下发,不 panic。
	if got := toEventItem(withCluster, nil, nil); got.ClusterSources != nil {
		t.Fatalf("无来源映射时不应下发, got %+v", got.ClusterSources)
	}
}
