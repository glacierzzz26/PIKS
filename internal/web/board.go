package web

import (
	"math"
	"net/http"
	"time"
)

// 早/晚档窗口 + 榜单(issue #83 分期 P-3,P1 窗口与打分接口)。
//
// 窗口理由 = **阅读节奏**(早上看隔夜+盘前 / 晚上看全天),**非热度切口**:
//
//	late  晚盘:当日 09:15 → 当日 18:30(9.25h)
//	early 早盘:前一日 18:30 → 当日 09:15(14.75h)
//
// 衔接:`early(d).end == late(d).start == d 09:15`、`late(d).end == early(d+1).start == d 18:30`
// ⇒ **无重叠、无缝隙**(单测钉死)。
//
// ⚠️ **排序信号 = 跨渠道独立报道数,作印证度标签,不排名**(issue P1 红线):本版榜单**按时间序**,
// 归一化加权 `boardScore` 照算并带出,但**不用于排序**(留给后续权重与实体/自选信号)。
//
// ⚠️ **窗口锚事件 `created_at`(入库/抽取时刻)**,不是 `occurred_at` —— 见 store.ListEventsInWindow。

// 窗口边界(北京时区,同日)。
const (
	boardLateStartH, boardLateStartM = 9, 15  // 晚盘起 09:15
	boardLateEndH, boardLateEndM     = 18, 30 // 晚盘止 / 早盘起 18:30
)

// BoardStage 档位标识。
const (
	BoardStageEarly = "early" // 早盘(前一日 18:30 → 当日 09:15)
	BoardStageLate  = "late"  // 晚盘(当日 09:15 → 当日 18:30)
)

// boardWindow 返回 date(北京日)对应档位的半开窗口 [start, end)。
// stage 非 early/late 时按「当前北京时刻」选档(>=18:30 → late,否则 early)。
func boardWindow(date time.Time, stage string) (start, end time.Time) {
	y, m, d := date.In(cst).Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, cst)
	lateStart := day.Add(time.Duration(boardLateStartH)*time.Hour + time.Duration(boardLateStartM)*time.Minute)
	lateEnd := day.Add(time.Duration(boardLateEndH)*time.Hour + time.Duration(boardLateEndM)*time.Minute)
	switch stage {
	case BoardStageLate:
		return lateStart, lateEnd
	case BoardStageEarly:
		return lateEnd.AddDate(0, 0, -1), lateStart // 前一日 18:30 → 当日 09:15
	default:
		return lateStart, lateEnd
	}
}

// boardStageNow 按当前北京时刻选默认档:>=18:30 → 晚盘,否则早盘(定稿见设计文档)。
func boardStageNow(now time.Time) string {
	l := now.In(cst)
	if l.Hour() > boardLateEndH || (l.Hour() == boardLateEndH && l.Minute() >= boardLateEndM) {
		return BoardStageLate
	}
	return BoardStageEarly
}

// Signals 打分信号(issue #83 P1 的归一化加权接口)。本版只有 CrossChannel 非 0。
type Signals struct {
	CrossChannel float64 // 跨渠道**独立**报道数(转载已并;P-1 的 independent_count)
	Entity       float64 // 命中实体数(本版恒 0,接口先立)
	Watch        float64 // 是否自选股(本版恒 0,接口先立)
}

// Weights 各信号权重。本版 {CrossChannel:1, Entity:0, Watch:0}(issue P1)。
type Weights struct {
	CrossChannel float64
	Entity       float64
	Watch        float64
}

// boardWeights 本版权重(将来调权重只改这里,接口形状不变)。
var boardWeights = Weights{CrossChannel: 1, Entity: 0, Watch: 0}

// boardScore 归一化加权分:`score = Σ wᵢ · normᵢ(信号ᵢ)`,normᵢ = sigᵢ / maxᵢ(窗口内最大值)。
// max 为 0(窗口内该项信号全为 0)时该项贡献 0,避免除零。
//
// ⚠️ **本版 score 不用于排序**(榜单按时间序;issue P1「不排名」)。它是接口预留:
// 将来引入实体/自选信号与权重后,此处即为排序依据,不必再改响应形状。
func boardScore(sig, max Signals, w Weights) float64 {
	norm := func(v, m float64) float64 {
		if m <= 0 {
			return 0
		}
		return v / m
	}
	return w.CrossChannel*norm(sig.CrossChannel, max.CrossChannel) +
		w.Entity*norm(sig.Entity, max.Entity) +
		w.Watch*norm(sig.Watch, max.Watch)
}

