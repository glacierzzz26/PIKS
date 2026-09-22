package web

// GET /api/v1/hot-topics —— 热榜快照(issue #68 D 层)。
//
// 🔴 隔离红线(设计 §6):热度**可被操纵**,只作展示,绝不作为「重要性」判定,
//   也**不与印证度(source_count/cluster_sources)合并计算**。故本接口:
//     - 只读独立表 hot_topic_items —— 与 events/raw_documents **零交集**;
//     - **两源各出各的、分列下发,不合并、不加权、不排名**(实测两源同题对 = 0、粒度不同,
//       混排必然误导 —— 方案 A,见 docs/phase11/design/hot-topic.md)。
//
// 响应形状:{"sources":[{key,name,note,items:[{rank,title,hot_value,url}]}], "snapshot_at"}。
// 按源分列而非拍平,是为了让前端**天然无法**跨源混排(结构上杜绝错误展示)。
// ⚠️ `extra`(上游原始字段)落库留档但**不下发** —— 当前无消费方,不下发未成形的字段。
//
// ⚠️ hot_value 仅**源内可比**:同花顺 hot_value 与财联社 readNum 量纲不同,且各源自身
//   尺度也不同(财联社榜内自己就相差 9 倍)。前端须如实标注「仅源内可比」。
// ⚠️ 热度可为 null(上游未给)—— NULL ≠ 0,前端不填 0、如实不显示。

import (
	"net/http"

	"piks/internal/collector"
)

// hotSourceMeta 前端展示用的源元信息(显示名与口径说明,**单一真源在此**)。
// 与 collector.HotSource* 枚举一一对应;新增源须同步此处。
var hotSourceMeta = []struct {
	Key  string
	Name string
	Note string // 口径说明 —— 如实告诉用户这是哪家的什么榜
}{
	{
		Key:  collector.HotSourceThs,
		Name: "同花顺话题榜",
		Note: "同花顺用户讨论话题,按站内热度排序;含关联个股。仅本榜内可比。",
	},
	{
		Key:  collector.HotSourceCLS,
		Name: "财联社热文",
		Note: "财联社首页热文,按阅读数排序(文章/复盘,非单一事件)。仅本榜内可比。",
	},
}

type apiHotTopicItem struct {
	Rank     int    `json:"rank"`
	Title    string `json:"title"`
	HotValue *int64 `json:"hot_value"` // null = 上游未给(≠0,前端不填 0)
	URL      string `json:"url,omitempty"`
}

type apiHotTopicSource struct {
	Key   string            `json:"key"`
	Name  string            `json:"name"`
	Note  string            `json:"note"`
	Items []apiHotTopicItem `json:"items"`
}

type apiHotTopics struct {
	Sources []apiHotTopicSource `json:"sources"`
	// SnapshotAt 本批快照时刻(取所有源里最新的一个,仅供展示「数据到几点」;
	// 两源各自真实采集时刻可能差几秒,此处不假装它们同一时刻)。
	SnapshotAt string `json:"snapshot_at"`
}

// handleAPIHotTopics GET /api/v1/hot-topics
func (s *Server) handleAPIHotTopics(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListHotTopicLatest(r.Context())
	if err != nil {
		s.apiErr(w, "hot-topics", err)
		return
	}

	// 按 source 分桶 —— 即使查询返回多源混合顺序,也各归各列(结构上杜绝混排)。
	byKey := map[string][]apiHotTopicItem{}
	var latest string
	for _, it := range rows {
		byKey[it.Source] = append(byKey[it.Source], apiHotTopicItem{
			Rank:     it.Rank,
			Title:    it.Title,
			HotValue: it.HotValue,
			URL:      orStr(it.URL, ""),
		})
		if ts := it.SnapshotAt.In(cst).Format("2006-01-02 15:04"); ts > latest {
			latest = ts
		}
	}

	// 固定源顺序下发(即使某源本轮无数据也占位,前端可如实显示「该源暂无数据」,
	// 而不是整列消失 —— 少一列会让用户以为这个榜不存在)。
	out := make([]apiHotTopicSource, 0, len(hotSourceMeta))
	for _, m := range hotSourceMeta {
		items := byKey[m.Key]
		if items == nil {
			items = []apiHotTopicItem{}
		}
		out = append(out, apiHotTopicSource{Key: m.Key, Name: m.Name, Note: m.Note, Items: items})
	}
	s.writeJSON(w, apiHotTopics{Sources: out, SnapshotAt: latest})
}
