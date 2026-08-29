package web

// /api/v1 只读投影层 —— React 前端(frontend/)数据源。
//
// 只读、复用 store 查询、字段对齐 frontend/src/lib/types.ts;
// 未产出的字段如实给零值/省略(数据诚实),不做猜测映射。
// 列表端点返回全量,分页由前端客户端完成(page/size 只在 URL)。

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"piks/internal/model"
	"piks/internal/store"
)

// ---- 映射类型(对齐前端 types.ts) ----

type apiEventItem struct {
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	EventType  string        `json:"event_type"`
	Summary    string        `json:"summary"`
	Facts      []string      `json:"facts"`
	Affected   []apiAffected `json:"affected"`
	OccurredAt string        `json:"occurred_at"`
	Confidence float64       `json:"confidence"`
	Status     string        `json:"status"`
	Source     string        `json:"source"`
	SourceURL  *string       `json:"source_url,omitempty"`
}

type apiAffected struct {
	Word       string `json:"word"`
	EntityID   string `json:"entity_id,omitempty"`
	EntityName string `json:"entity_name,omitempty"`
}

type apiEntity struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	UpdatedAt   string   `json:"updated_at"`
}

type apiRelationship struct {
	ID         string  `json:"id"`
	FromID     string  `json:"from_id"`
	ToID       string  `json:"to_id"`
	RelType    string  `json:"rel_type"`
	Confidence float64 `json:"confidence"`
}

type apiIndex struct {
	Name      string  `json:"name"`
	Close     float64 `json:"close"`
	ChangePct float64 `json:"change_pct"`
}

type apiLimitUpStock struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Boards     int     `json:"boards"`
	SealAmount float64 `json:"seal_amount"`
	FirstTime  string  `json:"first_time"`
	Industry   string  `json:"industry"`
	Reason     string  `json:"reason"`
	Turnover   float64 `json:"turnover"`
	FloatMv    float64 `json:"float_mv"`
}

type apiIndustryDist struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type apiMarketSnapshot struct {
	TradeDate    string            `json:"trade_date"`
	Indices      []apiIndex        `json:"indices"`
	LimitUp      int               `json:"limit_up"`
	LimitDown    int               `json:"limit_down"`
	BrokenLimit  int               `json:"broken_limit"`
	MaxBoard     int               `json:"max_board"`
	TurnoverYi   float64           `json:"turnover_yi"`
	EmotionScore float64           `json:"emotion_score"`
	EmotionState string            `json:"emotion_state"`
	Ladder       []apiLimitUpStock `json:"ladder"`
	IndustryDist []apiIndustryDist `json:"industry_dist"`
}

