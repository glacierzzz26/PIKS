package web

// 交易页(/trades):交易记录 × 持仓快照 + 截图导入(AI 视觉抽取 → 预览确认)+ 手动录入 + AI 带引用复盘。
// 设计 docs/phase2/design/trades.md。纪律:GET 永不调 LLM;导入/解读均手动显式触发;
// 截图抽取后必须预览确认才入库(防误识别污染);解读为复盘视角非建议,带引用且防自造。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

const tradeMaxUpload = 5 << 20 // 5MB

type TradesPage struct {
	Common
	Trades    []TradeView
	Positions []PositionView
	Note      string
	Preview   *ImportPreview // 非空 = 导入预览态(待确认)
}

type TradeView struct {
	ID, Code, Name string
	Side, SideLabel string
	Date           string
	Price, Amount  float64
	Qty            int
	Note           string
	Source         string
	Review         *TradeReviewView
}

type PositionView struct {
	Date, Code, Name                       string
	Qty                                    int
	CostPrice, Price, MarketValue, PL      string
}

// TradeReviewView 复盘渲染视图(解包 review JSONB)。
type TradeReviewView struct {
	Text     string
	Model    string
	Tokens   int64
	GenAt    string
	RefEvents, RefEntities, RefNotes []store.ChatRef
	Mistakes []tradeMistake
}

// ImportPreview 截图抽取预览(确认前不落库)。
type ImportPreview struct {
	Kind         string // trade / position
	AttachmentID string
	Trades       []PreviewTrade
	Positions    []PreviewPosition
}

type PreviewTrade struct {
	Include bool
	Date, Code, Name, Side string
	Price, Qty, Amount     string
	Exists                 bool // 与既有交易重复提示
}

type PreviewPosition struct {
	Include   bool
	Code, Name, Qty, CostPrice, Price, MarketValue, PL string
}

// tradeReview 复盘 JSONB 结构(与模板/存储契约)。
type tradeReview struct {
	Review   string        `json:"review"`
	Refs     tradeRefs     `json:"refs"`
	Mistakes []tradeMistake `json:"mistakes"`
	Model    string        `json:"model"`
	Tokens   int64         `json:"tokens"`
	GenAt    string        `json:"generated_at"`
}

type tradeRefs struct {
	Events   []store.ChatRef `json:"events"`
	Entities []store.ChatRef `json:"entities"`
	Notes    []store.ChatRef `json:"notes"`
}

type tradeMistake struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (s *Server) handleTrades(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		switch r.FormValue("action") {
		case "add":
			s.tradeAdd(w, r)
		case "import":
			s.tradeImport(w, r)
		case "confirm":
			s.tradeConfirm(w, r)
		default:
			http.Redirect(w, r, "/trades", http.StatusSeeOther)
		}
		return
	}
	s.tradesShow(w, r, nil, "")
}

// tradesShow 渲染交易页。preview 非空 = 导入预览态。
func (s *Server) tradesShow(w http.ResponseWriter, r *http.Request, preview *ImportPreview, note string) {
	ctx := r.Context()
	if note == "" {
		note = tradeNote(r.URL.Query().Get("g"))
	}
	page := TradesPage{
		Common: Common{Title: "交易 · PIKS", Active: "trades"},
		Note:   note,
		Preview: preview,
	}
	ts, err := s.store.ListTrades(ctx, 200)
	if err != nil {
		s.fail(w, "trades", &page.Common, err)
		return
	}
	ps, err := s.store.LatestPositions(ctx)
	if err != nil {
		s.fail(w, "trades", &page.Common, err)
		return
	}
	page.Trades = toTradeViews(ts)
	page.Positions = toPositionViews(ps)
	s.render(w, "trades", page)
}

