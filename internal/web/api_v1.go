package web

// /api/v1 只读投影层 —— React 前端(frontend/)数据源。
//
// 只读、复用 store 查询、字段对齐 frontend/src/lib/types.ts;
// 未产出的字段如实给零值/省略(数据诚实),不做猜测映射。
// 列表端点返回全量,分页由前端客户端完成(page/size 只在 URL)。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"piks/internal/cluster"
	"piks/internal/model"
	"piks/internal/store"
)

// ---- 映射类型(对齐前端 types.ts) ----

// apiEventItem 事件流一条。
//
// ⚠️ 展示单元(issue #83 P-4 / P8):前端「事件详情」应展示的是**簇**(clusters),而非单个成员事件。
// 故本结构在簇存在时同时下发:
//   - `cluster_title`:簇标题(展示单元标题;无则前端退回 `title`);
//   - `cluster_sources`:来源列表,**取 raw 层全集**(某家报了但没抽成事件也在此列);
//   - `source_count`(机构数)/`independent_count`(独立来源数)仍是**客观计数**。
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
	// ClusterTitle 展示单元标题(issue #83 P-4 / P8):簇标题(event_clusters.title)。
	// 仅在事件属于某簇时下发;前端在簇视图里用它替代成员事件标题。空 = 无簇/簇无标题。
	ClusterTitle string `json:"cluster_title,omitempty"`
	// ClusterSources 簇内各源来源(issue #48 T2):同一真实事件被哪些机构报道过。
	// 仅当事件属于一个**跨源**簇(≥2 个不同机构)时才下发 ——单源簇/未聚类事件省略该字段,
	// 前端据此判断是否展示「N 源印证」;不给单源事件挂一个只有自己的「多源」假象。
	ClusterSources []apiClusterSource `json:"cluster_sources,omitempty"`
	// SourceCount 报道过该事件的**机构**数(issue #49 T3),去重后计数;未聚类事件 = 1。
	// 为何要单列:ClusterSources 是 omitempty,单源簇与未聚类**都不下发** —— 客户端
	// 无法据此区分「只有 1 家在报」和「还没聚类」。显式计数才能让「单一来源」可判定。
	// ⚠️ 这是**客观计数**,不是可信度判断:单源 ≠ 假消息(issue #49 红线)。
	SourceCount int `json:"source_count"`
	// EventConflicts 簇内跨源**数值冲突**(issue #49 T3):同一量被报成不同的数。
	// 仅在真检出冲突时下发;每条带**双方原文**,前端必须两条都显示(禁止静默择一)。
	EventConflicts []apiEventConflict `json:"event_conflicts,omitempty"`
	// IndependentCount **独立来源**数(issue #83 P-1):把簇内近逐字的「转载」并成一源后的计数。
	// 与 SourceCount(机构数)是两件事:同一篇通稿被 3 家原样转发 → SourceCount=3 而
	// IndependentCount=1。**印证度三级判定(单一来源/多家印证/广泛报道)以本字段为准** ——
	// 机构数会被转载刷高,只有独立来源数才是「几家在**各自**报」。未聚类事件 = 1。
	IndependentCount int `json:"independent_count"`
	// ClusterFacts 簇内**各成员事实句的并集**(issue #83 P-4 / P8):展示单元 = 簇,
	// 内容应是整个簇的成员事实,而非 canonical 单条。去重、保持成员到达顺序。无簇/无合并时省略。
	ClusterFacts []string `json:"cluster_facts,omitempty"`
	// ClusterAffected 簇内**各成员影响实体的并集**(同上):canonical 的 affected 只是子集。
	ClusterAffected []apiAffected `json:"cluster_affected,omitempty"`
}

// apiEventConflict 一条跨源数值冲突:什么量、两侧的值、双方原话。
//
// SentenceA/SentenceB 是**原句**,不是模型改写 —— 三层模型 Fact ≠ Inference:
// 机器只负责把分歧**指出来**,谁对由人判断。
type apiEventConflict struct {
	Unit      string    `json:"unit"`
	Values    []float64 `json:"values"`
	SentenceA string    `json:"sentence_a"`
	SentenceB string    `json:"sentence_b"`
}

// apiClusterSource 簇内一个来源:**机构名 + 该机构全部原文链接 + 上游一级源**(issue #83 P-4 / P8)。
//
// ⚠️ 与 P-4 之前的变化:
//   - 来源集**取 raw 层全集**(某家报了但没抽成事件也在列),不再只取事件层可见的机构;
//   - 同机构可能有多条链接,故新增 `urls` **数组**(全部如实列出、不合并)。
//
// ⚠️ `url` 保留 = `urls` 确定性首项(`urls[0]`,无则空串)。**非破坏性变更**:旧消费方读 `url`
// 语义不变(「该机构的一条原文链接」),新消费方读 `urls` 拿全集。二者同源,不会不一致。
type apiClusterSource struct {
	Source string `json:"source"`
	// URL 该机构的一条原文链接(= urls 首项;兼容旧契约,勿删)。空串 = 该源如实无外链。
	URL string `json:"url,omitempty"`
	// URLs 该机构**全部**原文链接;**可空数组** —— 空数组 = 该源如实无外链(绝不拼假链接)。
	URLs []string `json:"urls"`
	// Origin 上游自带的一级源名(金十/同花顺 extra.source,如「新华社」):「这条转述的是谁」,
	// 与「我们从哪个机构采到」(Source)是两件事。⚠️ 金十的 URL **就是**一级源的链接
	// (collector/jin10.go:金十无逐条原文 URL,不造假链接),故前端须把渠道名与一级源名**分区**展示,
	// 不得把一级源链接挂在渠道名下(issue #83 事实前提更正 ③)。
	Origin string `json:"origin,omitempty"`
	// Reprint 该来源是否为**转载**(issue #83 P-1):与同转载组另一机构近逐字(指纹 Jaccard ≥ 0.85)。
	// 仅作如实标注「(转载)」,**不隐藏也不合并显示**(红线「不静默」)。
	Reprint bool `json:"reprint,omitempty"`
	// Canonical 该来源所属转载组代表 id(issue #83 P-4):前端可据此把同组来源归并显示。
	// 与 reprint 配对 —— 同一代表组内,代表行 Canonical==自身、reprint=false,其余 reprint=true。
	Canonical string `json:"canonical,omitempty"`
}

