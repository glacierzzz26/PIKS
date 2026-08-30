package web

// /api/v1 写接口 —— React 交互页(笔记/周报/交易/设置/对话)的数据入口。
// 复用 store 查询与既有业务函数(aggregateWeek/buildNote/answerChat/...);
// 校验失败返回 {error} 4xx,成功返回 JSON。AI 调用失败如实返回,不编造。

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

// apiErrJSON 写 JSON 错误响应。
func apiErrJSON(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ==================== 笔记 ====================

// validateNoteInput 笔记输入校验(类型/标题/内容)。
func validateNoteInput(in noteInput) string {
	if _, ok := noteTypeLabel[in.Type]; !ok {
		return "类型不合法: " + in.Type
	}
	if in.Title == "" {
		return "标题必填"
	}
	if in.Content == "" {
		return "内容必填"
	}
	return ""
}

// POST /api/v1/notes —— 新建笔记。
func (s *Server) createNoteAPI(w http.ResponseWriter, r *http.Request) {
	var in noteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if errMsg := validateNoteInput(in); errMsg != "" {
		apiErrJSON(w, http.StatusBadRequest, errMsg)
		return
	}
	n, refs, err := buildNote(in)
	if err != nil {
		apiErrJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.store.CreatePersonalNote(r.Context(), &n)
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "创建失败: "+err.Error())
		return
	}
	if err := s.store.ReplaceNoteRefs(r.Context(), id, refs); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "关联失败: "+err.Error())
		return
	}
	s.writeJSON(w, map[string]string{"id": id})
}

// PUT /api/v1/notes/{id} —— 更新笔记。
func (s *Server) updateNoteAPI(w http.ResponseWriter, r *http.Request, id string) {
	var in noteInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if errMsg := validateNoteInput(in); errMsg != "" {
		apiErrJSON(w, http.StatusBadRequest, errMsg)
		return
	}
	n, refs, err := buildNote(in)
	if err != nil {
		apiErrJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	n.ID = id
	n.UpdatedBy = strPtr("me")
	if err := s.store.UpdatePersonalNote(r.Context(), &n); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "保存失败: "+err.Error())
		return
	}
	if err := s.store.ReplaceNoteRefs(r.Context(), id, refs); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "关联失败: "+err.Error())
		return
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// DELETE /api/v1/notes/{id} —— 归档笔记。
func (s *Server) archiveNoteAPI(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.store.ArchivePersonalNote(r.Context(), id); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "归档失败: "+err.Error())
		return
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// ==================== 周报 ====================