func toTradeViews(ts []model.Trade) []TradeView {
	out := make([]TradeView, 0, len(ts))
	for _, t := range ts {
		v := TradeView{
			ID: t.ID, Code: t.Code, Name: t.Name,
			Side: t.Side, SideLabel: sideLabel(t.Side),
			Date: t.TradeDate.Format("01-02"), Price: t.Price,
			Amount: t.Amount, Qty: t.Qty, Source: t.Source,
		}
		if t.Note != nil {
			v.Note = *t.Note
		}
		if len(t.Review) > 0 && string(t.Review) != "{}" {
			v.Review = parseTradeReview(t.Review)
		}
		out = append(out, v)
	}
	return out
}

func sideLabel(side string) string {
	switch side {
	case "buy":
		return "买入"
	case "sell":
		return "卖出"
	}
	return side
}

func toPositionViews(ps []model.Position) []PositionView {
	out := make([]PositionView, 0, len(ps))
	for _, p := range ps {
		v := PositionView{
			Date: p.SnapshotDate.Format("2006-01-02"), Code: p.Code, Name: p.Name, Qty: p.Qty,
		}
		if p.CostPrice != nil {
			v.CostPrice = fmt.Sprintf("%.3f", *p.CostPrice)
		} else {
			v.CostPrice = "—"
		}
		if p.Price != nil {
			v.Price = fmt.Sprintf("%.3f", *p.Price)
		} else {
			v.Price = "—"
		}
		if p.MarketValue != nil {
			v.MarketValue = fmt.Sprintf("%.2f", *p.MarketValue)
		} else {
			v.MarketValue = "—"
		}
		if p.PL != nil {
			v.PL = fmt.Sprintf("%+.2f", *p.PL)
		} else {
			v.PL = "—"
		}
		out = append(out, v)
	}
	return out
}

func parseTradeReview(raw json.RawMessage) *TradeReviewView {
	var rv tradeReview
	if err := json.Unmarshal(raw, &rv); err != nil || rv.Review == "" {
		return nil
	}
	v := &TradeReviewView{
		Text: rv.Review, Model: rv.Model, Tokens: rv.Tokens, GenAt: rv.GenAt,
		RefEvents: rv.Refs.Events, RefEntities: rv.Refs.Entities, RefNotes: rv.Refs.Notes,
		Mistakes: rv.Mistakes,
	}
	return v
}

// tradeAdd 手动录入兜底。
func (s *Server) tradeAdd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := strings.TrimSpace(r.FormValue("name"))
	code := strings.TrimSpace(r.FormValue("code"))
	side := strings.TrimSpace(r.FormValue("side"))
	if name == "" || code == "" || (side != "buy" && side != "sell") {
		s.tradesShow(w, r, nil, "⚠️ 请填写证券名称、代码与买卖方向。")
		return
	}
	price, err1 := strconv.ParseFloat(r.FormValue("price"), 64)
	qty, err2 := strconv.Atoi(r.FormValue("qty"))
	if err1 != nil || err2 != nil || qty <= 0 || price < 0 {
		s.tradesShow(w, r, nil, "⚠️ 价格/数量格式不正确。")
		return
	}
	var date time.Time
	if d := r.FormValue("trade_date"); d != "" {
		parsed, perr := time.ParseInLocation("2006-01-02", d, cst)
		if perr != nil {
			s.tradesShow(w, r, nil, "⚠️ 交易日期格式不正确(应为 YYYY-MM-DD)。")
			return
		}
		date = parsed
	} else {
		date = time.Now().In(cst)
	}
	var note *string
	if n := strings.TrimSpace(r.FormValue("note")); n != "" {
		note = &n
	}
	if err := s.store.InsertTrades(ctx, []model.Trade{{
		TradeDate: date, Code: code, Name: name, Side: side,
		Price: price, Qty: qty, Amount: price * float64(qty), Source: "manual", Note: note,
	}}); err != nil {
		s.tradesShow(w, r, nil, "⚠️ 入库失败: "+err.Error())
		return
	}
	if _, err := s.store.EnsureCompanyEntity(ctx, code, name); err != nil {
		s.tradesShow(w, r, nil, "⚠️ 交易已入库,但实体补全失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/trades?g=added", http.StatusSeeOther)
}