type apiAffected struct {
	Word       string `json:"word"`
	EntityID   string `json:"entity_id,omitempty"`
	EntityName string `json:"entity_name,omitempty"`
	Code       string `json:"code,omitempty"` // 公司实体 6 位代码 → 前端跳个股中心
}

type apiEntity struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	UpdatedAt   string   `json:"updated_at"`
	// Code 6 位股票代码(仅 type=company 且有 detail.code 时非空):
	// 前端据此决定是否显示「深研」按钮,并作为 POST /research-runs 的 code。
	Code string `json:"code,omitempty"`
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
	// URL 原文出处(raw_documents.url);为空时前端退化为纯文本,不渲染死链。
	URL string `json:"url,omitempty"`
}

// apiAnnouncement 公告行(GET /api/v1/announcements,issue #50)。
// 原始事件源:官方披露,不进 LLM 抽取,故无 event_id/confidence。
// URL 指向原始 PDF(巨潮);为空时前端退化为纯文本,不渲染死链。
type apiAnnouncement struct {
	ID         string `json:"id"`
	Time       string `json:"time"`
	Title      string `json:"title"`
	Source     string `json:"source"`
	SecCode    string `json:"sec_code,omitempty"`
	SecName    string `json:"sec_name,omitempty"`
	PageColumn string `json:"page_column,omitempty"`
	// Grade 分级(issue #68 A 层):must/important/routine/noise;空=未分级
	// (本迁移前的历史行)。**机器判定(Inference)非事实**,前端须如实标注。
	Grade string `json:"grade,omitempty"`
	URL   string `json:"url,omitempty"`
}

type apiDoc struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
	Content   string `json:"content"`
}

// apiNoteDetail 笔记详情(GET /api/v1/notes/:id):含状态/置信度/关联,供编辑表单回填。
type apiNoteDetail struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Content     string   `json:"content"`
	UpdatedAt   string   `json:"updated_at"`
	SelEvents   []string `json:"sel_events"`
	SelEntities []string `json:"sel_entities"`
}

// ---- handlers ----

// GET /api/v1/events?type&status&q&sort —— 结构化事件流。
// sort=time(默认,发生时间倒序)/confidence(置信度倒序)。
func (s *Server) handleAPIEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sortBy := r.URL.Query().Get("sort")
	evs, err := s.store.ListEventsForAPI(ctx, sortBy)
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

	filtered := make([]store.EventForAPI, 0, len(evs))
	clusterIDs := make([]string, 0, len(evs))
	for _, ev := range evs {
		if typ != "" && ev.EventType != typ {
			continue
		}
		if !eventStatusOK(ev.Status, status) {
			continue
		}
		if q != "" && !strSub(q, ev.Title, orStr(ev.Summary, ""), affectedTerms(ev.Affected)) {
			continue
		}
		filtered = append(filtered, ev)
		if ev.ClusterID != nil {
			clusterIDs = append(clusterIDs, *ev.ClusterID)
		}
	}
	// 簇内各源来源(issue #48 T2):一次批量取,避免每事件一次往返。
	clusters, err := s.store.ListClusterSources(ctx, clusterIDs)
	if err != nil {
		s.apiErr(w, "cluster sources", err)
		return
	}
	// 簇内成员 facts(issue #49 T3):冲突检测要逐成员的事实句,同样批量取一次。
	members, err := s.store.ListClusterMembersWithFacts(ctx, clusterIDs)
	if err != nil {
		s.apiErr(w, "cluster members", err)
		return
	}
	// raw 层全集来源 + 簇标题(issue #83 P-4 / P8):展示单元取 raw 层,计数仍走事件层。
	rawSrcs, err := s.store.ListClusterRawSources(ctx, clusterIDs)
	if err != nil {
		s.apiErr(w, "cluster raw sources", err)
		return
	}
	titles, err := s.store.ListClusterTitles(ctx, clusterIDs)
	if err != nil {
		s.apiErr(w, "cluster titles", err)
		return
	}
	in := eventItemInput{idx: idx, clusters: clusters, members: members, raw: rawSrcs, titles: titles}

	out := make([]apiEventItem, 0, len(filtered))
	for _, ev := range filtered {
		out = append(out, toEventItem(ev, in))
	}
	s.writeJSON(w, out)
}

// GET /api/v1/entities?type&q —— 统一实体。
func (s *Server) handleAPIEntities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	typ := r.URL.Query().Get("type")
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	var ents []model.Entity
	var err error
	switch {
	case status != "":
		// 自选列表数据源(design frontend-ia §2.3):status=watch → 自选;archived → 已移出。
		ents, err = s.store.ListEntitiesByStatus(ctx, status)
	case typ != "":
		ents, err = s.store.ListEntitiesByType(ctx, typ)
	default:
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

// GET /api/v1/flashes?q&source&sort&hours —— 快讯流(raw_documents 投影)。
// sort=time(默认,时间倒序)/important(已被抽取成事件的优先)。
//
// hours=N(issue #83 分期 P-5 实时层):只取**原始到达时刻**近 N 小时的行 —— 实时档
// 「滚动近 3h」(前端传 hours=3)。缺省/非法/≤0 = 不限(全量,与旧行为逐字一致,**ADDITIVE**).
// ⚠️ 实时是**raw 层滚动读**,不新建采集进程(盘中采集由常驻 collector 承担)、不合并、不排名。
func (s *Server) handleAPIFlashes(w http.ResponseWriter, r *http.Request) {
	var since time.Time
	if h := r.URL.Query().Get("hours"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 {
			since = time.Now().Add(-time.Duration(n) * time.Hour)
		}
	}
	flashes, err := s.store.ListRawDocumentsWithSource(r.Context(), r.URL.Query().Get("sort"), since)
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
		// Title 即查询里 `COALESCE(rd.title, rd.content)`,也正是前端渲染的那段正文
		// (apiFlash.Content),故按正文搜索与用户所见一致(issue #80:此前占位符承诺搜正文却只搜标题)。
		if q != "" && !strSub(q, f.Title) {
			continue
		}
		out = append(out, toFlash(f))
	}
	s.writeJSON(w, out)
}

