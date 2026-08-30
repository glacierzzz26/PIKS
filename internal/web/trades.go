package web

// 交易页(/trades):交易记录 × 持仓快照 + 截图导入(AI 视觉抽取 → 预览确认)+ 手动录入 + AI 带引用复盘。
// 设计 docs/phase2/design/trades.md。纪律:GET 永不调 LLM;导入/解读均手动显式触发;
// 截图抽取后必须预览确认才入库(防误识别污染);解读为复盘视角非建议,带引用且防自造。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

const tradeMaxUpload = 5 << 20 // 5MB

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

func sideLabel(side string) string {
	switch side {
	case "buy":
		return "买入"
	case "sell":
		return "卖出"
	}
	return side
}

// parseTradeReview 解包交易复盘 JSONB(空 → nil)。
func parseTradeReview(raw json.RawMessage) *tradeReview {
	var rv tradeReview
	if err := json.Unmarshal(raw, &rv); err != nil || rv.Review == "" {
		return nil
	}
	return &rv
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

// tradeSaveMistakeCore 把复盘候选 mistake 存为个人笔记(type=mistake, status=hypothesis)。
// iter4 单向 harvest 语义:AI 提议、用户确认才入库;已存过(同 slug)如实提示不重复建。
// 返回 (成功与否, 用户可见消息);JSON 与 HTML 两条路径共用。
func (s *Server) tradeSaveMistakeCore(ctx context.Context, id string, n int) (bool, string) {
	t, err := s.store.GetTrade(ctx, id)
	if err != nil {
		return false, "⚠️ 交易不存在: " + err.Error()
	}
	rv := parseTradeReview(t.Review)
	if rv == nil || n < 0 || n >= len(rv.Mistakes) {
		return false, "⚠️ 复盘点不存在(复盘可能已更新,请重新解读)。"
	}
	m := rv.Mistakes[n]
	title := m.Title
	if title == "" {
		title = "交易复盘候选-" + id[:8]
	}
	slug := fmt.Sprintf("trade-%s-%d", id, n)
	if existing, err := s.store.GetPersonalNoteBySlug(ctx, "mistake", slug); err == nil && existing != nil {
		return true, "✅ 该复盘点已存为笔记,未重复创建。"
	} else if err != nil {
		return false, "⚠️ 查重失败: " + err.Error()
	}
	if _, err := s.store.CreatePersonalNote(ctx, &model.PersonalNote{
		Type: "mistake", Slug: slug, Title: &title,
		Status: "hypothesis", Content: &m.Content,
	}); err != nil {
		return false, "⚠️ 存为笔记失败: " + err.Error()
	}
	return true, "✅ 已存为个人笔记。"
}

// tradeReviewCore 交易 AI 复盘并入库(带引用,手动触发)。
// 返回用户可见错误文案;空串 = 成功。JSON 与 HTML 两条路径共用。
func (s *Server) tradeReviewCore(ctx context.Context, id string) string {
	t, err := s.store.GetTrade(ctx, id)
	if err != nil {
		return "⚠️ 交易不存在: " + err.Error()
	}
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		return "⚠️ 读 AI 配置失败: " + err.Error()
	}
	base, key := cfgMap["ai_service_base_url"], cfgMap["ai_api_key"]
	model := cfgMap["ai_model_reasoning"]
	if model == "" {
		model = cfgMap["ai_model_extract"]
	}
	if base == "" || key == "" || model == "" {
		return "⚠️ AI 未配置,复盘暂缺(请到设置页配置)。"
	}
	if budget, _ := strconv.ParseInt(cfgMap["ai_daily_token_budget"], 10, 64); budget > 0 {
		if today, err := s.store.TokensSince(ctx, time.Now().Truncate(24*time.Hour)); err == nil && today >= budget {
			return "⚠️ 今日 AI 预算已用尽,复盘暂缺(预算恢复后重试)。"
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
		return "⚠️ 检索知识库失败: " + err.Error()
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
		return "⚠️ 检索个人笔记失败: " + err.Error()
	}

	runID, err := s.store.StartTaskRun(ctx, "trade-review")
	if err != nil {
		return "⚠️ 记账失败: " + err.Error()
	}
	system, user := reviewPrompt(t, events, entities, notes)
	c := ai.NewOpenAICompat(base, key, model)
	resp, err := c.StructuredOutput(ctx, ai.StructuredRequest{System: system, User: user})
	if err != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"trade_id": id})
		return "⚠️ 复盘生成失败: " + err.Error()
	}
	rv, verr := validateTradeReview(resp.Data, events, entities, notes, model, resp.Usage.Total())
	if verr != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", verr.Error(), map[string]any{"trade_id": id})
		return "⚠️ 复盘解析失败: " + verr.Error()
	}
	_ = s.store.FinishTaskRun(ctx, runID, "success", "", map[string]any{"trade_id": id, "model": model, "ai_tokens": resp.Usage.Total()})
	raw, _ := json.Marshal(rv)
	if err := s.store.SetTradeReview(ctx, id, raw); err != nil {
		return "⚠️ 复盘入库失败: " + err.Error()
	}
	return ""
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
	filterReviewRefs(&rv.Refs, events, entities, notes)
	for i := range rv.Mistakes {
		rv.Mistakes[i].Title = strings.TrimSpace(rv.Mistakes[i].Title)
		rv.Mistakes[i].Content = strings.TrimSpace(rv.Mistakes[i].Content)
	}
	rv.Model = modelName
	rv.Tokens = tokens
	rv.GenAt = time.Now().In(cst).Format("2006-01-02 15:04")
	return rv, nil
}

