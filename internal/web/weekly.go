package web

// 周报聚合页(/weekly):本周行情快照 × 本周事件 × 本周个人笔记 + AI 综述(iter4 D26,Web 适配)。
// 综述:高智档(reasoning)每周一次生成,落 weekly_summaries 表按 ISO 周缓存;
// 手动按钮触发(POST action=generate),GET 永不调 LLM(缓存命中零成本)。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"piks/internal/ai"
	"piks/internal/store"
)

// WeeklyPage 周报页数据。
type WeeklyPage struct {
	Common
	Week       string
	Range      string
	Offset     int
	PrevOffset int
	NextOffset int
	Snaps      []WeeklySnap
	Events     []WeeklyEvent
	Notes      []WeeklyNote
	Trades     []WeeklyTrade // 本周交易(交易闭环,design trade-loop.md)
	Positions  []WeeklyPosition // 本周持仓(周末前最近快照)
	Summary    *store.WeeklySummary // 非空=已生成(展示);nil=空态占位
	SummaryNote string              // 降级/未配置/预算/失败提示(如实)
}

type WeeklySnap struct {
	Date     string
	Weekday  string
	Emotion  string // 情绪(英文,模板 zh)
	LimitUp  int
	LimitDown int
	Turnover string
	Judgment string
}

type WeeklyEvent struct {
	ID, Title, Date, EventType string
}

type WeeklyNote struct {
	ID, Title, Type, TypeLabel, Updated string
}

// WeeklyTrade 本周一笔交易(页面渲染与综述上下文同源)。
type WeeklyTrade struct {
	Date, Name, Code string
	Side, SideLabel  string
	Qty              int
	Price, Amount    float64
}

// WeeklyPosition 本周末最近快照的一只持仓。
type WeeklyPosition struct {
	Date, Code, Name  string
	Qty               int
	Cost, Price, MV, PL string
}

var cnWeekday = [7]string{"一", "二", "三", "四", "五", "六", "日"}

// genStatus 综述生成结果状态(经 redirect query ?g= 回显,页面如实标注)。
type genStatus string

const (
	genOK      genStatus = "ok"       // 综述已生成
	genNoData  genStatus = "nodata"   // 当周无行情/事件/沉淀,无可综述
	genNoCfg   genStatus = "noconfig" // AI 未配置
	genBudget  genStatus = "budget"   // 今日 AI 预算已用尽
	genFailed  genStatus = "failed"   // LLM 调用/解析失败
)

func (s *Server) handleWeekly(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, _ = strconv.Atoi(v)
	}
	ctx := r.Context()
	start, end, week, rng := weekRange(time.Now().In(cst), offset)

	// 生成综述(POST),成功后 302 回 GET 展示(避免刷新重复提交)。
	if r.Method == http.MethodPost {
		if r.FormValue("action") != "generate" {
			http.Redirect(w, r, "/weekly?offset="+strconv.Itoa(offset), http.StatusSeeOther)
			return
		}
		st := s.generateWeeklySummary(ctx, week, start, end)
		http.Redirect(w, r, "/weekly?offset="+strconv.Itoa(offset)+"&g="+string(st), http.StatusSeeOther)
		return
	}

	page := WeeklyPage{
		Common:     Common{Title: "周报 · PIKS", Active: "weekly"},
		Week:       week, Range: rng, Offset: offset,
		PrevOffset: offset + 1, NextOffset: offset - 1,
	}
	snaps, events, notes, trades, poss, err := s.aggregateWeek(ctx, start, end)
	if err != nil {
		s.fail(w, "weekly", &page.Common, err)
		return
	}
	page.Snaps, page.Events, page.Notes = snaps, events, notes
	page.Trades, page.Positions = trades, poss

	// 综述缓存(同周已生成 → 直接展示;无 → 空态占位)。
	sum, err := s.store.GetWeeklySummary(ctx, week)
	if err != nil {
		s.fail(w, "weekly", &page.Common, err)
		return
	}
	page.Summary = sum
	page.SummaryNote = genNote(r.URL.Query().Get("g"))
	if sum == nil && page.SummaryNote == "" {
		page.SummaryNote = "本周暂无 AI 综述。"
	}

	s.render(w, "weekly", page)
}

// genNote 把 ?g= 状态映射为页面如实提示。
func genNote(g string) string {
	switch genStatus(g) {
	case genOK:
		return "✅ 综述已生成(高智档)。"
	case genNoData:
		return "本周暂无行情/事件/沉淀数据,暂无可综述。"
	case genNoCfg:
		return "⚠️ AI 未配置(请到 /settings 填写服务地址与密钥),综述暂缺。"
	case genBudget:
		return "⚠️ 今日 AI 预算已用尽,综述暂缺(预算恢复后重试)。"
	case genFailed:
		return "⚠️ 综述生成失败(暂缺),请稍后重试。"
	}
	return ""
}