// weeklySummaryJSON AI 综述输出视图(store.WeeklySummary 只带 db tag,这里映射成 JSON)。
type weeklySummaryJSON struct {
	ID        string `json:"id"`
	Week      string `json:"week"`
	Summary   string `json:"summary"`
	Model     string `json:"model"`
	Tokens    int64  `json:"tokens"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// GET /api/v1/weekly/detail?offset= —— 当周聚合数据 + AI 综述缓存。
func (s *Server) weeklyDetailAPI(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, _ = strconv.Atoi(v)
	}
	ctx := r.Context()
	start, end, week, rng := weekRange(time.Now().In(cst), offset)
	snaps, events, notes, trades, poss, err := s.aggregateWeek(ctx, start, end)
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	sum, err := s.store.GetWeeklySummary(ctx, week)
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	note := ""
	var sumView *weeklySummaryJSON
	if sum == nil {
		note = "本周暂无 AI 综述。"
	} else {
		sumView = &weeklySummaryJSON{
			ID: sum.ID, Week: sum.Week, Summary: sum.Summary, Model: sum.Model, Tokens: sum.Tokens,
			CreatedAt: fmtRFC3339(sum.CreatedAt), UpdatedAt: fmtRFC3339(sum.UpdatedAt),
		}
	}
	s.writeJSON(w, map[string]any{
		"week":         week,
		"range":        rng,
		"offset":       offset,
		"snaps":        snaps,
		"events":       events,
		"notes":        notes,
		"trades":       trades,
		"positions":    poss,
		"summary":      sumView,
		"summary_note": note,
	})
}

// POST /api/v1/weekly/generate?offset= —— 生成当周 AI 综述(手动显式触发)。
func (s *Server) weeklyGenerateAPI(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, _ = strconv.Atoi(v)
	}
	ctx := r.Context()
	start, end, week, _ := weekRange(time.Now().In(cst), offset)
	st := s.generateWeeklySummary(ctx, week, start, end)
	s.writeJSON(w, map[string]string{"status": string(st), "week": week})
}

// ==================== 交易 ====================

// POST /api/v1/trades —— 手动录入一笔交易。
func (s *Server) tradeAddAPI(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Name      string  `json:"name"`
		Code      string  `json:"code"`
		Side      string  `json:"side"`
		Price     float64 `json:"price"`
		Qty       int     `json:"qty"`
		TradeDate string  `json:"trade_date"`
		Note      string  `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	name := strings.TrimSpace(p.Name)
	code := strings.TrimSpace(p.Code)
	if name == "" || code == "" || (p.Side != "buy" && p.Side != "sell") {
		apiErrJSON(w, http.StatusBadRequest, "请填写证券名称、代码与买卖方向。")
		return
	}
	if p.Qty <= 0 || p.Price < 0 {
		apiErrJSON(w, http.StatusBadRequest, "价格/数量格式不正确。")
		return
	}
	var date time.Time
	if p.TradeDate != "" {
		parsed, perr := time.ParseInLocation("2006-01-02", p.TradeDate, cst)
		if perr != nil {
			apiErrJSON(w, http.StatusBadRequest, "交易日期格式不正确(应为 YYYY-MM-DD)。")
			return
		}
		date = parsed
	} else {
		date = time.Now().In(cst)
	}
	var note *string
	if n := strings.TrimSpace(p.Note); n != "" {
		note = &n
	}
	if err := s.store.InsertTrades(r.Context(), []model.Trade{{
		TradeDate: date, Code: code, Name: name, Side: p.Side,
		Price: p.Price, Qty: p.Qty, Amount: p.Price * float64(p.Qty), Source: "manual", Note: note,
	}}); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "入库失败: "+err.Error())
		return
	}
	if _, err := s.store.EnsureCompanyEntity(r.Context(), code, name); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "交易已入库,但实体补全失败: "+err.Error())
		return
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// 截图导入预览行(确认前不落库,带可编辑字段)。
type apiPreviewTrade struct {
	Include bool   `json:"include"`
	Exists  bool   `json:"exists"`
	Date    string `json:"date"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Side    string `json:"side"`
	Price   string `json:"price"`
	Qty     string `json:"qty"`
	Amount  string `json:"amount"`
}

type apiPreviewPosition struct {
	Include     bool   `json:"include"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Qty         string `json:"qty"`
	CostPrice   string `json:"cost_price"`
	Price       string `json:"price"`
	MarketValue string `json:"market_value"`
	PL          string `json:"pl"`
}

type apiImportPreview struct {
	Kind         string               `json:"kind"`
	AttachmentID string               `json:"attachment_id"`
	Trades       []apiPreviewTrade    `json:"trades"`
	Positions    []apiPreviewPosition `json:"positions"`
}

func toAPIImportPreview(p *ImportPreview) apiImportPreview {
	out := apiImportPreview{Kind: p.Kind, AttachmentID: p.AttachmentID, Trades: []apiPreviewTrade{}, Positions: []apiPreviewPosition{}}
	for _, t := range p.Trades {
		out.Trades = append(out.Trades, apiPreviewTrade{
			Include: t.Include, Exists: t.Exists, Date: t.Date, Code: t.Code,
			Name: t.Name, Side: t.Side, Price: t.Price, Qty: t.Qty, Amount: t.Amount,
		})
	}
	for _, pv := range p.Positions {
		out.Positions = append(out.Positions, apiPreviewPosition{
			Include: pv.Include, Code: pv.Code, Name: pv.Name, Qty: pv.Qty,
			CostPrice: pv.CostPrice, Price: pv.Price, MarketValue: pv.MarketValue, PL: pv.PL,
		})
	}
	return out
}

