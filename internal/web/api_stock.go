package web

// 个股中心聚合端点(设计 frontend-ia §2.4)。
// GET /api/v1/stock/:code —— 一站返回某只票在我持有的/我判断的/外部信息三层的数据。
//
// 原则:以 code 为主键,entity 为可选富化。查不到公司实体时,持仓/深研/涨停照常返回,
// 事件/笔记/行业如实空。不做 GET 触发的实体懒创建(那会污染实体库)。

import (
	"context"
	"net/http"
	"strings"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// ==================== DTO(对齐 frontend/src/lib/types.ts 的 StockHub)====================

type apiStockEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Description string `json:"description"`
}

type apiStockIndustry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type apiStockEvent struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	EventType  string  `json:"event_type"`
	OccurredAt string  `json:"occurred_at"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

type apiStockNote struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type apiStockHub struct {
	Code     string                  `json:"code"`
	Symbol   string                  `json:"symbol"`
	Entity   *apiStockEntity         `json:"entity"`
	Industry *apiStockIndustry       `json:"industry"`
	Position *apiPosition            `json:"position"`
	Trades   []apiTrade              `json:"trades"`
	Research []apiResearchRunSummary `json:"research"`
	Events   []apiStockEvent         `json:"events"`
	Notes    []apiStockNote          `json:"notes"`
	LimitUps []string                `json:"limit_ups"`
	// Decisions 每笔交易的决策关联(P6-4「当时在看什么」):trade_id → 研报/事件/笔记。
	// 只含有边且目标可解析的交易;无关联的交易不出现在此 map(前端如实空态)。
	Decisions map[string][]apiDecisionRef `json:"decisions"`
}

// ==================== 路由 ====================

// GET /api/v1/stock/:code —— 个股中心聚合(单次请求,6 节)。
func (s *Server) handleAPIStock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET")
		return
	}
	code := store.NormalizeCode(strings.TrimPrefix(r.URL.Path, "/api/v1/stock/"))
	if code == "" || strings.Contains(code, "/") {
		apiErrJSON(w, http.StatusBadRequest, "缺少股票代码")
		return
	}
	ctx := r.Context()

	out := apiStockHub{
		Code:      code,
		Symbol:    stockSymbol(code),
		Trades:    []apiTrade{},
		Research:  []apiResearchRunSummary{},
		Events:    []apiStockEvent{},
		Notes:     []apiStockNote{},
		LimitUps:  []string{},
		Decisions: map[string][]apiDecisionRef{},
	}

	// 1. 公司实体(code → entity;可为空,后续事件/笔记/行业依赖它)。
	ent, err := s.store.GetCompanyEntityByCode(ctx, code)
	if err != nil {
		s.apiErr(w, "stock", err)
		return
	}
	if ent != nil {
		st := ent.Status
		if st == "" {
			st = "active"
		}
		out.Entity = &apiStockEntity{
			ID: ent.ID, Name: ent.Name, Status: st, Description: orStr(ent.Description, ""),
		}

		// 行业/概念归位(company → industry 关系)。
		rels, err := s.store.ListEntityRelationships(ctx, ent.ID)
		if err != nil {
			s.apiErr(w, "stock", err)
			return
		}
		out.Industry = firstIndustry(ctx, s.store, rels, ent.ID)

		// 相关事件(affects 到该公司实体)。
		refs, err := s.store.ListEventsAffectingEntities(ctx, []string{ent.ID})
		if err != nil {
			s.apiErr(w, "stock", err)
			return
		}
		ids := make([]string, 0, len(refs))
		for _, ref := range refs {
			ids = append(ids, ref.ID)
		}
		evs, err := s.store.ListEventsByIDs(ctx, ids)
		if err != nil {
			s.apiErr(w, "stock", err)
			return
		}
		// 时间倒序(新→旧):ListEventsByIDs 升序,反转。
		for i := len(evs) - 1; i >= 0; i-- {
			ev := evs[i]
			out.Events = append(out.Events, apiStockEvent{
				ID:         ev.ID,
				Title:      ev.Title,
				EventType:  ev.EventType,
				OccurredAt: fmtRFC3339(eventTime(ev)),
				Confidence: ev.Confidence,
				Source:     ev.SourceName,
			})
		}

		// 我的笔记(引用了该实体)。
		notes, err := s.store.ListNotesReferencingEntity(ctx, ent.ID)
		if err != nil {
			s.apiErr(w, "stock", err)
			return
		}
		for _, n := range notes {
			out.Notes = append(out.Notes, apiStockNote{
				ID: n.ID, Type: n.Type, Title: n.Title, Status: n.Status,
				UpdatedAt: fmtRFC3339(n.UpdatedAt),
			})
		}
	}

	// 2. 深研报告(只依赖 code,与实体无关)。
	runs, err := s.store.ListResearchRuns(ctx, code, 0)
	if err != nil {
		s.apiErr(w, "stock", err)
		return
	}
	names := s.researchNames(ctx, runs)
	for i := range runs {
		out.Research = append(out.Research, toSummary(&runs[i], names[runs[i].Code]))
	}

	// 3. 我的持仓与成交(只依赖 code)。
	pos, err := s.store.LatestPositionByCode(ctx, code)
	if err != nil {
		s.apiErr(w, "stock", err)
		return
	}
	if pos != nil {
		p := toAPIPosition(*pos)
		out.Position = &p
	}
	trades, err := s.store.ListTradesByCode(ctx, code, 0)
	if err != nil {
		s.apiErr(w, "stock", err)
		return
	}
	tids := make([]string, 0, len(trades))
	for _, t := range trades {
		tids = append(tids, t.ID)
	}
	decisions, err := s.decisionRefsForTrades(ctx, tids)
	if err != nil {
		s.apiErr(w, "stock", err)
		return
	}
	for _, t := range trades {
		tr := toAPITrade(t)
		if refs := decisions[t.ID]; len(refs) > 0 {
			tr.BasedOn = refs
			out.Decisions[t.ID] = refs
		}
		out.Trades = append(out.Trades, tr)
	}

	// 4. 涨停记录(只依赖 code)。
	zt, err := s.store.ListZTAppearances(ctx, code)
	if err != nil {
		s.apiErr(w, "stock", err)
		return
	}
	for _, d := range zt {
		out.LimitUps = append(out.LimitUps, d.In(cst).Format("2006-01-02"))
	}

	s.writeJSON(w, out)
}

// ==================== 小工具 ====================

// stockSymbol 6 位代码 → research 风格 full_code(前缀 sh/sz/bj)。
func stockSymbol(code string) string {
	if len(code) != 6 {
		return code
	}
	switch code[0] {
	case '6':
		return "sh" + code
	case '0', '3':
		return "sz" + code
	case '4', '8':
		return "bj" + code
	}
	return code
}

// eventTime 事件时间:优先 occurred_at,缺省用创建时间(与 toEventItem 一致)。
func eventTime(ev store.EventForAPI) (t time.Time) {
	if ev.OccurredAt != nil {
		return *ev.OccurredAt
	}
	return ev.CreatedAt
}

// firstIndustry 取该公司实体关系里第一条指向行业/概念实体的目标(公司→行业)。
// 关系任一方向命中即可;找不到返回 nil(如实空,不编造)。
func firstIndustry(ctx context.Context, st *store.Store, rels []model.Relationship, selfID string) *apiStockIndustry {
	for _, rel := range rels {
		var targetID string
		switch {
		case rel.FromType == "entity" && rel.FromID == selfID:
			if rel.ToType != "entity" {
				continue
			}
			targetID = rel.ToID
		case rel.ToType == "entity" && rel.ToID == selfID:
			if rel.FromType != "entity" {
				continue
			}
			targetID = rel.FromID
		default:
			continue
		}
		ent, err := st.GetEntityByID(ctx, targetID)
		if err != nil || ent == nil {
			continue
		}
		if ent.Type == "industry" || ent.Type == "concept" {
			return &apiStockIndustry{ID: ent.ID, Name: ent.Name}
		}
	}
	return nil
}