// filterReviewRefs 白名单过滤引用:id 必须真实存在于检索结果(防 LLM 自造),并补上真实标题。
func filterReviewRefs(refs *tradeRefs, events []model.Event, entities []model.Entity, notes []model.PersonalNote) {
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
	filter := func(src []store.ChatRef, set map[string]string) []store.ChatRef {
		out := src[:0]
		for _, r := range src {
			if title, ok := set[r.ID]; ok {
				out = append(out, store.ChatRef{ID: r.ID, Title: title})
			}
		}
		return out
	}
	refs.Events = filter(refs.Events, evSet)
	refs.Entities = filter(refs.Entities, enSet)
	refs.Notes = filter(refs.Notes, noteSet)
}

// ==== 持仓 AI 诊断(交易闭环,design trade-loop.md)====

// positionReview 持仓诊断 JSONB 结构(risks 语义 = AI 提议的可复盘点候选,用户确认才入库)。
type positionReview struct {
	Review string         `json:"review"`
	Refs   tradeRefs      `json:"refs"`
	Risks  []tradeMistake `json:"risks"`
	Model  string         `json:"model"`
	Tokens int64          `json:"tokens"`
	GenAt  string         `json:"generated_at"`
}

func parsePositionReview(raw json.RawMessage) *positionReview {
	var rv positionReview
	if err := json.Unmarshal(raw, &rv); err != nil || strings.TrimSpace(rv.Review) == "" {
		return nil
	}
	return &rv
}

// validatePositionReview 解析 + 白名单过滤(与 tradeReview 同逻辑,仅字段名 risks)。
func validatePositionReview(data json.RawMessage, events []model.Event, entities []model.Entity, notes []model.PersonalNote, modelName string, tokens int64) (positionReview, error) {
	var rv positionReview
	if err := json.Unmarshal(data, &rv); err != nil {
		return rv, err
	}
	rv.Review = strings.TrimSpace(rv.Review)
	if rv.Review == "" {
		return rv, fmt.Errorf("empty review")
	}
	filterReviewRefs(&rv.Refs, events, entities, notes)
	for i := range rv.Risks {
		rv.Risks[i].Title = strings.TrimSpace(rv.Risks[i].Title)
		rv.Risks[i].Content = strings.TrimSpace(rv.Risks[i].Content)
	}
	rv.Model = modelName
	rv.Tokens = tokens
	rv.GenAt = time.Now().In(cst).Format("2006-01-02 15:04")
	return rv, nil
}

// PositionAgg 持仓聚合(Go 计算的数字事实,LLM 只解读不编造数字)。
type PositionAgg struct {
	SnapshotDate string
	TotalMV      float64 // 总市值
	TotalPL      float64 // 总盈亏
	PlPct        float64 // 总盈亏 / 总市值 ×100
	ProfitN      int     // 盈利只数
	LossN        int     // 亏损只数
	Top1Pct      float64 // 最大持仓市值占比 %
	Top3Pct      float64 // 前3大持仓市值占比 %
	Rows         []PositionAggRow
}