// tradeImport 截图导入:视觉抽取 → 预览态(不落库)。
func (s *Server) tradeImport(w http.ResponseWriter, r *http.Request) {
	kind := r.FormValue("type")
	if kind != "trade" && kind != "position" {
		s.tradesShow(w, r, nil, "⚠️ 请选择截图类型(今日交易 / 持仓)。")
		return
	}
	if err := r.ParseMultipartForm(tradeMaxUpload + 1<<20); err != nil {
		s.tradesShow(w, r, nil, "⚠️ 表单解析失败(文件超 5MB?): "+err.Error())
		return
	}
	file, fh, err := r.FormFile("file")
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 未收到截图文件。")
		return
	}
	defer file.Close()
	if !allowedImageType(fh.Header.Get("Content-Type")) {
		s.tradesShow(w, r, nil, "⚠️ 仅支持图片(png/jpeg/webp/gif)。")
		return
	}
	data, rerr := io.ReadAll(io.LimitReader(file, tradeMaxUpload+1))
	if rerr != nil {
		s.tradesShow(w, r, nil, "⚠️ 读文件失败: "+rerr.Error())
		return
	}
	if len(data) > tradeMaxUpload {
		s.tradesShow(w, r, nil, "⚠️ 图片超过 5MB 上限。")
		return
	}

	ctx := r.Context()
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 读 AI 配置失败: "+err.Error())
		return
	}
	base, key := cfgMap["ai_service_base_url"], cfgMap["ai_api_key"]
	vision := cfgMap["ai_model_vision"]
	if base == "" || key == "" || vision == "" {
		s.tradesShow(w, r, nil, "⚠️ AI 未配置或视觉模型未配置(请到 /settings 配置 `ai_model_vision`),可用手动录入兜底。")
		return
	}
	// 预算护栏(全局日账本)。
	if budget, _ := strconv.ParseInt(cfgMap["ai_daily_token_budget"], 10, 64); budget > 0 {
		if today, err := s.store.TokensSince(ctx, time.Now().Truncate(24*time.Hour)); err == nil && today >= budget {
			s.tradesShow(w, r, nil, "⚠️ 今日 AI 预算已用尽,导入暂缓(预算恢复后重试)。")
			return
		}
	}

	attID, err := s.saveUpload(ctx, fh.Filename, fh.Header.Get("Content-Type"), data)
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 保存截图失败: "+err.Error())
		return
	}

	runID, err := s.store.StartTaskRun(ctx, "trade-import")
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 记账失败: "+err.Error())
		return
	}
	c := ai.NewOpenAICompat(base, key, vision)
	system, user, schema := importPrompt(kind)
	resp, err := c.StructuredOutput(ctx, ai.StructuredRequest{
		System: system, User: user, Schema: json.RawMessage(schema),
		Image: &ai.ImagePart{Data: data, MIME: fh.Header.Get("Content-Type")},
	})
	if err != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"kind": kind})
		s.tradesShow(w, r, nil, "⚠️ 截图识别失败: "+err.Error())
		return
	}
	_ = s.store.FinishTaskRun(ctx, runID, "success", "", map[string]any{"kind": kind, "model": vision, "ai_tokens": resp.Usage.Total()})

	preview, perr := buildImportPreview(ctx, s.store, kind, attID, resp.Data)
	if perr != nil {
		s.tradesShow(w, r, nil, "⚠️ 识别结果解析失败: "+perr.Error())
		return
	}
	if preview == nil || (len(preview.Trades) == 0 && len(preview.Positions) == 0) {
		s.tradesShow(w, r, nil, "⚠️ 未识别到交易/持仓,请检查截图是否为同花顺今日交易/持仓页,或手动录入。")
		return
	}
	s.tradesShow(w, r, preview, "✅ 已识别 "+fmt.Sprintf("%d", len(preview.Trades)+len(preview.Positions))+" 条,请核对后「导入勾选项」(名称/代码/价格可编辑)。")
}