// aggregateWeek 聚合当周行情/事件/沉淀/交易/持仓(与页面同源,渲染与综述上下文共用)。
func (s *Server) aggregateWeek(ctx context.Context, start, end time.Time) ([]WeeklySnap, []WeeklyEvent, []WeeklyNote, []WeeklyTrade, []WeeklyPosition, error) {
	snaps, err := s.store.ListMarketSnapshots(ctx, 30)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var ss []WeeklySnap
	for _, sn := range snaps {
		day := sn.TradeDate.In(cst)
		if day.Before(start) || !day.Before(end) {
			continue
		}
		ws := WeeklySnap{
			Date:    day.Format("2006-01-02"),
			Weekday: "周" + cnWeekday[(int(day.Weekday())+6)%7],
			Emotion: orStr(sn.EmotionState, "—"),
		}
		if sn.LimitUpCount != nil {
			ws.LimitUp = *sn.LimitUpCount
		}
		if sn.LimitDownCount != nil {
			ws.LimitDown = *sn.LimitDownCount
		}
		if sn.TurnoverAmt != nil {
			ws.Turnover = fmt.Sprintf("%.0f 亿", *sn.TurnoverAmt)
		} else {
			ws.Turnover = "—"
		}
		ws.Judgment = orStr(sn.MyJudgment, "")
		ss = append(ss, ws)
	}

	evs, err := s.store.ListEventsBetween(ctx, start, end)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var ee []WeeklyEvent
	for _, e := range evs {
		d := e.CreatedAt.In(cst).Format("01-02")
		if e.OccurredAt != nil {
			d = e.OccurredAt.In(cst).Format("01-02")
		}
		ee = append(ee, WeeklyEvent{ID: e.ID, Title: e.Title, Date: d, EventType: e.EventType})
	}

	notes, err := s.store.ListPersonalNotesBetween(ctx, start, end)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var nn []WeeklyNote
	for _, n := range notes {
		nn = append(nn, WeeklyNote{
			ID: n.ID, Title: orStr(n.Title, n.Slug),
			Type: n.Type, TypeLabel: noteTypeLabel[n.Type], Updated: fmtTime(n.UpdatedAt),
		})
	}

	// 本周交易(交易闭环):trade_date 在周内。
	ts, err := s.store.ListTradesBetween(ctx, start, end)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var tt []WeeklyTrade
	for _, t := range ts {
		tt = append(tt, WeeklyTrade{
			Date: t.TradeDate.Format("01-02"), Name: t.Name, Code: t.Code,
			Side: t.Side, SideLabel: sideLabel(t.Side),
			Qty: t.Qty, Price: t.Price, Amount: t.Amount,
		})
	}

	// 本周持仓(周末前最近快照,防未来函数:不含周内之后快照)。
	poss, err := s.store.LatestPositionsBefore(ctx, end)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var pp []WeeklyPosition
	for _, p := range poss {
		v := WeeklyPosition{
			Date: p.SnapshotDate.Format("2006-01-02"), Code: p.Code, Name: p.Name, Qty: p.Qty,
			Cost: "—", Price: "—", MV: "—", PL: "—",
		}
		if p.CostPrice != nil {
			v.Cost = fmt.Sprintf("%.3f", *p.CostPrice)
		}
		if p.Price != nil {
			v.Price = fmt.Sprintf("%.3f", *p.Price)
		}
		if p.MarketValue != nil {
			v.MV = fmt.Sprintf("%.2f", *p.MarketValue)
		}
		if p.PL != nil {
			v.PL = fmt.Sprintf("%+.2f", *p.PL)
		}
		pp = append(pp, v)
	}
	return ss, ee, nn, tt, pp, nil
}