// POST /api/v1/trades/import —— 截图导入:视觉抽取 → 预览(不落库)。
func (s *Server) tradeImportAPI(w http.ResponseWriter, r *http.Request) {
	kind := r.FormValue("type")
	if kind != "trade" && kind != "position" {
		apiErrJSON(w, http.StatusBadRequest, "请选择截图类型(今日交易 / 持仓)。")
		return
	}
	if err := r.ParseMultipartForm(tradeMaxUpload + 1<<20); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "表单解析失败(文件超 5MB?): "+err.Error())
		return
	}
	file, fh, err := r.FormFile("file")
	if err != nil {
		apiErrJSON(w, http.StatusBadRequest, "未收到截图文件。")
		return
	}
	defer file.Close()
	if !allowedImageType(fh.Header.Get("Content-Type")) {
		apiErrJSON(w, http.StatusBadRequest, "仅支持图片(png/jpeg/webp/gif)。")
		return
	}
	data, rerr := io.ReadAll(io.LimitReader(file, tradeMaxUpload+1))
	if rerr != nil {
		apiErrJSON(w, http.StatusBadRequest, "读文件失败: "+rerr.Error())
		return
	}
	if len(data) > tradeMaxUpload {
		apiErrJSON(w, http.StatusBadRequest, "图片超过 5MB 上限。")
		return
	}

	ctx := r.Context()
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "读 AI 配置失败: "+err.Error())
		return
	}
	base, key := cfgMap["ai_service_base_url"], cfgMap["ai_api_key"]
	vision := cfgMap["ai_model_vision"]
	if base == "" || key == "" || vision == "" {
		apiErrJSON(w, http.StatusBadRequest, "AI 未配置或视觉模型未配置(请到设置页配置 `ai_model_vision`),可用手动录入兜底。")
		return
	}
	if budget, _ := strconv.ParseInt(cfgMap["ai_daily_token_budget"], 10, 64); budget > 0 {
		if today, err := s.store.TokensSince(ctx, time.Now().Truncate(24*time.Hour)); err == nil && today >= budget {
			apiErrJSON(w, http.StatusTooManyRequests, "今日 AI 预算已用尽,导入暂缓(预算恢复后重试)。")
			return
		}
	}

	attID, err := s.saveUpload(ctx, fh.Filename, fh.Header.Get("Content-Type"), data)
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "保存截图失败: "+err.Error())
		return
	}
	runID, err := s.store.StartTaskRun(ctx, "trade-import")
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "记账失败: "+err.Error())
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
		apiErrJSON(w, http.StatusBadGateway, "截图识别失败: "+err.Error())
		return
	}
	_ = s.store.FinishTaskRun(ctx, runID, "success", "", map[string]any{"kind": kind, "model": vision, "ai_tokens": resp.Usage.Total()})

	preview, perr := buildImportPreview(ctx, s.store, kind, attID, resp.Data)
	if perr != nil {
		apiErrJSON(w, http.StatusInternalServerError, "识别结果解析失败: "+perr.Error())
		return
	}
	if preview == nil || (len(preview.Trades) == 0 && len(preview.Positions) == 0) {
		apiErrJSON(w, http.StatusUnprocessableEntity, "未识别到交易/持仓,请检查截图是否为同花顺今日交易/持仓页,或手动录入。")
		return
	}
	s.writeJSON(w, toAPIImportPreview(preview))
}

// POST /api/v1/trades/confirm —— 确认预览:批量入库 + 实体补全。
func (s *Server) tradeConfirmAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var p apiImportPreview
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if p.Kind == "position" {
		var ps []model.Position
		for _, row := range p.Positions {
			if !row.Include || strings.TrimSpace(row.Name) == "" {
				continue
			}
			code := strings.TrimSpace(row.Code)
			if code == "" {
				code = row.Name
			}
			qty, _ := strconv.Atoi(row.Qty)
			if qty <= 0 {
				continue
			}
			ps = append(ps, model.Position{
				SnapshotDate: time.Now().In(cst), Code: code, Name: row.Name, Qty: qty,
				CostPrice:   parseF(row.CostPrice),
				Price:       parseF(row.Price),
				MarketValue: parseF(row.MarketValue),
				PL:          parseF(row.PL),
				Source:      "screenshot", AttachmentID: optStr(p.AttachmentID),
			})
			if _, err := s.store.EnsureCompanyEntity(ctx, code, row.Name); err != nil {
				apiErrJSON(w, http.StatusInternalServerError, "实体补全失败: "+err.Error())
				return
			}
		}
		if len(ps) == 0 {
			apiErrJSON(w, http.StatusBadRequest, "没有勾选任何持仓行。")
			return
		}
		if err := s.store.InsertPositions(ctx, ps); err != nil {
			apiErrJSON(w, http.StatusInternalServerError, "持仓入库失败: "+err.Error())
			return
		}
		s.writeJSON(w, map[string]bool{"ok": true})
		return
	}

	var ts []model.Trade
	for _, row := range p.Trades {
		if !row.Include || strings.TrimSpace(row.Name) == "" {
			continue
		}
		code := strings.TrimSpace(row.Code)
		if code == "" {
			code = row.Name
		}
		if row.Side != "buy" && row.Side != "sell" {
			continue
		}
		price := parseF(row.Price)
		qty, _ := strconv.Atoi(row.Qty)
		if price == nil || *price < 0 || qty <= 0 {
			continue
		}
		date, err := time.ParseInLocation("2006-01-02", row.Date, cst)
		if err != nil {
			date = time.Now().In(cst)
		}
		ts = append(ts, model.Trade{
			TradeDate: date, Code: code, Name: row.Name, Side: row.Side,
			Price: *price, Qty: qty, Amount: *price * float64(qty),
			Source: "screenshot", AttachmentID: optStr(p.AttachmentID),
		})
		if _, err := s.store.EnsureCompanyEntity(ctx, code, row.Name); err != nil {
			apiErrJSON(w, http.StatusInternalServerError, "实体补全失败: "+err.Error())
			return
		}
	}
	if len(ts) == 0 {
		apiErrJSON(w, http.StatusBadRequest, "没有勾选任何交易行。")
		return
	}
	if err := s.store.InsertTrades(ctx, ts); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "交易入库失败: "+err.Error())
		return
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// ---- 交易/持仓 AI 复盘(核心逻辑抽在 trades.go,这里仅转 JSON) ----