type apiFlash struct {
	ID        string `json:"id"`
	Time      string `json:"time"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	Important bool   `json:"important"`
	EventID   string `json:"event_id,omitempty"`
}

type apiDoc struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
	Content   string `json:"content"`
}

// ---- handlers ----

// GET /api/v1/events?type&status&q —— 结构化事件流。
func (s *Server) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	evs, err := s.store.ListEventsForAPI(ctx)
	if err != nil {
		s.apiErr(w, "events", err)
		return
	}
	ents, err := s.store.ListAllEntities(ctx)
	if err != nil {
		s.apiErr(w, "entities", err)
		return
	}
	idx := buildNameIndex(ents)

	typ := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	out := make([]apiEventItem, 0, len(evs))
	for _, ev := range evs {
		if typ != "" && ev.EventType != typ {
			continue
		}
		if !eventStatusOK(ev.Status, status) {
			continue
		}
		if q != "" && !strSub(q, ev.Title, orStr(ev.Summary, "")) {
			continue
		}
		out = append(out, toEventItem(ev, idx))
	}
	s.writeJSON(w, out)
}

// GET /api/v1/entities?type&q —— 统一实体。
func (s *Server) handleAPIEntities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typ := r.URL.Query().Get("type")
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	var ents []model.Entity
	var err error
	if typ != "" {
		ents, err = s.store.ListEntitiesByType(ctx, typ)
	} else {
		ents, err = s.store.ListAllEntities(ctx)
	}
	if err != nil {
		s.apiErr(w, "entities", err)
		return
	}

	out := make([]apiEntity, 0, len(ents))
	for _, e := range ents {
		if q != "" && !strSub(q, e.Name, orStr(e.Description, "")) {
			continue
		}
		out = append(out, toEntity(e))
	}
	s.writeJSON(w, out)
}

// GET /api/v1/relationships —— 实体关系(全量,前端按节点集合过滤)。
func (s *Server) handleAPIRelationships(w http.ResponseWriter, r *http.Request) {
	rels, err := s.store.ListAllRelationships(r.Context())
	if err != nil {
		s.apiErr(w, "relationships", err)
		return
	}
	out := make([]apiRelationship, 0, len(rels))
	for _, rel := range rels {
		c := 0.0
		if rel.Confidence != nil {
			c = *rel.Confidence
		}
		out = append(out, apiRelationship{
			ID: rel.ID, FromID: rel.FromID, ToID: rel.ToID,
			RelType: rel.RelType, Confidence: c,
		})
	}
	s.writeJSON(w, out)
}

// GET /api/v1/market/snapshot?date —— 市场状态快照(含涨停池)。无 date 取最新。
func (s *Server) handleAPIMarketSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dateQ := r.URL.Query().Get("date")

	var snap *model.MarketSnapshot
	if dateQ != "" {
		t, err := time.ParseInLocation("2006-01-02", dateQ, cst)
		if err != nil {
			http.Error(w, "date 需为 YYYY-MM-DD", http.StatusBadRequest)
			return
		}
		snap, err = s.store.GetMarketSnapshotByDate(ctx, t)
		if err != nil {
			s.apiErr(w, "market snapshot", err)
			return
		}
	} else {
		recent, err := s.store.ListMarketSnapshots(ctx, 1)
		if err != nil {
			s.apiErr(w, "market snapshot", err)
			return
		}
		if len(recent) > 0 {
			snap = &recent[0]
		}
	}
	if snap == nil {
		http.Error(w, "无该日期快照", http.StatusNotFound)
		return
	}
	s.writeJSON(w, toSnapshot(snap))
}

// GET /api/v1/flashes?q&source —— 快讯流(raw_documents 投影)。
func (s *Server) handleAPIFlashes(w http.ResponseWriter, r *http.Request) {
	flashes, err := s.store.ListRawDocumentsWithSource(r.Context())
	if err != nil {
		s.apiErr(w, "flashes", err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	src := r.URL.Query().Get("source")

	out := make([]apiFlash, 0, len(flashes))
	for _, f := range flashes {
		if src != "" && f.Source != src {
			continue
		}
		if q != "" && !strSub(q, f.Title) {
			continue
		}
		out = append(out, toFlash(f))
	}
	s.writeJSON(w, out)
}

// GET /api/v1/notes?type —— 笔记列表(personal_notes 投影)。
// 类型过滤按后端实际 type 值(note/belief/case/mistake);daily-review/weekly 无对应存储,如实为空。
func (s *Server) handleAPINotes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	notes, err := s.store.ListPersonalNotes(ctx, r.URL.Query().Get("type"))
	if err != nil {
		s.apiErr(w, "notes", err)
		return
	}
	out := make([]apiDoc, 0, len(notes))
	for _, n := range notes {
		out = append(out, toDoc(n))
	}
	s.writeJSON(w, out)
}

// GET /api/v1/notes/:id —— 单篇笔记。
func (s *Server) handleAPINote(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/notes/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	n, err := s.store.GetPersonalNote(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
			// personal_notes 无此 id → 回退周报(周报阅读页经 /notes/:id 读 weekly_summaries)
			if wk, werr := s.store.GetWeeklySummaryByID(r.Context(), id); werr == nil && wk != nil {
				s.writeJSON(w, toWeeklyDoc(*wk))
				return
			}
			http.NotFound(w, r) // 无该 id / id 非法 → 404
			return
		}
		s.apiErr(w, "note", err)
		return
	}
	s.writeJSON(w, toDoc(*n))
}

// ---- 类型映射 ----

// buildNameIndex 实体名/别名 → 实体索引(事件 affected 词 → 实体链接)。
type nameRef struct{ id, name string }

func buildNameIndex(ents []model.Entity) map[string]nameRef {
	idx := make(map[string]nameRef, len(ents)*2)
	for _, e := range ents {
		add := func(k string) {
			if k == "" {
				return
			}
			if _, ok := idx[k]; !ok {
				idx[k] = nameRef{e.ID, e.Name}
			}
		}
		add(e.Name)
		var aliases []string
		_ = json.Unmarshal(e.Aliases, &aliases)
		for _, a := range aliases {
			add(a)
		}
	}
	return idx
}

func toEventItem(ev store.EventForAPI, idx map[string]nameRef) apiEventItem {
	at := ev.CreatedAt
	if ev.OccurredAt != nil {
		at = *ev.OccurredAt
	}
	var facts []string
	_ = json.Unmarshal(ev.Facts, &facts)
	var words []string
	_ = json.Unmarshal(ev.Affected, &words)
	affected := make([]apiAffected, 0, len(words))
	for _, w := range words {
		af := apiAffected{Word: w}
		if ref, ok := idx[w]; ok {
			af.EntityID = ref.id
			af.EntityName = ref.name
		}
		affected = append(affected, af)
	}
	return apiEventItem{
		ID:         ev.ID,
		Title:      ev.Title,
		EventType:  ev.EventType,
		Summary:    orStr(ev.Summary, ""),
		Facts:      facts,
		Affected:   affected,
		OccurredAt: fmtRFC3339(at),
		Confidence: ev.Confidence,
		Status:     eventStatusFront(ev.Status),
		Source:     ev.SourceName,
		SourceURL:  ev.SourceURL,
	}
}

// eventStatusFront 后端知识状态 → 前端展示状态(confirmed/pending)。
func eventStatusFront(backend string) string {
	switch backend {
	case "verified", "published":
		return "confirmed"
	case "extracted":
		return "pending"
	case "merged":
		return "archived"
	default:
		return backend
	}
}

// eventStatusOK 前端筛选状态匹配:confirmed → verified/published,pending → extracted。
func eventStatusOK(backend, filter string) bool {
	switch filter {
	case "", backend:
		return true
	case "confirmed":
		return backend == "verified" || backend == "published"
	case "pending":
		return backend == "extracted"
	default:
		return false
	}
}

func toEntity(e model.Entity) apiEntity {
	var aliases []string
	_ = json.Unmarshal(e.Aliases, &aliases)
	st := e.Status
	if st == "" {
		st = "active"
	}
	return apiEntity{
		ID:          e.ID,
		Type:        e.Type,
		Name:        e.Name,
		Aliases:     aliases,
		Description: orStr(e.Description, ""),
		Status:      st,
		UpdatedAt:   fmtRFC3339(e.UpdatedAt),
	}
}

func toSnapshot(snap *model.MarketSnapshot) apiMarketSnapshot {
	out := apiMarketSnapshot{
		TradeDate:    snap.TradeDate.In(cst).Format("2006-01-02"),
		Indices:      []apiIndex{},
		LimitUp:      intPtrVal(snap.LimitUpCount),
		LimitDown:    intPtrVal(snap.LimitDownCount),
		BrokenLimit:  intPtrVal(snap.BrokenLimitCount),
		MaxBoard:     intPtrVal(snap.MaxBoard),
		TurnoverYi:   fPtrVal(snap.TurnoverAmt),
		EmotionScore: fPtrVal(snap.EmotionScore),
		EmotionState: orStr(snap.EmotionState, ""),
		Ladder:       []apiLimitUpStock{},
		IndustryDist: []apiIndustryDist{},
	}

	// 指数 index_json:{"sh":{close,pct},"sz":{...},"cyb":{...}}
	type idxVal struct {
		Close float64 `json:"close"`
		Pct   float64 `json:"pct"`
	}
	var idxMap map[string]idxVal
	_ = json.Unmarshal(snap.IndexJSON, &idxMap)
	for _, k := range []struct{ key, name string }{
		{"sh", "上证指数"}, {"sz", "深证成指"}, {"cyb", "创业板指"},
	} {
		if v, ok := idxMap[k.key]; ok {
			out.Indices = append(out.Indices, apiIndex{Name: k.name, Close: v.Close, ChangePct: v.Pct})
		}
	}

	// 涨停池 zt_pool:[{code,name,zdp,lbc,fund,hybk}]
	// → ladder。first_time/reason/turnover/float_mv 源数据未采集 → 如实零值。
	type ztItem struct {
		Code string  `json:"code"`
		Name string  `json:"name"`
		Lbc  int     `json:"lbc"`
		Fund float64 `json:"fund"`
		Hybk string  `json:"hybk"`
	}
	var pool []ztItem
	_ = json.Unmarshal(snap.ZTPool, &pool)
	for _, z := range pool {
		out.Ladder = append(out.Ladder, apiLimitUpStock{
			Code:       z.Code,
			Name:       z.Name,
			Boards:     z.Lbc,
			SealAmount: z.Fund,
			Industry:   z.Hybk,
		})
	}

	// 行业分布 industry_dist:{"家居用品":N} → [{name,count}],count 倒序。
	var dist map[string]int
	_ = json.Unmarshal(snap.IndustryDist, &dist)
	for name, count := range dist {
		out.IndustryDist = append(out.IndustryDist, apiIndustryDist{Name: name, Count: count})
	}
	sort.Slice(out.IndustryDist, func(i, j int) bool {
		if out.IndustryDist[i].Count != out.IndustryDist[j].Count {
			return out.IndustryDist[i].Count > out.IndustryDist[j].Count
		}
		return out.IndustryDist[i].Name < out.IndustryDist[j].Name
	})

	return out
}

func toFlash(f store.RawDocWithSource) apiFlash {
	return apiFlash{
		ID:        f.ID,
		Time:      f.FlashAt.In(cst).Format("2006-01-02 15:04"),
		Content:   f.Title,
		Source:    f.Source,
		Important: f.EventID != nil, // 已被抽取成事件 → 高亮(近似,源无独立标记)
		EventID:   orStr(f.EventID, ""),
	}
}

// toWeeklyDoc 周综述 → Doc 形状(周报列表 + /notes/:id 阅读回退共用)。
func toWeeklyDoc(w store.WeeklySummary) apiDoc {
	return apiDoc{
		ID:        w.ID,
		Type:      "weekly",
		Slug:      w.Week,
		Title:     "周报 · " + w.Week,
		UpdatedAt: fmtRFC3339(w.UpdatedAt),
		Content:   w.Summary,
	}
}

func toDoc(n model.PersonalNote) apiDoc {
	return apiDoc{
		ID:        n.ID,
		Type:      n.Type,
		Slug:      n.Slug,
		Title:     orStr(n.Title, ""),
		UpdatedAt: fmtRFC3339(n.UpdatedAt),
		Content:   orStr(n.Content, ""),
	}
}

// ---- 小工具 ----

// isInvalidUUID id 参数非合法 UUID(pg 报 22P02)时按"无此资源"处理。
func isInvalidUUID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02"
}

func intPtrVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func fPtrVal(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func fmtRFC3339(t time.Time) string { return t.In(cst).Format(time.RFC3339) }

// strSub 大小写不敏感的多字段子串匹配。
func strSub(q string, fields ...string) bool {
	q = strings.ToLower(q)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func (s *Server) apiErr(w http.ResponseWriter, what string, err error) {
	http.Error(w, what+": "+err.Error(), http.StatusInternalServerError)
}

// ---- Phase 2: 只读预览端点(dashboard/recon/reviews/trades/chat/settings/weekly) ----

type apiStatCard struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

type apiSnapRow struct {
	Date         string  `json:"date"`
	EmotionScore float64 `json:"emotion_score"`
	EmotionState string  `json:"emotion_state"`
	LimitUp      int     `json:"limit_up"`
	LimitDown    int     `json:"limit_down"`
	BrokenLimit  int     `json:"broken_limit"`
	MaxBoard     int     `json:"max_board"`
}

type apiTopEvent struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Score int    `json:"score"`
}

type apiTaskRun struct {
	Command string `json:"command"`
	Status  string `json:"status"`
	Time    string `json:"time"`
	Note    string `json:"note,omitempty"`
}

type apiDashboard struct {
	Stats       []apiStatCard     `json:"stats"`
	Market      apiMarketSnapshot `json:"market"`
	SnapHistory []apiSnapRow      `json:"snap_history"`
	Review      string            `json:"review"`
	TopEvents   []apiTopEvent     `json:"top_events"`
	TaskRuns    []apiTaskRun      `json:"task_runs"`
}

// GET /api/v1/dashboard —— 看板(统计 + 最新快照 + 历史情绪 + 每日复盘 + 管线)。
func (s *Server) handleAPIDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := s.store.Counts(ctx)
	if err != nil {
		s.apiErr(w, "dashboard", err)
		return
	}
	notes, trades, err := s.store.NoteTradeCounts(ctx)
	if err != nil {
		s.apiErr(w, "dashboard", err)
		return
	}
	snaps, err := s.store.ListMarketSnapshots(ctx, 6)
	if err != nil {
		s.apiErr(w, "dashboard", err)
		return
	}
	evs, err := s.store.ListEventsForAPI(ctx)
	if err != nil {
		s.apiErr(w, "dashboard", err)
		return
	}
	runs, err := s.store.ListTaskRuns(ctx, 6)
	if err != nil {
		s.apiErr(w, "dashboard", err)
		return
	}

	out := apiDashboard{
		Stats: []apiStatCard{
			{Label: "结构化事件", Value: c.Events},
			{Label: "统一实体", Value: c.Entities},
			{Label: "知识笔记", Value: notes},
			{Label: "交易记录", Value: trades},
		},
	}
	if len(snaps) > 0 {
		out.Market = toSnapshot(&snaps[0])
		out.SnapHistory = toSnapHistory(snaps)
		out.Review = reviewMarkdown(&snaps[0], evs)
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].Confidence > evs[j].Confidence })
	for i := 0; i < len(evs) && i < 6; i++ {
		out.TopEvents = append(out.TopEvents, apiTopEvent{
			ID: evs[i].ID, Title: evs[i].Title,
			Score: int(math.Round(evs[i].Confidence * 100)),
		})
	}
	for _, rn := range runs {
		out.TaskRuns = append(out.TaskRuns, toTaskRun(rn))
	}
	s.writeJSON(w, out)
}

// reviewMarkdown 从最新快照生成"每日复盘"Markdown(全部真实数字,模板化投影,不虚构)。
func reviewMarkdown(snap *model.MarketSnapshot, evs []store.EventForAPI) string {
	var b strings.Builder
	b.WriteString("## 市场概况\n\n")
	b.WriteString(fmt.Sprintf("- 情绪分 **%.0f**（%s）\n", fPtrVal(snap.EmotionScore), orStr(snap.EmotionState, "-")))
	b.WriteString(fmt.Sprintf("- 涨停 %d 家 / 跌停 %d 家 / 炸板 %d 家 / 最高连板 %d 板\n",
		intPtrVal(snap.LimitUpCount), intPtrVal(snap.LimitDownCount),
		intPtrVal(snap.BrokenLimitCount), intPtrVal(snap.MaxBoard)))
	if t := fPtrVal(snap.TurnoverAmt); t > 0 {
		b.WriteString(fmt.Sprintf("- 两市成交 %.0f 亿元\n", t))
	}
	var dist map[string]int
	_ = json.Unmarshal(snap.IndustryDist, &dist)
	if len(dist) > 0 {
		b.WriteString("\n## 涨停行业分布\n\n")
		names := make([]string, 0, len(dist))
		for n := range dist {
			names = append(names, n)
		}
		sort.Slice(names, func(i, j int) bool { return dist[names[i]] > dist[names[j]] })
		for i := 0; i < len(names) && i < 6; i++ {
			b.WriteString(fmt.Sprintf("- %s %d 家\n", names[i], dist[names[i]]))
		}
	}
	if len(evs) > 0 {
		b.WriteString("\n## 高置信事件\n\n")
		for i := 0; i < len(evs) && i < 3; i++ {
			b.WriteString(fmt.Sprintf("- %s（置信度 %.0f%%）\n", evs[i].Title, evs[i].Confidence*100))
		}
	}
	return b.String()
}

func toSnapHistory(snaps []model.MarketSnapshot) []apiSnapRow {
	out := make([]apiSnapRow, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, apiSnapRow{
			Date:         s.TradeDate.In(cst).Format("2006-01-02"),
			EmotionScore: fPtrVal(s.EmotionScore),
			EmotionState: orStr(s.EmotionState, ""),
			LimitUp:      intPtrVal(s.LimitUpCount),
			LimitDown:    intPtrVal(s.LimitDownCount),
			BrokenLimit:  intPtrVal(s.BrokenLimitCount),
			MaxBoard:     intPtrVal(s.MaxBoard),
		})
	}
	return out
}

func toTaskRun(r model.TaskRun) apiTaskRun {
	status := "running"
	switch r.Status {
	case "success", "ok", "done":
		status = "ok"
	case "failed", "error":
		status = "failed"
	}
	return apiTaskRun{
		Command: r.Command,
		Status:  status,
		Time:    r.StartedAt.In(cst).Format("15:04"),
		Note:    orStr(r.Error, ""),
	}
}

// GET /api/v1/recon —— 每日对账。
type apiReconRow struct {
	Date      string `json:"date"`
	Flashes   int    `json:"flashes"`
	Events    int    `json:"events"`
	Anomalies int    `json:"anomalies"`
	Status    string `json:"status"`
	Note      string `json:"note,omitempty"`
}

func (s *Server) handleAPIRecon(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListReconDaily(r.Context())
	if err != nil {
		s.apiErr(w, "recon", err)
		return
	}
	out := make([]apiReconRow, 0, len(rows))
	for _, row := range rows {
		status, note := "ok", ""
		if row.Anomalies > 0 {
			status = "warn"
			note = fmt.Sprintf("快讯抽取失败 %d 条待重试", row.Anomalies)
		}
		out = append(out, apiReconRow{
			Date:      row.Date.In(cst).Format("2006-01-02"),
			Flashes:   row.Flashes,
			Events:    row.Events,
			Anomalies: row.Anomalies,
			Status:    status,
			Note:      note,
		})
	}
	s.writeJSON(w, out)
}

// GET /api/v1/reviews —— 持仓 AI 诊断列表。
type apiReview struct {
	Date    string `json:"date"`
	Scope   string `json:"scope"`
	Summary string `json:"summary"`
	Refs    int    `json:"refs"`
	State   string `json:"state"`
}

type posReviewJSON struct {
	Review string `json:"review"`
	Risks  []struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	} `json:"risks"`
	Mistakes []struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	} `json:"mistakes"`
	Refs struct {
		Events   []store.ChatRef `json:"events"`
		Entities []store.ChatRef `json:"entities"`
		Notes    []store.ChatRef `json:"notes"`
	} `json:"refs"`
}

func (s *Server) handleAPIReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := s.store.ListPositionReviews(r.Context(), 20)
	if err != nil {
		s.apiErr(w, "reviews", err)
		return
	}
	out := make([]apiReview, 0, len(reviews))
	for _, p := range reviews {
		var rj posReviewJSON
		_ = json.Unmarshal(p.Review, &rj)
		nrisk := len(rj.Risks) + len(rj.Mistakes)
		state := "positive"
		switch {
		case nrisk == 1:
			state = "neutral"
		case nrisk >= 2:
			state = "negative"
		}
		out = append(out, apiReview{
			Date:    p.SnapshotDate.In(cst).Format("2006-01-02"),
			Scope:   "组合持仓诊断",
			Summary: rj.Review,
			Refs:    len(rj.Refs.Events) + len(rj.Refs.Entities) + len(rj.Refs.Notes),
			State:   state,
		})
	}
	s.writeJSON(w, out)
}

// GET /api/v1/trades —— 成交记录 + 持仓快照。
type apiTrade struct {
	Date   string  `json:"date"`
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Side   string  `json:"side"`
	Price  float64 `json:"price"`
	Qty    int     `json:"qty"`
	Amount float64 `json:"amount"`
	Source string  `json:"source"`
	Note   string  `json:"note,omitempty"`
}

type apiPosition struct {
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Qty    int     `json:"qty"`
	Cost   float64 `json:"cost"`
	Last   float64 `json:"last"`
	PnlPct float64 `json:"pnl_pct"`
}

type apiTrades struct {
	Trades    []apiTrade    `json:"trades"`
	Positions []apiPosition `json:"positions"`
}

func (s *Server) handleAPITrades(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ts, err := s.store.ListTrades(ctx, 0)
	if err != nil {
		s.apiErr(w, "trades", err)
		return
	}
	ps, err := s.store.LatestPositions(ctx)
	if err != nil {
		s.apiErr(w, "trades", err)
		return
	}
	out := apiTrades{Trades: []apiTrade{}, Positions: []apiPosition{}}
	for _, t := range ts {
		out.Trades = append(out.Trades, apiTrade{
			Date:   t.TradeDate.In(cst).Format("2006-01-02"),
			Code:   t.Code,
			Name:   t.Name,
			Side:   t.Side,
			Price:  t.Price,
			Qty:    t.Qty,
			Amount: t.Amount,
			Source: t.Source,
			Note:   orStr(t.Note, ""),
		})
	}
	for _, p := range ps {
		cost := fPtrVal(p.CostPrice)
		last := fPtrVal(p.Price)
		pnl := 0.0
		if cost > 0 { // PL 存的是绝对盈亏额,前端展示百分比 → 由成本价/现价推导
			pnl = (last - cost) / cost * 100
		}
		out.Positions = append(out.Positions, apiPosition{
			Code:   p.Code,
			Name:   p.Name,
			Qty:    p.Qty,
			Cost:   cost,
			Last:   last,
			PnlPct: pnl,
		})
	}
	s.writeJSON(w, out)
}

// GET /api/v1/chat —— 历史对话(最近会话)。
type apiChatMsg struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Time    string   `json:"time"`
	Refs    []string `json:"refs,omitempty"`
}

func (s *Server) handleAPIChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sid, err := s.store.LatestChatSessionID(ctx)
	if err != nil {
		s.apiErr(w, "chat", err)
		return
	}
	out := []apiChatMsg{}
	if sid == "" {
		s.writeJSON(w, out)
		return
	}
	msgs, err := s.store.ListChatMessages(ctx, sid)
	if err != nil {
		s.apiErr(w, "chat", err)
		return
	}
	for _, m := range msgs {
		var refs store.ChatRefs
		_ = json.Unmarshal(m.Refs, &refs)
		refStrs := []string{}
		for _, e := range refs.Events {
			refStrs = append(refStrs, "事件："+e.Title)
		}
		for _, e := range refs.Entities {
			refStrs = append(refStrs, "实体："+e.Title)
		}
		cm := apiChatMsg{
			Role:    m.Role,
			Content: m.Content,
			Time:    m.CreatedAt.In(cst).Format("15:04"),
		}
		if len(refStrs) > 0 {
			cm.Refs = refStrs
		}
		out = append(out, cm)
	}
	s.writeJSON(w, out)
}

// GET /api/v1/settings —— 大模型分层配置(密钥掩码)。
type apiSettingSection struct {
	Group string      `json:"group"`
	Rows  [][2]string `json:"rows"`
}

func (s *Server) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.ListAppConfig(r.Context())
	if err != nil {
		s.apiErr(w, "settings", err)
		return
	}
	base := m["ai_service_base_url"]
	sections := []apiSettingSection{}
	for _, g := range []struct{ label, modelKey string }{
		{"抽取模型(extract)", "ai_model_extract"},
		{"推理模型(reasoning)", "ai_model_reasoning"},
		{"视觉模型(vision)", "ai_model_vision"},
	} {
		if m[g.modelKey] != "" {
			sections = append(sections, apiSettingSection{
				Group: g.label,
				Rows:  [][2]string{{"服务地址", base}, {"模型", m[g.modelKey]}},
			})
		}
	}
	if k := m["ai_api_key"]; k != "" {
		sections = append(sections, apiSettingSection{
			Group: "访问凭证",
			Rows:  [][2]string{{"API Key", maskSecret(k)}},
		})
	}
	s.writeJSON(w, sections)
}

// GET /api/v1/weekly —— 周报列表(weekly_summaries → Doc 形状)。
func (s *Server) handleAPIWeekly(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListWeeklySummaries(r.Context())
	if err != nil {
		s.apiErr(w, "weekly", err)
		return
	}
	out := make([]apiDoc, 0, len(rows))
	for _, w := range rows {
		out = append(out, apiDoc{
			ID:        w.ID,
			Type:      "weekly",
			Slug:      w.Week,
			Title:     "周报 · " + w.Week,
			UpdatedAt: fmtRFC3339(w.UpdatedAt),
			Content:   w.Summary,
		})
	}
	s.writeJSON(w, out)
}