// generateWeeklySummary 用高智档(reasoning,未配置回退 extract)生成当周综述并入库。
// 预算/未配置/无数据/失败均如实返回状态,不调用则绝不调用,不报错不编造。
func (s *Server) generateWeeklySummary(ctx context.Context, week string, start, end time.Time) genStatus {
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		return genFailed
	}
	base, key := cfgMap["ai_service_base_url"], cfgMap["ai_api_key"]
	if base == "" || key == "" {
		return genNoCfg
	}
	model := cfgMap["ai_model_reasoning"] // 高智档(iter0 D2)
	if model == "" {
		model = cfgMap["ai_model_extract"]
	}
	if model == "" {
		return genNoCfg
	}

	// 预算护栏:今日已用 ≥ 预算 → 跳过(全局日账本一致,iter4 §4)。
	if budget, _ := strconv.ParseInt(cfgMap["ai_daily_token_budget"], 10, 64); budget > 0 {
		today, err := s.store.TokensSince(ctx, time.Now().Truncate(24*time.Hour))
		if err == nil && today >= budget {
			return genBudget
		}
	}

	snaps, events, notes, trades, poss, err := s.aggregateWeek(ctx, start, end)
	if err != nil {
		return genFailed
	}
	if len(snaps) == 0 && len(events) == 0 && len(notes) == 0 && len(trades) == 0 && len(poss) == 0 {
		return genNoData
	}

	system := `你是 PIKS 个人 A 股投资知识系统的周报综述助手。下方是本周已经整理好的数据。
规则:
- 用中文输出一段 ≤300 字的综述,概括本周市场情绪与事件脉络,并结合"本周沉淀"(个人笔记)与"本周交易/持仓"提示值得复盘的点(如:买入是否基于当周事件、持仓集中度、交易与笔记信念的印证/违背);
- 复盘视角,禁止输出买卖建议或预测涨跌;
- 严格只总结下方已列出的数据,禁止新增事实、数字或推断;
- 若某方面无数据,如实说明"本周无该方面数据",不要编造;
- 仅输出 JSON,结构为 {"summary":"..."}。`
	user := buildWeekContext(week, snaps, events, notes, trades, poss)

	runID, err := s.store.StartTaskRun(ctx, "weekly-summary")
	if err != nil {
		return genFailed
	}
	c := ai.NewOpenAICompat(base, key, model)
	resp, err := c.StructuredOutput(ctx, ai.StructuredRequest{System: system, User: user})
	if err != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"week": week})
		return genFailed
	}
	var out struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil || strings.TrimSpace(out.Summary) == "" {
		msg := "empty summary"
		if err != nil {
			msg = err.Error()
		}
		_ = s.store.FinishTaskRun(ctx, runID, "failed", msg, map[string]any{"week": week})
		return genFailed
	}
	tokens := resp.Usage.Total()
	_ = s.store.FinishTaskRun(ctx, runID, "success", "", map[string]any{"week": week, "model": model, "ai_tokens": tokens})
	if err := s.store.UpsertWeeklySummary(ctx, week, strings.TrimSpace(out.Summary), model, tokens); err != nil {
		return genFailed
	}
	return genOK
}

// buildWeekContext 把当周聚合数据拼成综述 LLM 上下文(与页面同源,含星期标签 + 交易/持仓段)。
func buildWeekContext(week string, snaps []WeeklySnap, events []WeeklyEvent, notes []WeeklyNote, trades []WeeklyTrade, poss []WeeklyPosition) string {
	var b strings.Builder
	fmt.Fprintf(&b, "周报 %s\n", week)
	b.WriteString("== 本周行情 ==\n")
	if len(snaps) == 0 {
		b.WriteString("(无)\n")
	}
	for _, sn := range snaps {
		fmt.Fprintf(&b, "%s %s 情绪=%s 涨停=%d 跌停=%d 成交=%s 我的判断=%s\n",
			sn.Date, sn.Weekday, sn.Emotion, sn.LimitUp, sn.LimitDown, sn.Turnover, orEmpty(sn.Judgment))
	}
	b.WriteString("\n== 本周事件 ==\n")
	if len(events) == 0 {
		b.WriteString("(无)\n")
	}
	for _, e := range events {
		fmt.Fprintf(&b, "%s %s (%s)\n", e.Date, e.Title, e.EventType)
	}
	b.WriteString("\n== 本周沉淀(个人笔记)==\n")
	if len(notes) == 0 {
		b.WriteString("(无)\n")
	}
	for _, n := range notes {
		fmt.Fprintf(&b, "[%s] %s\n", n.TypeLabel, n.Title)
	}
	b.WriteString("\n== 本周交易 ==\n")
	if len(trades) == 0 {
		b.WriteString("(无)\n")
	}
	for _, t := range trades {
		fmt.Fprintf(&b, "%s %s(%s) %s %d股 @%.3f 金额%.2f\n",
			t.Date, t.Name, t.Code, t.SideLabel, t.Qty, t.Price, t.Amount)
	}
	b.WriteString("\n== 本周持仓(截至最近快照)==\n")
	if len(poss) == 0 {
		b.WriteString("(无)\n")
	}
	for _, p := range poss {
		fmt.Fprintf(&b, "%s(%s) %d股 成本%s 现价%s 市值%s 盈亏%s\n",
			p.Name, p.Code, p.Qty, p.Cost, p.Price, p.MV, p.PL)
	}
	return b.String()
}

// orEmpty 空指针/空串 → "—"(综述上下文友好)。
func orEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// weekRange 当前周(北京时区)的 [start, end)。offset 表示往前 N 周。
func weekRange(now time.Time, offset int) (start, end time.Time, label, rng string) {
	t := now.AddDate(0, 0, -7*offset)
	wd := (int(t.Weekday()) + 6) % 7 // 周一=0
	mon := t.AddDate(0, 0, -wd)
	start = time.Date(mon.Year(), mon.Month(), mon.Day(), 0, 0, 0, 0, cst)
	end = start.AddDate(0, 0, 7)
	y, w := mon.ISOWeek()
	label = fmt.Sprintf("%04d-W%02d", y, w)
	rng = start.Format("01-02") + " ~ " + end.AddDate(0, 0, -1).Format("01-02")
	return
}