// POST /api/v1/trades/{id}/review —— 单笔交易 AI 复盘。
func (s *Server) tradeReviewAPI(w http.ResponseWriter, r *http.Request, id string) {
	msg := s.tradeReviewCore(r.Context(), id)
	if msg != "" {
		apiErrJSON(w, http.StatusBadGateway, msg)
		return
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// POST /api/v1/trades/{id}/save-mistake/{n} —— 复盘点存为个人笔记。
func (s *Server) tradeSaveMistakeAPI(w http.ResponseWriter, r *http.Request, id string, n int) {
	ok, msg := s.tradeSaveMistakeCore(r.Context(), id, n)
	if !ok {
		apiErrJSON(w, http.StatusBadRequest, msg)
		return
	}
	s.writeJSON(w, map[string]string{"message": msg})
}

// POST /api/v1/trades/positions/review —— 持仓组合 AI 诊断。
func (s *Server) positionDiagnoseAPI(w http.ResponseWriter, r *http.Request) {
	msg := s.positionDiagnoseCore(r.Context())
	if msg != "" {
		apiErrJSON(w, http.StatusBadGateway, msg)
		return
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// POST /api/v1/trades/positions/save-risk/{n} —— 诊断风险候选存为笔记。
func (s *Server) positionSaveRiskAPI(w http.ResponseWriter, r *http.Request, n int) {
	ok, msg := s.positionSaveRiskCore(r.Context(), n)
	if !ok {
		apiErrJSON(w, http.StatusBadRequest, msg)
		return
	}
	s.writeJSON(w, map[string]string{"message": msg})
}

// /api/v1/trades/* 子路径分发(import/confirm/review/save-mistake/positions)。
func (s *Server) handleAPITradesSub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/trades/")
	switch {
	case rest == "import":
		s.tradeImportAPI(w, r)
	case rest == "confirm":
		s.tradeConfirmAPI(w, r)
	case rest == "positions/review":
		s.positionDiagnoseAPI(w, r)
	case strings.HasPrefix(rest, "positions/save-risk/"):
		n, _ := strconv.Atoi(strings.TrimPrefix(rest, "positions/save-risk/"))
		s.positionSaveRiskAPI(w, r, n)
	default:
		if strings.HasSuffix(rest, "/review") {
			s.tradeReviewAPI(w, r, strings.TrimSuffix(rest, "/review"))
			return
		}
		if idx := strings.LastIndex(rest, "/save-mistake/"); idx > 0 {
			s.tradeSaveMistakeAPI(w, r, rest[:idx], atoiOr(rest[idx+len("/save-mistake/"):], -1))
			return
		}
		apiErrJSON(w, http.StatusNotFound, "未知交易接口: "+rest)
	}
}

// ==================== 设置 ====================

// apiSettingsForm 设置编辑表单数据(密钥掩码,绝不回填明文)。
type apiSettingsForm struct {
	BaseURL        string   `json:"base_url"`
	KeyMasked      string   `json:"key_masked"`
	ModelExtract   string   `json:"model_extract"`
	ModelReasoning string   `json:"model_reasoning"`
	ModelVision    string   `json:"model_vision"`
	Budget         string   `json:"budget"`
	ModelOptions   []string `json:"model_options"`
	ModelNote      string   `json:"model_note,omitempty"`
}

// GET /api/v1/settings/form —— 设置编辑表单数据。
func (s *Server) settingsFormAPI(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.ListAppConfig(r.Context())
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "读取配置失败: "+err.Error())
		return
	}
	f := apiSettingsForm{
		BaseURL:        m["ai_service_base_url"],
		KeyMasked:      maskSecret(m["ai_api_key"]),
		ModelExtract:   m["ai_model_extract"],
		ModelReasoning: m["ai_model_reasoning"],
		ModelVision:    m["ai_model_vision"],
		Budget:         m["ai_daily_token_budget"],
	}
	if f.Budget == "" {
		f.Budget = "0"
	}
	f.ModelOptions = s.fetchModelOptions(r.Context(), m)
	f.ModelOptions = mergeOpts(f.ModelOptions, f.ModelExtract, f.ModelReasoning, f.ModelVision)
	if len(f.ModelOptions) <= len(onlyNonEmpty(f.ModelExtract, f.ModelReasoning, f.ModelVision)) {
		f.ModelNote = "模型列表获取失败(检查服务地址/密钥);下拉仅含已保存模型。"
	}
	s.writeJSON(w, f)
}

// POST /api/v1/settings —— 保存 AI 配置(密钥留空不改)。
func (s *Server) settingsSaveAPI(w http.ResponseWriter, r *http.Request) {
	var p struct {
		BaseURL        string `json:"ai_service_base_url"`
		Key            string `json:"ai_api_key"`
		ModelExtract   string `json:"ai_model_extract"`
		ModelReasoning string `json:"ai_model_reasoning"`
		ModelVision    string `json:"ai_model_vision"`
		Budget         string `json:"ai_daily_token_budget"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "解析失败: "+err.Error())
		return
	}
	ctx := r.Context()
	base := strings.TrimSpace(p.BaseURL)
	extract := strings.TrimSpace(p.ModelExtract)
	reason := strings.TrimSpace(p.ModelReasoning)
	vision := strings.TrimSpace(p.ModelVision)
	budget := strings.TrimSpace(p.Budget)
	if base == "" || extract == "" || reason == "" {
		apiErrJSON(w, http.StatusBadRequest, "AI 服务地址、文本处理模型、深度推理模型必填(不接受留空);截图模型可留空(回退文本模型)。")
		return
	}
	if budget == "" {
		budget = "0"
	}
	if _, err := strconv.ParseInt(budget, 10, 64); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "日 token 预算必须是整数(0 = 关闭护栏)。")
		return
	}
	for k, v := range map[string]string{
		"ai_service_base_url":   base,
		"ai_model_extract":      extract,
		"ai_model_reasoning":    reason,
		"ai_model_vision":       vision,
		"ai_daily_token_budget": budget,
	} {
		if err := s.store.UpsertAppConfig(ctx, k, v); err != nil {
			apiErrJSON(w, http.StatusInternalServerError, "保存失败: "+err.Error())
			return
		}
	}
	if p.Key != "" {
		if err := s.store.UpsertAppConfig(ctx, "ai_api_key", p.Key); err != nil {
			apiErrJSON(w, http.StatusInternalServerError, "保存失败: "+err.Error())
			return
		}
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// ==================== AI 对话 ====================

// apiChatReply 发送消息后的返回(assistant 消息 + 提示)。
type apiChatReply struct {
	Message apiChatMsg `json:"message"`
	Note    string     `json:"note,omitempty"`
}

// POST /api/v1/chat —— 发送问题/上传截图。multipart(question+file) 或 JSON {question}。
func (s *Server) chatPostAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ct := r.Header.Get("Content-Type")
	isJSON := strings.HasPrefix(ct, "application/json")
	isMultipart := strings.HasPrefix(ct, "multipart/form-data")

	var question string
	var file io.ReadCloser
	var fh *multipart.FileHeader
	if isJSON {
		var p struct {
			Question string `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			apiErrJSON(w, http.StatusBadRequest, "解析失败: "+err.Error())
			return
		}
		question = strings.TrimSpace(p.Question)
	} else if isMultipart {
		if err := r.ParseMultipartForm(chatMaxUpload + 1<<20); err != nil {
			apiErrJSON(w, http.StatusBadRequest, "表单解析失败(文件超 5MB?): "+err.Error())
			return
		}
		question = strings.TrimSpace(r.FormValue("question"))
		f, h, err := r.FormFile("file")
		if err != nil && err != http.ErrMissingFile {
			apiErrJSON(w, http.StatusBadRequest, "读取上传文件失败: "+err.Error())
			return
		}
		file, fh = f, h
	} else {
		apiErrJSON(w, http.StatusBadRequest, "不支持的 Content-Type: "+ct)
		return
	}
	if question == "" && fh == nil {
		apiErrJSON(w, http.StatusBadRequest, "请输入问题或上传截图。")
		return
	}

	sid, err := s.store.GetOrCreateChatSession(ctx)
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "会话创建失败: "+err.Error())
		return
	}
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "读 AI 配置失败: "+err.Error())
		return
	}
	if cfgMap["ai_service_base_url"] == "" || cfgMap["ai_api_key"] == "" {
		apiErrJSON(w, http.StatusBadRequest, "AI 未配置:请先到设置页填写服务地址与密钥。")
		return
	}

	attIDs := []string{}
	var img *ai.ImagePart
	if fh != nil {
		if !allowedImageType(fh.Header.Get("Content-Type")) {
			apiErrJSON(w, http.StatusBadRequest, "仅支持图片(png/jpeg/webp/gif)。")
			return
		}
		data, rerr := io.ReadAll(io.LimitReader(file, chatMaxUpload+1))
		if rerr != nil {
			apiErrJSON(w, http.StatusBadRequest, "读文件失败: "+rerr.Error())
			return
		}
		if len(data) > chatMaxUpload {
			apiErrJSON(w, http.StatusBadRequest, "图片超过 5MB 上限。")
			return
		}
		attID, aerr := s.saveUpload(ctx, fh.Filename, fh.Header.Get("Content-Type"), data)
		if aerr != nil {
			apiErrJSON(w, http.StatusInternalServerError, "保存截图失败: "+aerr.Error())
			return
		}
		attIDs = append(attIDs, attID)
		img = &ai.ImagePart{Data: data, MIME: fh.Header.Get("Content-Type")}
	}
	userMsg := model.ChatMessage{SessionID: sid, Role: "user", Content: question}
	if len(attIDs) > 0 {
		b, _ := json.Marshal(attIDs)
		userMsg.Attachments = b
	}
	if err := s.store.InsertChatMessage(ctx, &userMsg); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "存用户消息失败: "+err.Error())
		return
	}

	assistant, note, aerr := s.answerChat(ctx, cfgMap, question, img)
	if aerr != nil {
		// 如实降级:失败写入对话历史(用户可见)。
		assistant = &model.ChatMessage{SessionID: sid, Role: "assistant", Content: "⚠️ 调用失败:" + aerr.Error()}
	}
	assistant.SessionID = sid
	if err := s.store.InsertChatMessage(ctx, assistant); err != nil {
		apiErrJSON(w, http.StatusInternalServerError, "存回答失败: "+err.Error())
		return
	}

	msg := apiChatMsg{Role: assistant.Role, Content: assistant.Content, Time: time.Now().In(cst).Format("15:04")}
	var refs store.ChatRefs
	_ = json.Unmarshal(assistant.Refs, &refs)
	for _, e := range refs.Events {
		msg.Refs = append(msg.Refs, "事件："+e.Title)
	}
	for _, e := range refs.Entities {
		msg.Refs = append(msg.Refs, "实体："+e.Title)
	}
	out := apiChatReply{Message: msg}
	switch {
	case aerr != nil:
		out.Note = note + " AI 调用失败: " + aerr.Error()
	case note != "":
		out.Note = note
	}
	s.writeJSON(w, out)
}

// POST /api/v1/chat/clear —— 清空当前会话。
func (s *Server) chatClearAPI(w http.ResponseWriter, r *http.Request) {
	sid, err := s.store.GetOrCreateChatSession(r.Context())
	if err == nil {
		_ = s.store.ClearChatSession(r.Context(), sid)
	}
	s.writeJSON(w, map[string]bool{"ok": true})
}

// atoiOr 容错整数解析。
func atoiOr(s string, def int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

// 确保 context 被引用(避免未来删改时误删 import)。
var _ context.Context