type PositionAggRow struct {
	Code, Name   string
	Qty          int
	Cost, Price  float64
	MV, PL       float64 // 可用则算,缺数据为 0
	HasMV        bool
	MVShare      float64 // 市值占比 %
	PlPct        float64 // 单只盈亏率 %
	RecentTrades []string // 近14天交易描述,如 "08-28 买入 100股"
}

// aggPositions 组合聚合:归一化每只持仓的市值/盈亏,算组合整体与集中度,挂近14天交易联动。
func aggPositions(ps []model.Position, recent []model.Trade) PositionAgg {
	agg := PositionAgg{}
	if len(ps) == 0 {
		return agg
	}
	agg.SnapshotDate = ps[0].SnapshotDate.Format("2006-01-02")
	// code → 近14天交易
	trMap := map[string][]string{}
	for _, t := range recent {
		if t.Source == "screenshot" || t.Source == "manual" { // 两种来源都算
			trMap[t.Code] = append(trMap[t.Code], fmt.Sprintf("%s %s %d股",
				t.TradeDate.Format("01-02"), sideLabel(t.Side), t.Qty))
		}
	}
	for _, p := range ps {
		row := PositionAggRow{Code: p.Code, Name: p.Name, Qty: p.Qty, RecentTrades: trMap[p.Code]}
		if p.CostPrice != nil {
			row.Cost = *p.CostPrice
		}
		if p.Price != nil {
			row.Price = *p.Price
		}
		if p.MarketValue != nil {
			row.MV = *p.MarketValue
			row.HasMV = true
		} else if row.Price > 0 {
			row.MV = row.Price * float64(p.Qty)
			row.HasMV = true
		}
		if p.PL != nil {
			row.PL = *p.PL
		} else if row.HasMV && row.Cost > 0 {
			row.PL = row.MV - row.Cost*float64(p.Qty)
		}
		if row.Cost > 0 && row.Price > 0 {
			row.PlPct = (row.Price - row.Cost) / row.Cost * 100
		}
		agg.Rows = append(agg.Rows, row)
	}
	for i := range agg.Rows {
		agg.TotalMV += agg.Rows[i].MV
		agg.TotalPL += agg.Rows[i].PL
		if agg.Rows[i].PL > 0 {
			agg.ProfitN++
		} else if agg.Rows[i].PL < 0 {
			agg.LossN++
		}
	}
	if agg.TotalMV > 0 {
		agg.PlPct = agg.TotalPL / agg.TotalMV * 100
	}
	// 集中度:按市值降序,占比算在 sorted 行上(避免把 0 值 share 抄回)
	sorted := make([]PositionAggRow, len(agg.Rows))
	copy(sorted, agg.Rows)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].MV != sorted[j].MV {
			return sorted[i].MV > sorted[j].MV
		}
		return sorted[i].Name < sorted[j].Name
	})
	shareByCode := map[string]float64{}
	for i, r := range sorted {
		sh := 0.0
		if agg.TotalMV > 0 {
			sh = r.MV / agg.TotalMV * 100
		}
		shareByCode[r.Code] = sh
		if i == 0 {
			agg.Top1Pct = sh
		}
		if i < 3 {
			agg.Top3Pct += sh
		}
	}
	for i := range agg.Rows {
		agg.Rows[i].MVShare = shareByCode[agg.Rows[i].Code]
	}
	return agg
}