// importPrompt 返回截图抽取的 system/user/schema(与 design trades.md §2.2 一致)。
func importPrompt(kind string) (system, user, schema string) {
	if kind == "position" {
		system = `你是 PIKS 的交易截图识别助手。识别同花顺 App「持仓」截图,抽取结构化持仓数据。
规则:
- 只抽取截图中明确出现的条目;字段缺失标 null,禁止推断或补全;
- 名称映射 code(6 位)用截图标注;无代码则 code 填名称;
- 若图片不是持仓截图,返回空数组 {"positions":[]},不要编造;
- 仅输出 JSON。`
		user = "识别这张持仓截图,输出持仓列表。"
		schema = `{"type":"object","properties":{"positions":{"type":"array","items":{"type":"object","properties":{"code":{"type":"string"},"name":{"type":"string"},"qty":{"type":"number"},"cost_price":{"type":"number"},"price":{"type":"number"},"market_value":{"type":"number"},"pl":{"type":"number"}}}}}}`
		return
	}
	system = `你是 PIKS 的交易截图识别助手。识别同花顺 App「今日交易」截图,抽取结构化交易记录。
规则:
- 只抽取截图中明确出现的交易;字段缺失标 null,禁止推断或补全;
- side 用 buy(买入)/sell(卖出)映射截图的「买入/卖出」;
- date 用截图显示的交易日期(格式 2006-01-02);无 code 则 code 填名称;
- 若图片不是今日交易截图,返回空数组 {"trades":[]},不要编造;
- 仅输出 JSON。`
	user = "识别这张今日交易截图,输出交易列表。"
	schema = `{"type":"object","properties":{"trades":{"type":"array","items":{"type":"object","properties":{"date":{"type":"string"},"code":{"type":"string"},"name":{"type":"string"},"side":{"type":"string"},"price":{"type":"number"},"qty":{"type":"number"},"amount":{"type":"number"}}}}}}`
	return
}