// GET /api/v1/event-types —— 事件类型枚举(issue #61)。
//
// 真源 = `model.EventTypes`(单一切片)。前端**不存本地类型表**,下拉/标签/配色一律由此
// 驱动 —— 后端加类型时前端零改动即跟上,这是修掉「前后端枚举漂移」的根治手段。
//
// ⚠️ 不要改回「只下发 key、前端自带 label」:那样后端加类型前端照样回落英文,漂移只修一半。
func (s *Server) handleAPIEventTypes(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, model.EventTypes)
}

// GET /api/v1/announcements?q&source —— 公告流(原始事件源,issue #50)。
// 只读投影:raw_documents 中 source_type='announcement' 的行,按时间倒序。
// 与「快讯」是**两条独立投影**(快讯查询已显式排除 announcement,互不混入);
// 公告官方披露、不进 LLM,故无 event_id/置信度字段。
// 分页沿用本项目既有约定(前端 usePagedQuery 客户端切片),故此处全量下发。
func (s *Server) handleAPIAnnouncements(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListAnnouncementsWithSource(r.Context())
	if err != nil {
		s.apiErr(w, "announcements", err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	src := r.URL.Query().Get("source")
	// grade 过滤(issue #68 A 层):按级别折叠时前端传 grade=must|important|routine|noise。
	// 值为 "all"/空 时不过滤。**未分级的行(grade=NULL)不匹配任何具体级别** ——
	// 前端把 NULL 归入「常规」展示,故后端按 grade=routine 查询时**同时**放行 NULL
	// (历史行不能因为没分级就从「常规」视图里消失;宁可多显示,不可误隐藏)。
	grd := r.URL.Query().Get("grade")

	out := make([]apiAnnouncement, 0, len(rows))
	for _, a := range rows {
		if src != "" && a.Source != src {
			continue
		}
		if q != "" && !strSub(q, a.Title, orStr(a.SecCode, ""), orStr(a.SecName, "")) {
			continue
		}
		ag := orStr(a.Grade, "")
		if grd != "" && grd != "all" && !gradeMatch(ag, grd) {
			continue
		}
		out = append(out, apiAnnouncement{
			ID:         a.ID,
			Time:       a.AnnouncedAt.In(cst).Format("2006-01-02 15:04:05"),
			Title:      a.Title,
			Source:     a.Source,
			SecCode:    orStr(a.SecCode, ""),
			SecName:    orStr(a.SecName, ""),
			PageColumn: orStr(a.PageColumn, ""),
			Grade:      ag,
			URL:        orStr(a.URL, ""),
		})
	}
	s.writeJSON(w, out)
}

// gradeMatch 级别过滤匹配。空 grade(未分级的历史行)按「常规」对待 ——
// 与前端把 NULL 显示为常规一致,避免历史行在任何级别视图里都查不到。
func gradeMatch(rowGrade, filter string) bool {
	if rowGrade == "" {
		rowGrade = "routine"
	}
	return rowGrade == filter
}

// GET /api/v1/notes?type —— 笔记列表(personal_notes 投影)。
// 类型过滤按后端实际 type 值(note/belief/case/mistake);daily-review/weekly 无对应存储,如实为空。
// GET/POST /api/v1/notes —— 列表 / 新建。
func (s *Server) handleAPINotes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createNoteAPI(w, r)
		return
	case http.MethodGet:
	default:
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET/POST")
		return
	}
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

// GET/PUT/DELETE /api/v1/notes/:id —— 单篇笔记(读/改/归档)。
func (s *Server) handleAPINote(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/notes/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.updateNoteAPI(w, r, id)
		return
	case http.MethodDelete:
		s.archiveNoteAPI(w, r, id)
		return
	case http.MethodGet:
	default:
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET/PUT/DELETE")
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
	// 笔记详情含状态/置信度/关联(编辑表单回填);周报回退保持 apiDoc 形状。
	refs, rerr := s.store.ListNoteRefs(r.Context(), id)
	if rerr != nil {
		s.apiErr(w, "note", rerr)
		return
	}
	out := apiNoteDetail{
		ID: n.ID, Type: n.Type, Slug: n.Slug,
		Title: orStr(n.Title, ""), Status: n.Status,
		Content: orStr(n.Content, ""), UpdatedAt: fmtRFC3339(n.UpdatedAt),
		SelEvents: []string{}, SelEntities: []string{},
	}
	if n.Confidence != nil {
		out.Confidence = n.Confidence
	}
	for _, ref := range refs {
		switch ref.ToType {
		case "event":
			out.SelEvents = append(out.SelEvents, ref.ToID)
		case "entity":
			out.SelEntities = append(out.SelEntities, ref.ToID)
		}
	}
	s.writeJSON(w, out)
}

// ---- 类型映射 ----

// buildNameIndex 实体名/别名 → 实体索引(事件 affected 词 → 实体链接)。
// code = 公司实体的 6 位代码(前端 affected 词可直接跳个股中心);非公司为空。
type nameRef struct{ id, name, code string }

func buildNameIndex(ents []model.Entity) map[string]nameRef {
	idx := make(map[string]nameRef, len(ents)*2)
	for _, e := range ents {
		ref := nameRef{e.ID, e.Name, entityCode(e)}
		add := func(k string) {
			if k == "" {
				return
			}
			if _, ok := idx[k]; !ok {
				idx[k] = ref
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

// eventItemInput toEventItem 的批量取数上下文(一次查询、多事件复用,避免每事件一次往返)。
// 五个 map 都以 cluster_id 为键;`raw` 是 P-4 新增的 raw 层来源映射(见 ListClusterRawSources)。
type eventItemInput struct {
	idx      map[string]nameRef
	clusters map[string][]store.ClusterSource    // 事件层来源(计数与转载判定的真源)
	members  map[string][]store.ClusterMember    // 事件层成员事实(冲突检测 + 并集)
	raw      map[string][]store.ClusterRawSource // raw 层全集来源(展示用)
	titles   map[string]string                   // 簇标题(展示单元标题)
}

func toEventItem(ev store.EventForAPI, in eventItemInput) apiEventItem {
	at := ev.CreatedAt
	if ev.OccurredAt != nil {
		at = *ev.OccurredAt
	}
	var facts []string
	_ = json.Unmarshal(ev.Facts, &facts)
	if facts == nil {
		// 空 facts 必须编码成 `[]` 而非 `null`(issue #59 同类):前端 EventDetail
		// 直接 `event.facts.map()`。unmarshal `null`/缺字段都会留 nil,
		// 而 `[]` 反序列化出来本就是空 slice —— 故按 nil 判定即可。
		facts = []string{}
	}
	var words []string
	_ = json.Unmarshal(ev.Affected, &words)
	affected := make([]apiAffected, 0, len(words))
	for _, w := range words {
		af := apiAffected{Word: w}
		if ref, ok := in.idx[w]; ok {
			af.EntityID = ref.id
			af.EntityName = ref.name
			af.Code = ref.code
		}
		affected = append(affected, af)
	}
	out := apiEventItem{
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
		// 未聚类事件确实只有自己那一家 → 1(issue #49)。
		SourceCount:      1,
		IndependentCount: 1,
	}
	if ev.ClusterID == nil {
		return out
	}
	cid := *ev.ClusterID
	out.ClusterTitle = in.titles[cid]
	// 展示单元 = 簇(P8):canonical 单条的 facts/affected 只是子集,补上**成员并集**。
	// 仅在真有合并(len(members)>1)时才下发,单成员簇的并集恒等于它自己、徒增载荷。
	if mem := in.members[cid]; len(mem) > 1 {
		out.ClusterFacts, out.ClusterAffected = clusterContentUnion(mem, in.idx)
	}
	srcs := in.clusters[cid]
	// 机构数是**客观计数**:簇内去重后的机构个数(含 merged 成员贡献的来源)。
	if len(srcs) > 0 {
		out.SourceCount = len(srcs)
	}
	// 剥转载(issue #83 P-1):对簇内各机构**代表正文**跑分组,近逐字者并为**一个独立来源**。
	// 独立来源数 = 组数;逐来源标「(转载)」不隐藏、不合并显示(红线「不静默」)。
	// 原发锚点 = 本事件(它在簇内即 canonical,见 `ListEventsForAPI` 只发非 merged),
	// 保证「N 家为转载」与该标记数**恰好对得上**(原发不误标)。
	// SourceCount 保持机构数不变 —— 它没有错,错的是拿它当印证度用。
	contents := make([]string, len(srcs))
	canonicalIdx := -1
	for i, cs := range srcs {
		contents[i] = orStr(cs.Content, "")
		if cs.EventID == ev.ID {
			canonicalIdx = i
		}
	}
	flags := cluster.ReprintFlags(contents, canonicalIdx)
	out.IndependentCount = cluster.IndependentCount(contents)

	// 来源列表(P8):**取 raw 层全集** —— 含「报了这篇稿但没被抽成事件」的机构。
	// 计数与转载标记仍走事件层(上面的 srcs/flags,与 P-1/P-3 口径一致);
	// raw 层只负责把「哪些渠道报了 + 各自链接」**变全**。无 raw 数据时退回事件层(合法退化)。
	//
	// 是否下发:事件层 ≥2 机构(维持 issue #48「单源簇不挂 cluster_sources」) **或** raw 层
	// 出现了事件层看不见的机构(那正是 P8 要修的漏源)。
	if len(srcs) >= 2 || len(in.raw[cid]) > 0 {
		out.ClusterSources = clusterSourceItems(srcs, flags, in.raw[cid])
	}
	// 跨源数值冲突(issue #49 T3):簇内成员两两比对。仅有 ≥2 家机构时才有意义
	// (同机构多次报道不算跨源印证,比对它只会制造噪音)。
	if out.SourceCount >= 2 {
		out.EventConflicts = conflictsOf(in.members[cid])
	}
	return out
}

// clusterSourceItems 组装 P8 来源列表:机构名 → 该机构**全部** raw 层链接。
//
// 合并规则:
//   - 机构集 = 事件层机构 ∪ raw 层机构(并集 —— raw 层是全集,事件层是子集);
//   - 每机构的 URL 全集来自 raw 层;raw 层没有的机构(理论上不该有)退回事件层那条 `url`;
//   - `reprint` / `canonical` 取自**事件层**的 P-1 判定(计数真源),raw 层机构沿用同机构的事件层标记;
//   - `origin`(一级源名)按机构取(raw 层优先,回退事件层)—— 前端据此分区展示「渠道 / 一级源」。
//
// ⚠️ 同机构可能对应多个代表组(该机构对同一事件发过两条不同稿):此时 reprint 取「任一为转载即转载」,
// 宁可多标不漏标(如实,不隐藏)。
func clusterSourceItems(srcs []store.ClusterSource, flags []bool, raw []store.ClusterRawSource) []apiClusterSource {
	// 事件层:机构 → 标记 + origin + 回退 url。
	reprintOf := map[string]bool{}
	originOf := map[string]string{}
	fallbackURL := map[string]string{}
	for i, cs := range srcs {
		if flags[i] {
			reprintOf[cs.Source] = true
		}
		if o := orStr(cs.Origin, ""); o != "" {
			originOf[cs.Source] = o
		}
		if u := orStr(cs.URL, ""); u != "" && fallbackURL[cs.Source] == "" {
			fallbackURL[cs.Source] = u
		}
	}
	// raw 层:机构 → 全部 URL(保持库内稳定序)+ origin 回退。
	urlsOf := map[string][]string{}
	seen := map[string]map[string]bool{}
	order := make([]string, 0, len(srcs))
	addOrg := func(org string) {
		if _, ok := urlsOf[org]; !ok {
			urlsOf[org] = []string{}
			order = append(order, org)
		}
	}
	for _, sr := range srcs {
		addOrg(sr.Source)
	}
	for _, rs := range raw {
		addOrg(rs.Source)
		if u := orStr(rs.URL, ""); u != "" {
			if seen[rs.Source] == nil {
				seen[rs.Source] = map[string]bool{}
			}
			if !seen[rs.Source][u] {
				seen[rs.Source][u] = true
				urlsOf[rs.Source] = append(urlsOf[rs.Source], u)
			}
		}
		if o := orStr(rs.Origin, ""); o != "" {
			if originOf[rs.Source] == "" {
				originOf[rs.Source] = o
			}
		}
	}
	sort.Strings(order)
	out := make([]apiClusterSource, 0, len(order))
	for _, org := range order {
		urls := urlsOf[org]
		first := ""
		if len(urls) > 0 {
			first = urls[0]
		} else {
			first = fallbackURL[org] // raw 层该机构无行 → 退回事件层那条(不丢链接)
		}
		out = append(out, apiClusterSource{
			Source: org, URL: first, URLs: urls,
			Origin: originOf[org], Reprint: reprintOf[org],
		})
	}
	return out
}

// clusterContentUnion 汇总簇内**各成员**的 facts 与 affected 实体(并集,去重,保持到达顺序)。
//
// 为什么:展示单元 = 簇(issue #83 P8),但 `apiEventItem.Facts/Affected` 来自事件的
// **单条** raw(canonical)。合并后 canonical 只是簇内一员,其余成员的事实句/影响实体
// 若不下发就会「数据层合了多家、展示层回退到一家」—— 正是 P8 要修的问题。
//
// ⚠️ 事实句按**字面**去重(不模糊匹配):模糊归并可能把「净利润 3 亿」与「净利润 3.2 亿」
// 合成一条而**丢掉分歧**,那是 conflictsOf 该暴露的东西,不在这里静默合并。
func clusterContentUnion(mem []store.ClusterMember, idx map[string]nameRef) ([]string, []apiAffected) {
	facts := []string{}
	seenF := map[string]bool{}
	words := []string{}
	seenW := map[string]bool{}
	for _, m := range mem {
		var fs []string
		_ = json.Unmarshal(m.Facts, &fs)
		for _, f := range fs {
			if f == "" || seenF[f] {
				continue
			}
			seenF[f] = true
			facts = append(facts, f)
		}
		var ws []string
		_ = json.Unmarshal(m.Affected, &ws)
		for _, w := range ws {
			if w == "" || seenW[w] {
				continue
			}
			seenW[w] = true
			words = append(words, w)
		}
	}
	affected := make([]apiAffected, 0, len(words))
	for _, w := range words {
		af := apiAffected{Word: w}
		if ref, ok := idx[w]; ok {
			af.EntityID = ref.id
			af.EntityName = ref.name
			af.Code = ref.code
		}
		affected = append(affected, af)
	}
	return facts, affected
}

// conflictsOf 对簇内成员两两跑数值冲突检测,汇总去重。
//
// 用**两两**而非只跟 canonical 比:分歧可能出现在任意两家之间,只比 canonical 会漏掉
// 「A 与 B 不一致、而 canonical 恰好与 A 相同」的情形 —— 那正是最该暴露的静默择一。
// 每句对内部已产出「双方原文」,故冲突天然可回溯到具体两家。
func conflictsOf(mem []store.ClusterMember) []apiEventConflict {
	if len(mem) < 2 {
		return nil
	}
	factsOf := func(m store.ClusterMember) []string {
		var fs []string
		_ = json.Unmarshal(m.Facts, &fs)
		return fs
	}
	seen := make(map[string]bool)
	var out []apiEventConflict
	for i := 0; i < len(mem); i++ {
		for j := i + 1; j < len(mem); j++ {
			for _, c := range cluster.DetectFactConflicts(factsOf(mem[i]), factsOf(mem[j])) {
				k := c.SentenceA + "\x00" + c.SentenceB + "\x00" + c.Unit
				if seen[k] {
					continue
				}
				seen[k] = true
				out = append(out, apiEventConflict{
					Unit: c.Unit, Values: c.Values,
					SentenceA: c.SentenceA, SentenceB: c.SentenceB,
				})
			}
		}
	}
	return out
}

// eventStatusFront 后端 status → 前端筛选/展示口径。
//
// ⚠️ 只有两个**在产**知识态(issue #80 实测):`extracted`(抽取成功,唯一由
// `internal/extract` 写入)与 `merged`(聚类把重复报道并入代表后置的合并态)。
// 历史上另有 `verified`/`published`,但其**唯一写入方是已随 P6 下线的 vault 发布器**
// —— 发布生命周期改由 `published_at` 承载(`iter1-reliability.md` §3.4),生产实测
// 无任何 verified/published 行。故旧映射 `verified|published → confirmed` 是**结构上
// 恒空**的死选项,已移除;宁可如实标「已抽取」,不给一个永远点不出东西的「已确认」。
func eventStatusFront(backend string) string {
	switch backend {
	case "merged":
		return "merged"
	case "extracted", "verified", "published":
		// verified/published 为历史遗留行,知识态上仍是「已抽取未合并」。
		return "extracted"
	default:
		return backend
	}
}

// eventStatusOK 前端筛选状态匹配。空筛选 = 全放行;其余按**同一口径**精确匹配
// (与 eventStatusFront 的取值一致:extracted / merged)。
func eventStatusOK(backend, filter string) bool {
	if filter == "" {
		return true
	}
	return eventStatusFront(backend) == filter
}

func toEntity(e model.Entity) apiEntity {
	var aliases []string
	_ = json.Unmarshal(e.Aliases, &aliases)
	if aliases == nil {
		// 同 issue #59:空别名须为 `[]`。前端多处直接用(`CommandPalette` 的
		// `e.aliases.some(...)`、`entities.tsx` 的 `e.aliases.join(...)`)。
		aliases = []string{}
	}
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
		Code:        entityCode(e),
	}
}

// entityCode 从 entities.detail 取股票代码并归一为 6 位(与 research_runs.code 同口径)。
// detail 里存的是 {code:"600519",…}(见 EnsureCompanyEntity);非公司实体或无限 code → 空串,
// 前端据此隐藏「深研」按钮(不该给行业/概念挂个股深研入口)。
func entityCode(e model.Entity) string {
	if e.Type != "company" || len(e.Detail) == 0 {
		return ""
	}
	var d struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(e.Detail, &d); err != nil {
		return ""
	}
	return store.NormalizeCode(d.Code)
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

// emptyMarketSnapshot 无市场快照时的空态市场块(issue #59)。
//
// 存在的唯一理由:apiMarketSnapshot 的 slice 字段留零值会被序列化成 `null`,
// 而前端按「非可选数组」声明并直接 `.map()`(首页 `/`、`/market`、`/ladder` 均如此)
// → 空库整页白屏。此函数保证字段齐全且数组为 `[]`。
//
// 与 toSnapshot 的空态保持同一形状;标量零值(`""`/`0`)即正确空态,不另外造词。
func emptyMarketSnapshot() apiMarketSnapshot {
	return apiMarketSnapshot{
		Indices:      []apiIndex{},
		Ladder:       []apiLimitUpStock{},
		IndustryDist: []apiIndustryDist{},
	}
}

func toFlash(f store.RawDocWithSource) apiFlash {
	return apiFlash{
		ID:      f.ID,
		Time:    f.FlashAt.In(cst).Format("2006-01-02 15:04"),
		Content: f.Title,
		Source:  f.Source,
		// 重要度优先取**源自带标记**(important/confirmed,issue #43);
		// 无标记的源(东财/新浪/同花顺/富途)退化为「已被抽取成事件」这一既有近似。
		Important: f.Important || f.EventID != nil,
		EventID:   orStr(f.EventID, ""),
		URL:       orStr(f.URL, ""),
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

// affectedTerms 把事件的 `affected` JSON 词表摊平成搜索字段。
// 事件搜索框承诺搜「影响实体」(events.tsx 占位符),而实体名此前未参与 strSub,
// 故按公司名搜不到(issue #80)。解析失败按无词处理(与 toEventItem 同口径)。
func affectedTerms(raw json.RawMessage) string {
	var words []string
	_ = json.Unmarshal(raw, &words)
	return strings.Join(words, " ")
}

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
	// Failed = meta.failed(本次未入库条数)。issue #64:此前只上屏 status,
	// 后端把失败记进 meta 却无读取方 ⇒ 用户看不到「N 条未入库」。
	Failed int `json:"failed"`
}

type apiDashboard struct {
	Stats       []apiStatCard     `json:"stats"`
	Market      apiMarketSnapshot `json:"market"`
	SnapHistory []apiSnapRow      `json:"snap_history"`
	Review      string            `json:"review"`
	TopEvents   []apiTopEvent     `json:"top_events"`
	TaskRuns    []apiTaskRun      `json:"task_runs"`
}

// GET /api/v1/dashboard —— 看板(我的概况 + 最新快照 + 历史情绪 + 每日复盘 + 数据更新)。
func (s *Server) handleAPIDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
	// 事件**候选**:此处刻意用排除 merged 的查询(issue #80)。本 evs 供 reviewMarkdown
	// 的「高置信事件」取前 3 与 TopEvents 取前 6 两处,**两处都按置信度取**,且**都没有**
	// 知识态筛选 —— 若用含 merged 的 `ListEventsForAPI`(那是前端事件列表专用),
	// 「已被合并」的重复报道会作为独立条目混进 Top N。
	// (旧实现依赖该查询的升序默认值,「高置信事件」实际取到的是最老的 3 条 —— 现按置信度。)
	evs, err := s.store.ListTopEventsForDashboard(ctx)
	if err != nil {
		s.apiErr(w, "dashboard", err)
		return
	}
	runs, err := s.store.ListTaskRuns(ctx, 6)
	if err != nil {
		s.apiErr(w, "dashboard", err)
		return
	}

	// KPI 归「我」而非管线:P6-2 起看板回答「我的资产有什么」,不再报知识库规模。
	// 自选数/持仓数复用既有查询(零新 store 方法)。
	watchN := 0
	if watchEnts, err := s.store.ListEntitiesByStatus(ctx, "watch"); err == nil {
		watchN = len(watchEnts)
	}
	heldN := 0
	if ps, err := s.store.LatestPositions(ctx); err == nil {
		heldN = len(ps)
	}

	// ⚠️ 所有数组字段**必须初始化为空 slice,不能留零值 nil**(issue #59):
	// 零值 slice 经 encoding/json 会序列化成 `null`,而前端类型声明是非可选数组
	// (`types.ts` 的 `indices: {...}[]`) 并直接 `.map()` → 空库时整页白屏。
	// 空态必须编码成 `[]` —— 与 CLAUDE.md 强制规则 9(loading/error/**empty** 三态)同源。
	out := apiDashboard{
		Stats: []apiStatCard{
			{Label: "我的自选", Value: watchN},
			{Label: "持仓股票", Value: heldN},
			{Label: "我的笔记", Value: notes},
			{Label: "交易记录", Value: trades},
		},
		// 无快照日时 Market 字段仍须齐全:前端无条件读 market.trade_date / emotion_score
		// 等标量,零值结构体正好给出""与 0;数组则由 toSnapshot 显式给空 slice。
		Market:      emptyMarketSnapshot(),
		SnapHistory: []apiSnapRow{},
		TopEvents:   []apiTopEvent{},
		TaskRuns:    []apiTaskRun{},
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

// toTaskRun 把 task_runs 行映射为前端看板的条目。
//
// ⚠️ switch **必须显式列出每个已知状态**:未匹配者一律落 'running'(「进行中」),
// 是另一种误导(issue #64)。新增状态(如 'partial'/'skipped')时**同步改这里**。
func toTaskRun(r model.TaskRun) apiTaskRun {
	status := "running"
	switch r.Status {
	case "success", "ok", "done":
		status = "ok"
	case "partial":
		status = "partial" // 部分失败:入库了但有条目没落,需如实可见(#64)
	case "failed", "error":
		status = "failed"
	case "skipped":
		status = "skipped"
	}
	return apiTaskRun{
		Command: r.Command,
		Status:  status,
		Time:    r.StartedAt.In(cst).Format("15:04"),
		Note:    orStr(r.Error, ""),
		Failed:  metaFailed(r.Meta),
	}
}

// metaFailed 读 task_runs.meta.failed(未入库条数)。缺失/非数字 ⇒ 0(如非采集类任务)。
func metaFailed(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var m struct {
		Failed int `json:"failed"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0
	}
	return m.Failed
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
	Date     string           `json:"date"`
	Scope    string           `json:"scope"`
	Summary  string           `json:"summary"`
	Refs     int              `json:"refs"`
	State    string           `json:"state"`
	Risks    []apiReviewPoint `json:"risks,omitempty"`
	Mistakes []apiReviewPoint `json:"mistakes,omitempty"`
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

// toAPIReview 把一条持仓诊断投影为前端 ReviewRow。
// 纯函数(无 DB),便于对「risks + mistakes 都出、state 由二者合计决定」做回归。
func toAPIReview(p store.PositionReview) apiReview {
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
	risks := make([]apiReviewPoint, 0, len(rj.Risks))
	for _, r := range rj.Risks {
		risks = append(risks, apiReviewPoint{Title: r.Title, Content: r.Content})
	}
	mistakes := make([]apiReviewPoint, 0, len(rj.Mistakes))
	for _, m := range rj.Mistakes {
		mistakes = append(mistakes, apiReviewPoint{Title: m.Title, Content: m.Content})
	}
	return apiReview{
		Date:     p.SnapshotDate.In(cst).Format("2006-01-02"),
		Scope:    "组合持仓诊断",
		Summary:  rj.Review,
		Refs:     len(rj.Refs.Events) + len(rj.Refs.Entities) + len(rj.Refs.Notes),
		State:    state,
		Risks:    risks,
		Mistakes: mistakes,
	}
}

func (s *Server) handleAPIReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := s.store.ListPositionReviews(r.Context(), 20)
	if err != nil {
		s.apiErr(w, "reviews", err)
		return
	}
	out := make([]apiReview, 0, len(reviews))
	for _, p := range reviews {
		out = append(out, toAPIReview(p))
	}
	s.writeJSON(w, out)
}

// GET /api/v1/account —— 最近账户资金汇总(issue #19)。无快照 → {"account":null}。
// 独立端点(而非塞进 /reviews 数组)避免破坏既有响应形状;/trades 与 /reviews 共用。
func (s *Server) handleAPIAccount(w http.ResponseWriter, r *http.Request) {
	acc, err := s.store.LatestAccountSnapshot(r.Context())
	if err != nil {
		s.apiErr(w, "account", err)
		return
	}
	s.writeJSON(w, map[string]any{"account": toAPIAccount(acc)})
}

// GET /api/v1/trades —— 成交记录 + 持仓快照。
type apiReviewPoint struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// apiDecisionRef 决策关联目标(买入时在看什么,P6-4):研报 / 事件 / 笔记。
type apiDecisionRef struct {
	Kind  string `json:"kind"` // research | event | note
	ID    string `json:"id"`
	Title string `json:"title"`
	Date  string `json:"date,omitempty"` // 研报 as_of / 事件发生日;笔记无
	URL   string `json:"url,omitempty"`  // 深链
}

type apiTrade struct {
	ID       string           `json:"id"`
	Date     string           `json:"date"`
	Code     string           `json:"code"`
	Name     string           `json:"name"`
	Side     string           `json:"side"`
	Price    float64          `json:"price"`
	Qty      int              `json:"qty"`
	Amount   float64          `json:"amount"`
	Source   string           `json:"source"`
	Note     string           `json:"note,omitempty"`
	Review   string           `json:"review,omitempty"`
	Mistakes []apiReviewPoint `json:"mistakes,omitempty"`
	BasedOn  []apiDecisionRef `json:"based_on,omitempty"`
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
	Account   *apiAccount   `json:"account"` // 账户汇总(issue #19);无快照 → null
}

// apiAccount 账户级汇总 DTO(issue #19)。字段用 *float64:**null = 截图没这个数**,
// 前端据此不显示该项(区别于 0「确实为零」)。绝不在这里把 null 折成 0。
type apiAccount struct {
	Date       string   `json:"date"`
	TotalAsset *float64 `json:"total_asset"`
	TotalMV    *float64 `json:"total_mv"`
	FloatPL    *float64 `json:"float_pl"`
	DailyPL    *float64 `json:"daily_pl"`
}

func toAPIAccount(a *model.AccountSnapshot) *apiAccount {
	if a == nil {
		return nil
	}
	return &apiAccount{
		Date:       a.SnapshotDate.In(cst).Format("2006-01-02"),
		TotalAsset: a.TotalAsset, TotalMV: a.TotalMV,
		FloatPL: a.FloatPL, DailyPL: a.DailyPL,
	}
}

// toAPITrade 单条成交 → 前端 DTO(含 AI 复盘/复盘点解析)。交易页与个股中心共用。
func toAPITrade(t model.Trade) apiTrade {
	tr := apiTrade{
		ID:     t.ID,
		Date:   t.TradeDate.In(cst).Format("2006-01-02"),
		Code:   t.Code,
		Name:   t.Name,
		Side:   t.Side,
		Price:  t.Price,
		Qty:    t.Qty,
		Amount: t.Amount,
		Source: t.Source,
		Note:   orStr(t.Note, ""),
	}
	if rv := parseTradeReview(t.Review); rv != nil {
		tr.Review = rv.Review
		tr.Mistakes = make([]apiReviewPoint, 0, len(rv.Mistakes))
		for _, m := range rv.Mistakes {
			tr.Mistakes = append(tr.Mistakes, apiReviewPoint{Title: m.Title, Content: m.Content})
		}
	}
	return tr
}

// decisionRefsForTrades 批量取交易决策边并解析为可渲染引用(P6-4 一次查询,免 N 次往返)。
// 悬空边(目标已删/状态不符)诚实跳过,不渲染无名空节点。
func (s *Server) decisionRefsForTrades(ctx context.Context, tradeIDs []string) (map[string][]apiDecisionRef, error) {
	out := map[string][]apiDecisionRef{}
	if len(tradeIDs) == 0 {
		return out, nil
	}
	rels, err := s.store.ListRelationshipsFromIDs(ctx, "trade", tradeIDs, "based_on")
	if err != nil {
		return nil, err
	}
	// 收集三类目标 id,各自批量取标题。
	var runIDs, eventIDs, noteIDs []string
	for _, r := range rels {
		switch r.ToType {
		case "research_run":
			runIDs = append(runIDs, r.ToID)
		case "event":
			eventIDs = append(eventIDs, r.ToID)
		case "personal_note":
			noteIDs = append(noteIDs, r.ToID)
		}
	}
	runMeta := map[string]struct {
		Title, Date, RunID string
	}{}
	runs, err := s.store.ListResearchRunsByIDs(ctx, runIDs)
	if err != nil {
		return nil, err
	}
	names := s.researchNames(ctx, runs)
	for _, r := range runs {
		runMeta[r.ID] = struct {
			Title, Date, RunID string
		}{researchRunTitle(r.Code, r.Symbol, names[r.Code], r.Profile), fmtDate(r.AsOf.In(cst)), r.RunID}
	}
	evTitle := map[string]string{}
	evDate := map[string]string{}
	evs, err := s.store.ListEventsByIDs(ctx, eventIDs)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		evTitle[e.ID] = e.Title
		evDate[e.ID] = eventTime(e).In(cst).Format("2006-01-02")
	}
	noteTitle := map[string]string{}
	ns, err := s.store.ListPersonalNotesByIDs(ctx, noteIDs)
	if err != nil {
		return nil, err
	}
	for _, n := range ns {
		noteTitle[n.ID] = orStr(n.Title, "")
	}
	for _, r := range rels {
		var ref apiDecisionRef
		switch r.ToType {
		case "research_run":
			m, ok := runMeta[r.ToID]
			if !ok {
				continue // 报告非 done 或已删 → 不渲染
			}
			ref = apiDecisionRef{Kind: "research", ID: r.ToID, Title: m.Title, Date: m.Date, URL: "/research/" + m.RunID}
		case "event":
			t, ok := evTitle[r.ToID]
			if !ok {
				continue
			}
			ref = apiDecisionRef{Kind: "event", ID: r.ToID, Title: t, Date: evDate[r.ToID], URL: "/events/" + r.ToID}
		case "personal_note":
			t, ok := noteTitle[r.ToID]
			if !ok {
				continue
			}
			ref = apiDecisionRef{Kind: "note", ID: r.ToID, Title: t, URL: "/notes/" + r.ToID}
		default:
			continue
		}
		out[r.FromID] = append(out[r.FromID], ref)
	}
	return out, nil
}

// toAPIPosition 单条持仓 → 前端 DTO(盈亏由成本/现价推导百分比)。交易页与个股中心共用。
func toAPIPosition(p model.Position) apiPosition {
	cost := fPtrVal(p.CostPrice)
	last := fPtrVal(p.Price)
	pnl := 0.0
	if cost > 0 { // PL 存的是绝对盈亏额,前端展示百分比 → 由成本价/现价推导
		pnl = (last - cost) / cost * 100
	}
	return apiPosition{
		Code:   p.Code,
		Name:   p.Name,
		Qty:    p.Qty,
		Cost:   cost,
		Last:   last,
		PnlPct: pnl,
	}
}

// GET/POST /api/v1/trades —— 成交+持仓 / 手动录入。
func (s *Server) handleAPITrades(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.tradeAddAPI(w, r)
		return
	case http.MethodGet:
	default:
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET/POST")
		return
	}
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
	acc, err := s.store.LatestAccountSnapshot(ctx)
	if err != nil {
		s.apiErr(w, "trades", err)
		return
	}
	out := apiTrades{Trades: []apiTrade{}, Positions: []apiPosition{}, Account: toAPIAccount(acc)}
	ids := make([]string, 0, len(ts))
	for _, t := range ts {
		ids = append(ids, t.ID)
	}
	refs, err := s.decisionRefsForTrades(ctx, ids)
	if err != nil {
		s.apiErr(w, "trades", err)
		return
	}
	for _, t := range ts {
		tr := toAPITrade(t)
		tr.BasedOn = refs[t.ID]
		out.Trades = append(out.Trades, tr)
	}
	for _, p := range ps {
		out.Positions = append(out.Positions, toAPIPosition(p))
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

// GET/POST /api/v1/chat —— 历史对话 / 发送消息。
func (s *Server) handleAPIChat(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.chatPostAPI(w, r)
		return
	case http.MethodGet:
	default:
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET/POST")
		return
	}
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

// GET/POST /api/v1/settings —— 配置展示 / 保存。
func (s *Server) handleAPISettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.settingsSaveAPI(w, r)
		return
	case http.MethodGet:
	default:
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET/POST")
		return
	}
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