// positionReviewPrompt 组装持仓诊断 system/user(数字全部来自聚合,LLM 禁止编造)。
func positionReviewPrompt(agg PositionAgg, events []model.Event, entities []model.Entity, notes []model.PersonalNote) (system, user string) {
	system = `你是 PIKS 个人 A 股投资知识系统的持仓诊断助手。下方是本组合持仓(Go 已算好的数字)+ 近14天交易联动 + 知识库检索结果。
规则:
- 用中文输出一段 ≤300 字的诊断:组合整体盈亏结构、仓位集中度、近期交易与持仓的一致性/矛盾(是否追高/摊薄/高抛)、值得复盘的风险点;
- 复盘视角,禁止输出买卖建议或预测涨跌;
- 严格只基于下方数据:数字一律以「聚合数据」为准,禁止新增或改写数字;知识库无相关事件/实体/笔记时如实说明「知识库无该组合相关事件/实体/笔记」,不要编造;
- refs 的 id 必须来自下方方括号标注(E:事件 id / N:实体 id / P:笔记 id),没有关联就留空数组,不要自造;
- 正文每提到一个知识库事件/实体/笔记,就必须在对应 refs 数组里给出其真实 id 与标题;正文不得提及未列入 refs 的条目;
- 若发现组合风险点,在 risks 提议(标题+内容),否则留空;
- 仅输出 JSON,结构: {"review":"...","refs":{"events":[{"id":"..","title":".."}],"entities":[{"id":"..","title":".."}],"notes":[{"id":"..","title":".."}]},"risks":[{"title":"..","content":".."}]}。`
	var b strings.Builder
	fmt.Fprintf(&b, "== 持仓聚合(快照日 %s)==\n", agg.SnapshotDate)
	fmt.Fprintf(&b, "总市值 %.0f 总盈亏 %+.0f 盈亏率 %.2f%% 盈利%d只/亏损%d只 最大持仓占比 %.1f%% 前3大占比 %.1f%%\n\n",
		agg.TotalMV, agg.TotalPL, agg.PlPct, agg.ProfitN, agg.LossN, agg.Top1Pct, agg.Top3Pct)
	for _, row := range agg.Rows {
		fmt.Fprintf(&b, "%s(%s) %d股 成本%.3f 现价%.3f 市值%.0f 盈亏%+.0f 盈亏率%+.2f%% 占比%.1f%%",
			row.Name, row.Code, row.Qty, row.Cost, row.Price, row.MV, row.PL, row.PlPct, row.MVShare)
		if len(row.RecentTrades) > 0 {
			fmt.Fprintf(&b, " 近14天交易:%s", strings.Join(row.RecentTrades, ", "))
		}
		b.WriteString("\n")
	}
	if len(events) > 0 {
		b.WriteString("\n== 相关事件 ==\n")
		for _, e := range events {
			fmt.Fprintf(&b, "[E:%s] %s (类型=%s)\n", e.ID, e.Title, e.EventType)
		}
	}
	if len(entities) > 0 {
		b.WriteString("\n== 相关实体 ==\n")
		for _, en := range entities {
			fmt.Fprintf(&b, "[N:%s] %s (类型=%s)\n", en.ID, en.Name, en.Type)
		}
	}
	if len(notes) > 0 {
		b.WriteString("\n== 相关笔记 ==\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "[P:%s] [%s] %s\n", n.ID, noteTypeLabel[n.Type], orStr(n.Title, n.Slug))
		}
	}
	return system, b.String()
}

