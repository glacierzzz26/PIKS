package web

// 自选(Watchlist)聚合端点(设计 frontend-ia §2.3)。
// GET /api/v1/watchlist —— 首页数据源:自选实体 + 持仓标记 + 快照盈亏,
// 一次请求喂满首页,避免对每只票打 N 次 /stock/:code。
//
// 自选语义复用 entities.status='watch'(零迁移);archived = 曾自选已移出(历史/深研/笔记保留)。

import (
	"net/http"

	"piks/internal/store"
)

type apiWatchItem struct {
	Code        string       `json:"code"`
	EntityID    string       `json:"entity_id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Held        bool         `json:"held"`
	Position    *apiPosition `json:"position"`
}

type apiWatchlist struct {
	Items []apiWatchItem `json:"items"`
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
	if positions, err := s.store.LatestPositions(ctx); err == nil {
		for _, p := range positions {
			posByCode[store.NormalizeCode(p.Code)] = toAPIPosition(p)
		}
	}

	items := make([]apiWatchItem, 0, len(ents))
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
		}
		items = append(items, it)
	}
	s.writeJSON(w, apiWatchlist{Items: items})
}