// buildImportPreview 把抽取 JSON → 预览行(去重标记 TradeExists)。
func buildImportPreview(ctx context.Context, st *store.Store, kind, attID string, data json.RawMessage) (*ImportPreview, error) {
	prev := &ImportPreview{Kind: kind, AttachmentID: attID}
	// 注意:此处必须用带 json 标签的内联 struct。model.Position 只有 db 标签,
	// encoding/json 无法把 cost_price/market_value 这类 snake_case 键匹配到
	// CostPrice/MarketValue 字段(会静默丢弃),交易路径同样用内联 struct。
	if kind == "position" {
		var out struct {
			Positions []struct {
				Code        string   `json:"code"`
				Name        string   `json:"name"`
				Qty         int      `json:"qty"`
				CostPrice   *float64 `json:"cost_price"`
				Price       *float64 `json:"price"`
				MarketValue *float64 `json:"market_value"`
				PL          *float64 `json:"pl"`
			} `json:"positions"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		for _, p := range out.Positions {
			if strings.TrimSpace(p.Name) == "" {
				continue
			}
			qty := ""
			if p.Qty > 0 {
				qty = strconv.Itoa(p.Qty)
			}
			prev.Positions = append(prev.Positions, PreviewPosition{
				Include: true, Code: p.Code, Name: p.Name, Qty: qty,
				CostPrice: ftoa(p.CostPrice), Price: ftoa(p.Price),
				MarketValue: ftoa(p.MarketValue), PL: ftoa(p.PL),
			})
		}
		return prev, nil
	}
	var out struct {
		Trades []struct {
			Date, Code, Name, Side string
			Price, Amount          *float64
			Qty                    *int
		} `json:"trades"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	for _, t := range out.Trades {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		side := strings.ToLower(t.Side)
		if side != "buy" && side != "sell" {
			if strings.Contains(t.Side, "卖") {
				side = "sell"
			} else {
				side = "buy"
			}
		}
		price, qty, amt := "", "", ""
		if t.Price != nil {
			price = trimF(*t.Price)
		}
		if t.Qty != nil {
			qty = strconv.Itoa(*t.Qty)
		}
		if t.Amount != nil {
			amt = trimF(*t.Amount)
		}
		exists := false
		if t.Date != "" && t.Code != "" && qty != "" {
			if d, err := time.ParseInLocation("2006-01-02", t.Date, cst); err == nil {
				if n, err := strconv.Atoi(qty); err == nil {
					exists, _ = st.TradeExists(ctx, d, t.Code, side, n)
				}
			}
		}
		prev.Trades = append(prev.Trades, PreviewTrade{
			Include: !exists, Exists: exists,
			Date: t.Date, Code: t.Code, Name: t.Name, Side: side,
			Price: price, Qty: qty, Amount: amt,
		})
	}
	return prev, nil
}

func ftoa(p *float64) string {
	if p == nil {
		return ""
	}
	return trimF(*p)
}

func trimF(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// tradeConfirm 预览确认:批量入库 + 实体补全。
func (s *Server) tradeConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind := r.FormValue("kind")
	attID := r.FormValue("attachment_id")
	if kind == "position" {
		var ps []model.Position
		codes := r.Form["p_code"]
		names := r.Form["p_name"]
		qtys := r.Form["p_qty"]
		incs := r.Form["p_inc"]
		incSet := map[int]bool{}
		for _, v := range incs {
			if i, err := strconv.Atoi(v); err == nil {
				incSet[i] = true
			}
		}
		for i, name := range names {
			if !incSet[i] || strings.TrimSpace(name) == "" {
				continue
			}
			code := formAt(codes, i)
			if code == "" {
				code = name
			}
			qty, _ := strconv.Atoi(formAt(qtys, i))
			if qty <= 0 {
				continue
			}
			ps = append(ps, model.Position{
				SnapshotDate: time.Now().In(cst), Code: code, Name: name, Qty: qty,
				CostPrice:   parseF(formAt(r.Form["p_cost"], i)),
				Price:       parseF(formAt(r.Form["p_price"], i)),
				MarketValue: parseF(formAt(r.Form["p_mv"], i)),
				PL:          parseF(formAt(r.Form["p_pl"], i)),
				Source:      "screenshot", AttachmentID: optStr(attID),
			})
			if _, err := s.store.EnsureCompanyEntity(ctx, code, name); err != nil {
				s.tradesShow(w, r, nil, "⚠️ 实体补全失败: "+err.Error())
				return
			}
		}
		if len(ps) == 0 {
			s.tradesShow(w, r, nil, "⚠️ 没有勾选任何持仓行。")
			return
		}
		if err := s.store.InsertPositions(ctx, ps); err != nil {
			s.tradesShow(w, r, nil, "⚠️ 持仓入库失败: "+err.Error())
			return
		}
		http.Redirect(w, r, "/trades?g=positions_imported", http.StatusSeeOther)
		return
	}

	var ts []model.Trade
	names := r.Form["t_name"]
	incs := r.Form["t_inc"]
	incSet := map[int]bool{}
	for _, v := range incs {
		if i, err := strconv.Atoi(v); err == nil {
			incSet[i] = true
		}
	}
	for i, name := range names {
		if !incSet[i] || strings.TrimSpace(name) == "" {
			continue
		}
		code := formAt(r.Form["t_code"], i)
		if code == "" {
			code = name
		}
		side := formAt(r.Form["t_side"], i)
		if side != "buy" && side != "sell" {
			continue
		}
		price := parseF(formAt(r.Form["t_price"], i))
		qty, _ := strconv.Atoi(formAt(r.Form["t_qty"], i))
		if price == nil || *price < 0 || qty <= 0 {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", formAt(r.Form["t_date"], i), cst)
		if err != nil {
			date = time.Now().In(cst)
		}
		ts = append(ts, model.Trade{
			TradeDate: date, Code: code, Name: name, Side: side,
			Price: *price, Qty: qty, Amount: *price * float64(qty),
			Source: "screenshot", AttachmentID: optStr(attID),
		})
		if _, err := s.store.EnsureCompanyEntity(ctx, code, name); err != nil {
			s.tradesShow(w, r, nil, "⚠️ 实体补全失败: "+err.Error())
			return
		}
	}
	if len(ts) == 0 {
		s.tradesShow(w, r, nil, "⚠️ 没有勾选任何交易行。")
		return
	}
	if err := s.store.InsertTrades(ctx, ts); err != nil {
		s.tradesShow(w, r, nil, "⚠️ 交易入库失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/trades?g=imported", http.StatusSeeOther)
}

// tradeNote 把 ?g= 状态映射为页面如实提示。
func tradeNote(g string) string {
	switch g {
	case "added":
		return "✅ 手动录入已入库(实体已补全)。"
	case "imported":
		return "✅ 交易已导入(实体已补全)。"
	case "positions_imported":
		return "✅ 持仓快照已导入。"
	case "reviewed":
		return "✅ 复盘已生成(带引用,可点跳转)。"
	case "mistake_saved":
		return "✅ 已存为个人笔记(type=mistake,状态 hypothesis,可在 /notes 查看)。"
	}
	return ""
}

// formAt 取重复表单字段第 i 个值,越界返回 ""(防索引 panic)。
func formAt(vals []string, i int) string {
	if i < 0 || i >= len(vals) {
		return ""
	}
	return vals[i]
}

func parseF(v string) *float64 {
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}

// optStr 空串 → nil(可空列不入 NULL),非空 → 指针。
func optStr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// handleTrade /trades/{id}/review(POST 解读)与 /trades/{id}/save-mistake/{n}(POST 存复盘点为笔记)。
func (s *Server) handleTrade(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/trades/")
	if r.Method == http.MethodPost {
		if strings.HasSuffix(rest, "/review") {
			s.tradeReview(w, r, strings.TrimSuffix(rest, "/review"))
			return
		}
		if idx := strings.LastIndex(rest, "/save-mistake/"); idx > 0 {
			id := rest[:idx]
			n, _ := strconv.Atoi(rest[idx+len("/save-mistake/"):])
			s.tradeSaveMistake(w, r, id, n)
			return
		}
	}
	http.Redirect(w, r, "/trades", http.StatusSeeOther)
}

// tradeSaveMistake 把复盘候选 mistake 存为个人笔记(type=mistake, status=hypothesis)。
// iter4 单向 harvest 语义:AI 提议、用户确认才入库;已存过(同 slug)如实提示不重复建。
func (s *Server) tradeSaveMistake(w http.ResponseWriter, r *http.Request, id string, n int) {
	ctx := r.Context()
	t, err := s.store.GetTrade(ctx, id)
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 交易不存在: "+err.Error())
		return
	}
	rv := parseTradeReview(t.Review)
	if rv == nil || n < 0 || n >= len(rv.Mistakes) {
		s.tradesShow(w, r, nil, "⚠️ 复盘点不存在(复盘可能已更新,请重新解读)。")
		return
	}
	m := rv.Mistakes[n]
	title := m.Title
	if title == "" {
		title = "交易复盘候选-" + id[:8]
	}
	slug := fmt.Sprintf("trade-%s-%d", id, n)
	if existing, err := s.store.GetPersonalNoteBySlug(ctx, "mistake", slug); err == nil && existing != nil {
		s.tradesShow(w, r, nil, "✅ 该复盘点已存为笔记,未重复创建。")
		return
	} else if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 查重失败: "+err.Error())
		return
	}
	if _, err := s.store.CreatePersonalNote(ctx, &model.PersonalNote{
		Type: "mistake", Slug: slug, Title: &title,
		Status: "hypothesis", Content: &m.Content,
	}); err != nil {
		s.tradesShow(w, r, nil, "⚠️ 存为笔记失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/trades?g=mistake_saved", http.StatusSeeOther)
}

// tradeReview 交易 AI 复盘(带引用,手动触发)。
func (s *Server) tradeReview(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	t, err := s.store.GetTrade(ctx, id)
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 交易不存在: "+err.Error())
		return
	}
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 读 AI 配置失败: "+err.Error())
		return
	}
	base, key := cfgMap["ai_service_base_url"], cfgMap["ai_api_key"]
	model := cfgMap["ai_model_reasoning"]
	if model == "" {
		model = cfgMap["ai_model_extract"]
	}
	if base == "" || key == "" || model == "" {
		s.tradesShow(w, r, nil, "⚠️ AI 未配置,复盘暂缺(请到 /settings 配置)。")
		return
	}
	if budget, _ := strconv.ParseInt(cfgMap["ai_daily_token_budget"], 10, 64); budget > 0 {
		if today, err := s.store.TokensSince(ctx, time.Now().Truncate(24*time.Hour)); err == nil && today >= budget {
			s.tradesShow(w, r, nil, "⚠️ 今日 AI 预算已用尽,复盘暂缺(预算恢复后重试)。")
			return
		}
	}

	// KB grounding:股票名/代码 → 同义扩展 + 检索(事件/实体)+ 个人笔记。
	q := t.Name
	if t.Code != "" && t.Code != t.Name {
		q += " " + t.Code
	}
	extra, _ := s.expandQuery(ctx, cfgMap, q)
	events, entities, err := s.store.SearchKnowledgeExpanded(ctx, q, extra, 8, 8)
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 检索知识库失败: "+err.Error())
		return
	}
	// 防未来函数:历史交易只含 trade_date 及之前的语境。
	cutoff := t.TradeDate.AddDate(0, 0, 1)
	kept := events[:0]
	for _, e := range events {
		if evWithin(e, cutoff) {
			kept = append(kept, e)
		}
	}
	events = kept
	notes, err := s.store.ListPersonalNotesByText(ctx, t.Name, 8)
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 检索个人笔记失败: "+err.Error())
		return
	}

	runID, err := s.store.StartTaskRun(ctx, "trade-review")
	if err != nil {
		s.tradesShow(w, r, nil, "⚠️ 记账失败: "+err.Error())
		return
	}
	system, user := reviewPrompt(t, events, entities, notes)
	c := ai.NewOpenAICompat(base, key, model)
	resp, err := c.StructuredOutput(ctx, ai.StructuredRequest{System: system, User: user})
	if err != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"trade_id": id})
		s.tradesShow(w, r, nil, "⚠️ 复盘生成失败: "+err.Error())
		return
	}
	rv, verr := validateTradeReview(resp.Data, events, entities, notes, model, resp.Usage.Total())
	if verr != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", verr.Error(), map[string]any{"trade_id": id})
		s.tradesShow(w, r, nil, "⚠️ 复盘解析失败: "+verr.Error())
		return
	}
	_ = s.store.FinishTaskRun(ctx, runID, "success", "", map[string]any{"trade_id": id, "model": model, "ai_tokens": resp.Usage.Total()})
	raw, _ := json.Marshal(rv)
	if err := s.store.SetTradeReview(ctx, id, raw); err != nil {
		s.tradesShow(w, r, nil, "⚠️ 复盘入库失败: "+err.Error())
		return
	}
	http.Redirect(w, r, "/trades?g=reviewed", http.StatusSeeOther)
}