// apiBoard 榜单响应(issue #83 P-3)。
//
// 排序**按时间序**(items 已由 store 按 created_at DESC 排好),`score` 带出但**不用于排序**。
// 印证度**标签**(单一来源/多家印证/广泛报道)由前端从 item 的 `independent_count` 派生 ——
// 后端只给客观计数,不新增中文标签(P-1 既有约定)。窗口内**全集不截断**(issue P1)。
type apiBoard struct {
	Stage       string        `json:"stage"`
	Date        string        `json:"date"`
	WindowStart string        `json:"window_start"`
	WindowEnd   string        `json:"window_end"`
	Count       int           `json:"count"`
	Items       []apiBoardRow `json:"items"`
}

// apiBoardRow 榜单一行 = 事件流投影 + 归一化加权分。
// 内嵌 apiEventItem(而非平铺),使榜单行与 `/api/v1/events` 单条**结构一致**,
// 前端复用同一渲染;`score` 是榜单特有字段。
type apiBoardRow struct {
	apiEventItem
	Score float64 `json:"score"`
}

// GET /api/v1/board?stage=early|late&date=YYYY-MM-DD —— 早/晚档事件榜单。
// stage 缺省按当前北京时刻选档;date 缺省为北京当日。
func (s *Server) handleAPIBoard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stage := r.URL.Query().Get("stage")
	if stage != BoardStageEarly && stage != BoardStageLate {
		stage = boardStageNow(time.Now())
	}
	date := time.Now()
	if ds := r.URL.Query().Get("date"); ds != "" {
		d, err := time.ParseInLocation("2006-01-02", ds, cst)
		if err != nil {
			http.Error(w, "bad date (want YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
		date = d
	}
	start, end := boardWindow(date, stage)

	evs, err := s.store.ListEventsInWindow(ctx, start, end)
	if err != nil {
		s.apiErr(w, "board", err)
		return
	}
	ents, err := s.store.ListAllEntities(ctx)
	if err != nil {
		s.apiErr(w, "entities", err)
		return
	}
	idx := buildNameIndex(ents)

	clusterIDs := make([]string, 0, len(evs))
	for _, ev := range evs {
		if ev.ClusterID != nil {
			clusterIDs = append(clusterIDs, *ev.ClusterID)
		}
	}
	clusters, err := s.store.ListClusterSources(ctx, clusterIDs)
	if err != nil {
		s.apiErr(w, "cluster sources", err)
		return
	}
	members, err := s.store.ListClusterMembersWithFacts(ctx, clusterIDs)
	if err != nil {
		s.apiErr(w, "cluster members", err)
		return
	}

	// 归一化需要窗口内各信号的**最大值**;本版只有跨渠道独立报道数非 0(实体/自选信号恒 0)。
	maxSig := Signals{}
	items := make([]apiEventItem, len(evs))
	maxIndep := 0
	for i, ev := range evs {
		items[i] = toEventItem(ev, idx, clusters, members)
		if items[i].IndependentCount > maxIndep {
			maxIndep = items[i].IndependentCount
		}
	}
	maxSig.CrossChannel = float64(maxIndep)

	rows := make([]apiBoardRow, len(items))
	for i := range items {
		sig := Signals{CrossChannel: float64(items[i].IndependentCount)}
		rows[i] = apiBoardRow{apiEventItem: items[i], Score: round2(boardScore(sig, maxSig, boardWeights))}
	}
	// 时间序沿用 store 的 created_at DESC(见 ListEventsInWindow),此处**不重排** —— 榜单按时间、
	// 不按 score(issue P1「不排名」)。

	y, m, d := date.In(cst).Date()
	s.writeJSON(w, apiBoard{
		Stage:       stage,
		Date:        time.Date(y, m, d, 0, 0, 0, 0, cst).Format("2006-01-02"),
		WindowStart: start.Format(time.RFC3339),
		WindowEnd:   end.Format(time.RFC3339),
		Count:       len(rows),
		Items:       rows,
	})
}

// round2 保留两位小数(与看板 Score 同一精度口径)。
func round2(f float64) float64 { return math.Round(f*100) / 100 }