// positionDiagnoseCore 生成最近快照的持仓诊断并缓存(POST /trades/positions/review)。
// 与 tradeReview 同纪律:预算护栏 + 防未来函数 + 引用白名单 + task_runs 记账;GET 不触发。
// 返回用户可见错误文案;空串 = 成功。JSON 与 HTML 两条路径共用。
func (s *Server) positionDiagnoseCore(ctx context.Context) string {
	ps, err := s.store.LatestPositions(ctx)
	if err != nil {
		return "⚠️ 读持仓失败: " + err.Error()
	}
	if len(ps) == 0 {
		return "⚠️ 暂无持仓快照,先导入持仓再诊断。"
	}
	snapshot := ps[0].SnapshotDate
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		return "⚠️ 读 AI 配置失败: " + err.Error()
	}
	base, key := cfgMap["ai_service_base_url"], cfgMap["ai_api_key"]
	mname := cfgMap["ai_model_reasoning"]
	if mname == "" {
		mname = cfgMap["ai_model_extract"]
	}
	if base == "" || key == "" || mname == "" {
		return "⚠️ AI 未配置,诊断暂缺(请到设置页配置)。"
	}
	if budget, _ := strconv.ParseInt(cfgMap["ai_daily_token_budget"], 10, 64); budget > 0 {
		if today, err := s.store.TokensSince(ctx, time.Now().Truncate(24*time.Hour)); err == nil && today >= budget {
			return "⚠️ 今日 AI 预算已用尽,诊断暂缺(预算恢复后重试)。"
		}
	}

	// 防未来函数:诊断只含快照时点及之前语境。
	cutoff := snapshot.AddDate(0, 0, 1)
	recent, err := s.store.ListTradesBetween(ctx, snapshot.AddDate(0, 0, -14), cutoff)
	if err != nil {
		return "⚠️ 读交易联动失败: " + err.Error()
	}
	agg := aggPositions(ps, recent)

	// KB grounding:持仓股名/代码 → 同义扩展 + 检索(事件/实体)+ 每只持仓笔记(去重)。
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	q := strings.Join(names, " ")
	extra, _ := s.expandQuery(ctx, cfgMap, q)
	events, entities, err := s.store.SearchKnowledgeExpanded(ctx, q, extra, 10, 10)
	if err != nil {
		return "⚠️ 检索知识库失败: " + err.Error()
	}
	kept := events[:0]
	for _, e := range events {
		if evWithin(e, cutoff) {
			kept = append(kept, e)
		}
	}
	events = kept
	var notes []model.PersonalNote
	seen := map[string]bool{}
	for _, p := range ps {
		ns, err := s.store.ListPersonalNotesByText(ctx, p.Name, 3)
		if err != nil {
			continue
		}
		for _, n := range ns {
			if !seen[n.ID] {
				seen[n.ID] = true
				notes = append(notes, n)
			}
		}
		if len(notes) >= 10 {
			break
		}
	}

	runID, err := s.store.StartTaskRun(ctx, "position-review")
	if err != nil {
		return "⚠️ 记账失败: " + err.Error()
	}
	system, user := positionReviewPrompt(agg, events, entities, notes)
	c := ai.NewOpenAICompat(base, key, mname)
	resp, err := c.StructuredOutput(ctx, ai.StructuredRequest{System: system, User: user})
	if err != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"snapshot": agg.SnapshotDate})
		return "⚠️ 诊断生成失败: " + err.Error()
	}
	rv, verr := validatePositionReview(resp.Data, events, entities, notes, mname, resp.Usage.Total())
	if verr != nil {
		_ = s.store.FinishTaskRun(ctx, runID, "failed", verr.Error(), map[string]any{"snapshot": agg.SnapshotDate})
		return "⚠️ 诊断解析失败: " + verr.Error()
	}
	_ = s.store.FinishTaskRun(ctx, runID, "success", "", map[string]any{"snapshot": agg.SnapshotDate, "model": mname, "ai_tokens": resp.Usage.Total()})
	raw, _ := json.Marshal(rv)
	if err := s.store.UpsertPositionReview(ctx, snapshot, raw, mname, resp.Usage.Total()); err != nil {
		return "⚠️ 诊断入库失败: " + err.Error()
	}
	return ""
}

// positionSaveRiskCore 把诊断候选 risk 存为个人笔记(type=mistake, status=hypothesis)。
// iter4 单向 harvest 语义:AI 提议、用户确认;已存过(同 slug)如实提示不重复建。
// 返回 (成功与否, 用户可见消息);JSON 与 HTML 两条路径共用。
func (s *Server) positionSaveRiskCore(ctx context.Context, n int) (bool, string) {
	ps, err := s.store.LatestPositions(ctx)
	if err != nil || len(ps) == 0 {
		return false, "⚠️ 暂无持仓快照。"
	}
	snapshot := ps[0].SnapshotDate
	pr, err := s.store.GetPositionReview(ctx, snapshot)
	if err != nil || pr == nil {
		return false, "⚠️ 诊断不存在(请先生成持仓诊断)。"
	}
	rv := parsePositionReview(pr.Review)
	if rv == nil || n < 0 || n >= len(rv.Risks) {
		return false, "⚠️ 风险候选不存在(诊断可能已更新,请重新诊断)。"
	}
	m := rv.Risks[n]
	title := m.Title
	if title == "" {
		title = "持仓诊断候选-" + snapshot.Format("2006-01-02")
	}
	slug := fmt.Sprintf("posrev-%s-%d", snapshot.Format("20060102"), n)
	if existing, err := s.store.GetPersonalNoteBySlug(ctx, "mistake", slug); err == nil && existing != nil {
		return true, "✅ 该风险候选已存为笔记,未重复创建。"
	} else if err != nil {
		return false, "⚠️ 查重失败: " + err.Error()
	}
	if _, err := s.store.CreatePersonalNote(ctx, &model.PersonalNote{
		Type: "mistake", Slug: slug, Title: &title,
		Status: "hypothesis", Content: &m.Content,
	}); err != nil {
		return false, "⚠️ 存为笔记失败: " + err.Error()
	}
	return true, "✅ 已存为个人笔记。"
}