// evWithin 事件是否发生在截止时间(含)之前(防未来函数)。
func evWithin(e model.Event, cutoff time.Time) bool {
	t := e.CreatedAt
	if e.OccurredAt != nil {
		t = *e.OccurredAt
	}
	return t.Before(cutoff)
}

// reviewPrompt 组装复盘 system/user。
func reviewPrompt(t *model.Trade, events []model.Event, entities []model.Entity, notes []model.PersonalNote) (system, user string) {
	system = `你是 PIKS 个人 A 股投资知识系统的交易复盘助手。下方是这笔交易 + 知识库检索结果(相关事件/实体/你的个人笔记)。
规则:
- 用中文输出一段 ≤300 字的复盘:这笔交易处于什么市场/事件背景、与你的哪些笔记信念相关、值得复盘什么(是否印证或违背你的笔记);
- 复盘视角,禁止输出买卖建议或预测涨跌;
- 严格只基于下方数据;知识库无相关事件/实体/笔记时如实说明「知识库无该股票相关事件/实体」,不要编造;
- refs 的 id 必须来自下方方括号标注(E:事件 id / N:实体 id / P:笔记 id),没有关联就留空数组,不要自造;
- 若发现交易与你的信念/案例相悖或疑似重复已知错误模式,在 mistakes 提议(标题+内容),否则留空;
- 仅输出 JSON,结构: {"review":"...","refs":{"events":[{"id":"..","title":".."}],"entities":[{"id":"..","title":".."}],"notes":[{"id":"..","title":".."}]},"mistakes":[{"title":"..","content":".."}]}。`
	var b strings.Builder
	fmt.Fprintf(&b, "== 本笔交易 ==\n%s %s(%s) %s %d股 @%.3f 金额%.2f\n\n",
		t.TradeDate.Format("2006-01-02"), t.Name, t.Code, sideLabel(t.Side), t.Qty, t.Price, t.Amount)
	if len(events) > 0 {
		b.WriteString("== 相关事件 ==\n")
		for _, e := range events {
			fmt.Fprintf(&b, "[E:%s] %s (类型=%s)\n", e.ID, e.Title, e.EventType)
		}
		b.WriteString("\n")
	}
	if len(entities) > 0 {
		b.WriteString("== 相关实体 ==\n")
		for _, en := range entities {
			fmt.Fprintf(&b, "[N:%s] %s (类型=%s)\n", en.ID, en.Name, en.Type)
		}
		b.WriteString("\n")
	}
	if len(notes) > 0 {
		b.WriteString("== 你的个人笔记 ==\n")
		for _, n := range notes {
			title := orStr(n.Title, n.Slug)
			fmt.Fprintf(&b, "[P:%s] [%s] %s\n", n.ID, noteTypeLabel[n.Type], title)
			if n.Content != nil {
				fmt.Fprintf(&b, "  内容: %s\n", *n.Content)
			}
		}
	}
	if len(events) == 0 && len(entities) == 0 && len(notes) == 0 {
		b.WriteString("(知识库无相关条目)")
	}
	return system, b.String()
}

