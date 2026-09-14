package web

// 个股深研 API(research 并入,design research-merge.md §4.8)。
// POST 触发返回 run_id 立即响应(单次 10~60s,不阻塞 —— D-10),前端轮询 GET 取状态。

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"piks/internal/ai"
	"piks/internal/research"
	"piks/internal/store"
)

// ==================== DTO(字段对齐 frontend/src/lib/types.ts 的 ResearchRun)====================

// apiResearchRun 单份深研报告。metrics/evidence = Fact;synthesis = Opinion;lint/gate = 机检。
type apiResearchRun struct {
	RunID     string          `json:"run_id"`
	Code      string          `json:"code"`
	Symbol    string          `json:"symbol"`
	Profile   string          `json:"profile"`
	AsOf      string          `json:"as_of"`
	Status    string          `json:"status"`
	Metrics   json.RawMessage `json:"metrics"`
	Synthesis json.RawMessage `json:"synthesis"`
	Markdown  string          `json:"markdown"`
	Lint      json.RawMessage `json:"lint"`
	Gate      json.RawMessage `json:"gate"`
	Evidence  json.RawMessage `json:"evidence"`
	Error     string          `json:"error"`
	Model     string          `json:"model"`
	Tokens    int64           `json:"tokens"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// apiResearchRunSummary 列表项(不带宽字段 markdown/metrics,列表页只要元信息 + 机检徽标)。
// ID = research_runs.id(UUID,供决策边 to_id 用);RunID = 业务键(TEXT,前端深链/轮询用)。
type apiResearchRunSummary struct {
	ID      string `json:"id"`
	RunID   string `json:"run_id"`
	Code    string `json:"code"`
	Symbol  string `json:"symbol"`
	Profile string `json:"profile"`
	AsOf    string `json:"as_of"`
	Status  string `json:"status"`
	LintOK  bool   `json:"lint_ok"`
	GateOK  bool   `json:"gate_ok"`
	Error   string `json:"error"`
	Model   string `json:"model"`
	Tokens  int64  `json:"tokens"`
}

// ==================== 路由 ====================

// GET/POST /api/v1/research-runs
//
//	GET  ?code=000560&entity=<uuid>&limit=20 → 报告列表(as_of DESC);均缺省 = 全部
//	     entity 走 §4.6 join(entities.detail->>'code' = research_runs.code)
//	POST {code, profile?, days?} → 触发深研,立即返回 {run_id,status}
func (s *Server) handleAPIResearchRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.researchRunsList(w, r)
	case http.MethodPost:
		s.researchRunTrigger(w, r)
	default:
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET/POST")
	}
}

// GET /api/v1/research-runs/:runId —— 单份报告全量。
func (s *Server) handleAPIResearchRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 GET")
		return
	}
	runID := strings.TrimPrefix(r.URL.Path, "/api/v1/research-runs/")
	if runID == "" {
		apiErrJSON(w, http.StatusBadRequest, "缺少 run_id")
		return
	}
	row, err := s.store.GetResearchRun(r.Context(), runID)
	if err != nil {
		s.apiErr(w, "research-runs", err)
		return
	}
	if row == nil {
		apiErrJSON(w, http.StatusNotFound, "报告不存在: "+runID)
		return
	}
	s.writeJSON(w, toAPIResearchRun(row))
}

// ==================== 处理 ====================

func (s *Server) researchRunsList(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	q := r.URL.Query()
	var rows []store.ResearchRun
	var err error
	// entity 优先:走 §4.6 join(code 归一对齐 detail->>'code'),实体页直接用。
	if eid := q.Get("entity"); eid != "" {
		rows, err = s.store.ListResearchRunsByEntity(r.Context(), eid)
		if limit > 0 && len(rows) > limit {
			rows = rows[:limit]
		}
	} else {
		rows, err = s.store.ListResearchRuns(r.Context(), q.Get("code"), limit)
	}
	if err != nil {
		s.apiErr(w, "research-runs", err)
		return
	}
	out := make([]apiResearchRunSummary, 0, len(rows))
	for i := range rows {
		out = append(out, toSummary(&rows[i]))
	}
	s.writeJSON(w, map[string]any{"runs": out})
}

// researchRunTrigger 触发一次深研:同步建 pending 行并返回 run_id,
// 实际编排在后台 goroutine 跑(D-10:10~60s 不能阻塞请求);前端轮询 GET 取状态。
func (s *Server) researchRunTrigger(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code    string `json:"code"`
		Profile string `json:"profile"`
		Days    int    `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	code := store.NormalizeCode(req.Code)
	if code == "" {
		apiErrJSON(w, http.StatusBadRequest, "缺少股票代码 code")
		return
	}
	profile := req.Profile
	if profile == "" {
		profile = "complete-stock"
	}

	// per-run 可取消:请求断开不该杀后台任务,故用独立的 Background ctx + 编排总超时兜底。
	ctx, cancel := context.WithTimeout(context.Background(), research.TimeoutTotal)

	o := research.New(s.store, s.researchProvider(), s.cfg.AIDailyTokenBudget)
	// 先占位建行(同步),前端马上拿到 run_id;失败即报,不留悬挂行。
	runID := research.NewRunID(code, profile, time.Now())
	created, err := s.store.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: code, Symbol: research.ToFullCode(code),
		Profile: profile, AsOf: time.Now(), Status: research.StatusPending,
	})
	if err != nil {
		cancel()
		s.apiErr(w, "research-trigger", err)
		return
	}
	if !created {
		cancel()
		// 同秒重触发 → 复用该 run(幂等),前端拿同一 run_id 轮询。
		s.writeJSON(w, map[string]any{"run_id": runID, "status": research.StatusPending})
		return
	}

	go func() {
		defer cancel()
		if _, err := o.Run(ctx, research.Options{RunID: runID, Code: code, Profile: profile, Days: req.Days}); err != nil {
			// 编排自身的失败已落 research_runs.error;此处只记服务端日志。
			log.Printf("research-run %s 编排失败: %v", runID, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	s.writeJSON(w, map[string]any{"run_id": runID, "status": research.StatusPending})
}

// researchProvider 构造 AI provider(与 settings/trades 同源:app_config)。
// 未配置返回 nil → 编排器在合成步如实失败(不降级不编造)。
// PIKS_AI_PROVIDER=mock 走 mock:与 cmd/research-run、entity-build 同一开关,便于 dev 验管道。
func (s *Server) researchProvider() ai.Provider {
	if os.Getenv("PIKS_AI_PROVIDER") == "mock" {
		return ai.NewMock()
	}
	if s.cfg.AIServiceBaseURL == "" || s.cfg.AIAPIKey == "" {
		return nil
	}
	model := s.cfg.AIModelExtract
	if model == "" {
		model = s.cfg.AIModelReasoning
	}
	if model == "" {
		return nil
	}
	return ai.NewOpenAICompat(s.cfg.AIServiceBaseURL, s.cfg.AIAPIKey, model)
}

// ==================== 转换 ====================

func toAPIResearchRun(r *store.ResearchRun) apiResearchRun {
	md := ""
	if r.Markdown != nil {
		md = *r.Markdown
	}
	return apiResearchRun{
		RunID: r.RunID, Code: r.Code, Symbol: r.Symbol, Profile: r.Profile,
		AsOf: fmtDate(r.AsOf.In(cst)), Status: r.Status,
		Metrics: r.Metrics, Synthesis: r.Synthesis, Markdown: md,
		Lint: r.Lint, Gate: r.Gate, Evidence: r.Evidence,
		Error: orStr(r.Error, ""), Model: r.Model, Tokens: r.Tokens,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toSummary(r *store.ResearchRun) apiResearchRunSummary {
	return apiResearchRunSummary{
		ID:    r.ID,
		RunID: r.RunID,
		Code:  r.Code, Symbol: r.Symbol, Profile: r.Profile,
		AsOf: fmtDate(r.AsOf.In(cst)), Status: r.Status,
		LintOK: research.JSONPassed(r.Lint),
		GateOK: research.JSONPassed(r.Gate),
		Error:  orStr(r.Error, ""), Model: r.Model, Tokens: r.Tokens,
	}
}

// runTitle 决策记录引用用的可读标题:code + profile(研报无自有 title 字段)。
func runTitle(r store.ResearchRun) string {
	code := r.Code
	if r.Symbol != "" {
		code = r.Symbol
	}
	if r.Profile != "" {
		return code + " · " + r.Profile
	}
	return code
}
