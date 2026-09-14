package web

// 自选(Watchlist)聚合端点(设计 frontend-ia §2.3 / phase6 ux-ia §2 区块3)。
// GET /api/v1/watchlist —— 首页数据源:自选实体 + 持仓标记 + 快照盈亏 + 富化徽标,
// 一次请求喂满首页,避免对每只票打 N 次 /stock/:code。
//
// 自选语义复用 entities.status='watch'(零迁移);archived = 曾自选已移出(历史/深研/笔记保留)。
//
// P6-3 富化(零 schema):每项加
//   latest_event  —— 最近一条 affects 到该实体的事件(标题/发生日/累计条数/深链 id)
//   position_date —— 持仓快照日(盈亏以此日为准,首页须标注「截至」)
//   has_research / latest_research_asof —— 是否深研过 / 最近一次深研日
// 复用现成 read 查询 + 一个批量 ListEventsByEntityIDs,无 schema 变更。

import (
	"context"
	"net/http"
	"sort"

	"piks/internal/store"
)

type apiWatchEvent struct {
	Title    string `json:"title"`
	Date     string `json:"date"`      // 事件发生日
	Count    int    `json:"count"`     // affects 到该实体的累计事件数
	LatestID string `json:"latest_id"` // 最近事件 id(深链跳详情)
}

type apiWatchItem struct {
	Code        string       `json:"code"`
	EntityID    string       `json:"entity_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Held        bool         `json:"held"`
	Position    *apiPosition `json:"position"`
	// 富化(P6-3)
	LatestEvent        *apiWatchEvent `json:"latest_event"`
	PositionDate       string         `json:"position_date"`        // 持仓快照日(空=无持仓)
	HasResearch        bool           `json:"has_research"`
	LatestResearchAsOf string         `json:"latest_research_asof"` // YYYY-MM-DD(空=未深研)
}

type apiWatchlist struct {
	Items []apiWatchItem `json:"items"`
	// 快照日(全部持仓共用一个最新快照日;空=无持仓)。首页整体标注「截至」用。
	PositionDate string `json:"position_date"`
	// 覆盖率:自选中做过深研的数量(「组合 N 只,其中 M 只做过深研」)。
	Researched int `json:"researched"`
}

// GET /api/v1/watchlist
func (s *Server) handleAPIWatchlist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ents, err := s.store.ListEntitiesByStatus(ctx, "watch")
	if err != nil {
		s.apiErr(w, "watchlist", err)
		return
	}

	// 现价/盈亏来自最新持仓快照:建 code → position 索引(O(n),非 N 次查询)。
	posByCode := map[string]apiPosition{}
	posDate := ""
	if positions, err := s.store.LatestPositions(ctx); err == nil {
		for _, p := range positions {
			posByCode[store.NormalizeCode(p.Code)] = toAPIPosition(p)
			if d := p.SnapshotDate.In(cst).Format("2006-01-02"); d > posDate {
				posDate = d
			}
		}
	}

	// 事件富化:一次批量查全部自选实体的 affects 事件,再按实体分组(时间倒序)。
	entIDs := make([]string, 0, len(ents))
	for _, e := range ents {
		entIDs = append(entIDs, e.ID)
	}
	evByEnt := s.eventsByEntity(ctx, entIDs)

	// 深研富化:一次批量取全部 run(空 code 取全量),按 code 取最新 as_of。
	researchAsOf := map[string]string{}
	if runs, err := s.store.ListResearchRuns(ctx, "", 0); err == nil {
		for _, run := range runs {
			code := store.NormalizeCode(run.Code)
			if code == "" {
				continue
			}
			if d := run.AsOf.In(cst).Format("2006-01-02"); d > researchAsOf[code] {
				researchAsOf[code] = d
			}
		}
	}

	items := make([]apiWatchItem, 0, len(ents))
	researched := 0
	for _, e := range ents {
		code := entityCode(e)
		it := apiWatchItem{
			Code:        code,
			EntityID:    e.ID,
			Name:        e.Name,
			Description: orStr(e.Description, ""),
		}
		if p, ok := posByCode[code]; ok && code != "" {
			pc := p
			it.Held = true
			it.Position = &pc
			it.PositionDate = posDate
		}
		if evs := evByEnt[e.ID]; len(evs) > 0 {
			latest := evs[0]
			latest.Count = len(evs)
			it.LatestEvent = &latest
		}
		if d, ok := researchAsOf[code]; ok && code != "" {
			it.HasResearch = true
			it.LatestResearchAsOf = d
			researched++
		}
		items = append(items, it)
	}
	// 稳定排序:有新消息 / 未深研的排前,便于首页「今天该看什么」取 Top。
	sort.SliceStable(items, func(i, j int) bool { return watchNeedRank(items[i]) < watchNeedRank(items[j]) })

	s.writeJSON(w, apiWatchlist{Items: items, PositionDate: posDate, Researched: researched})
}

// eventsByEntity 按实体分组 affects 事件(时间倒序)。零 schema 复用 relationships。
func (s *Server) eventsByEntity(ctx context.Context, entIDs []string) map[string][]apiWatchEvent {
	refs, err := s.store.ListEventsByEntityIDs(ctx, entIDs)
	if err != nil {
		return map[string][]apiWatchEvent{}
	}
	return groupWatchEvents(refs)
}

// groupWatchEvents 纯函数:把 (实体,事件) 明细按实体分组,每组按日期倒序。
// 抽出来便于 DB-free 单测(与 handleAPIWatchlist 的富化路径共享)。
func groupWatchEvents(refs []store.WatchEventRef) map[string][]apiWatchEvent {
	out := map[string][]apiWatchEvent{}
	for _, r := range refs {
		out[r.EntityID] = append(out[r.EntityID], apiWatchEvent{
			Title:    r.Title,
			Date:     r.OccurredAt.In(cst).Format("2006-01-02"),
			LatestID: r.EventID,
		})
	}
	for id, evs := range out {
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].Date > evs[j].Date })
		out[id] = evs
	}
	return out
}

// watchNeedRank 首页「需要你处理的」排序权重(小者在前):越缺关注越靠前。
func watchNeedRank(it apiWatchItem) int {
	switch {
	case it.LatestEvent != nil:
		return 0 // 有新消息 → 最该看
	case !it.HasResearch && it.Code != "":
		return 1 // 还没深研过
	default:
		return 2
	}
}