// validateTradeReview 校验 refs 防自造 + 组最终入库结构。
func validateTradeReview(data json.RawMessage, events []model.Event, entities []model.Entity, notes []model.PersonalNote, modelName string, tokens int64) (tradeReview, error) {
	var rv tradeReview
	if err := json.Unmarshal(data, &rv); err != nil {
		return rv, err
	}
	rv.Review = strings.TrimSpace(rv.Review)
	if rv.Review == "" {
		return rv, fmt.Errorf("empty review")
	}
	// 白名单:id 必须真实存在于检索结果。
	evSet := map[string]string{}
	for _, e := range events {
		evSet[e.ID] = e.Title
	}
	enSet := map[string]string{}
	for _, en := range entities {
		enSet[en.ID] = en.Name
	}
	noteSet := map[string]string{}
	for _, n := range notes {
		noteSet[n.ID] = orStr(n.Title, n.Slug)
	}
	filter := func(refs []store.ChatRef, set map[string]string) []store.ChatRef {
		out := refs[:0]
		for _, r := range refs {
			if title, ok := set[r.ID]; ok {
				out = append(out, store.ChatRef{ID: r.ID, Title: title})
			}
		}
		return out
	}
	rv.Refs.Events = filter(rv.Refs.Events, evSet)
	rv.Refs.Entities = filter(rv.Refs.Entities, enSet)
	rv.Refs.Notes = filter(rv.Refs.Notes, noteSet)
	for i := range rv.Mistakes {
		rv.Mistakes[i].Title = strings.TrimSpace(rv.Mistakes[i].Title)
		rv.Mistakes[i].Content = strings.TrimSpace(rv.Mistakes[i].Content)
	}
	rv.Model = modelName
	rv.Tokens = tokens
	rv.GenAt = time.Now().In(cst).Format("2006-01-02 15:04")
	return rv, nil
}
