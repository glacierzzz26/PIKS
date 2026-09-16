package web

// 个股深研 API(research 并入,design research-merge.md §4.8)。
// POST 触发返回 run_id 立即响应(单次 10~60s,不阻塞 —— D-10),前端轮询 GET 取状态。

import (
	"context"
	"encoding/json"
	"fmt"
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
	Name      string          `json:"name"` // 公司名(仅展示;经 entities.detail.code → name 富化,缺则不臆测)
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
	Name    string `json:"name"` // 公司名(展示富化;缺则空,前端不臆测)
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
	s.writeJSON(w, toAPIResearchRun(row, s.researchNames(r.Context(), []store.ResearchRun{*row})[row.Code]))
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
	names := s.researchNames(r.Context(), rows)
	out := make([]apiResearchRunSummary, 0, len(rows))
	for i := range rows {
		out = append(out, toSummary(&rows[i], names[rows[i].Code]))
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
	// 主体感知(P9 / issue #10):研报主体不限于个股 —— 行业(sw+6 位申万码)亦合法。
	// 归一后必须能识别为主体:实体库页可能传来股票名称(detail.code 为名称时),
	// 名称原样进编排会造出 run_id 含名称的失败记录(issue #2),此处拦在入口。
	subjectType, code := store.NormalizeSubject(req.Code)
	if subjectType == "" {
		apiErrJSON(w, http.StatusBadRequest, fmt.Sprintf(
			"无法识别的主体(收到 %q;公司应为 6 位数字,行业应为 sw+6 位申万代码)", strings.TrimSpace(req.Code)))
		return
	}
	profile := req.Profile
	if profile == "" {
		profile = "complete-stock"
	}

	// 触发侧防重(issue #7):同 code+profile 已有「进行中」的 run → 复用它,不落新行。
	// 同秒双击本由 run_id 的秒级时间戳 + ON CONFLICT 挡住,但隔几秒的重复触发挡不住
	// (实测 600519 prebuy 15:38:46 与 15:39:39 各落一行)。刷新页面/多标签也走这里。
	// ⚠️ 只复用进行中的:done/failed 不算 —— 已完成后再点「重新分析」是刻意保留的
	// 时间序列(决策记录 based_on 边指向 research_runs.id,不能被覆盖)。
	if active, err := s.store.FindActiveResearchRun(r.Context(), code, profile); err != nil {
		s.apiErr(w, "research-trigger", err)
		return
	} else if active != nil && !s.researchRunStale(active) {
		// 陈旧判定兜底:服务重启会让状态永远卡在 gathering/synthesizing(没人再推进它),
		// 此时不该无限复用一个死 run,照常新建。
		s.writeJSON(w, map[string]any{
			"run_id": active.RunID, "status": active.Status, "reused": true,
		})
		return
	}

	// per-run 可取消:请求断开不该杀后台任务,故用独立的 Background ctx + 编排总超时兜底。
	ctx, cancel := context.WithTimeout(context.Background(), research.TimeoutTotal)

	o := research.New(s.store, s.researchProvider(), s.cfg.AIDailyTokenBudget)
	// 先占位建行(同步),前端马上拿到 run_id;失败即报,不留悬挂行。
	// code 是**规范主体码**(公司=裸 6 位,行业=sw801010),它同时是产物文件名前缀与
	// Python CLI 入参 —— 三处必须是同一串(行业裸码会被 Python 当北交所股票)。
	runID := research.NewRunID(code, profile, time.Now())
	created, err := s.store.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: code, Symbol: research.SubjectFullCode(code),
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
		// PriorRuns=2:新一期把最近两份 done 研报作合成输入(issue #8),可对比。
		if _, err := o.Run(ctx, research.Options{
			RunID: runID, Code: code, Profile: profile, Days: req.Days,
			PriorRuns: research.PriorRunLimit,
		}); err != nil {
			// 编排自身的失败已落 research_runs.error;此处只记服务端日志。
			log.Printf("research-run %s 编排失败: %v", runID, err)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	s.writeJSON(w, map[string]any{"run_id": runID, "status": research.StatusPending})
}

// researchRunStale 判断一条「进行中」的 run 是否已死(不该再被复用)。
// 编排总上限 TimeoutTotal 之后必然有定论(done/failed)——若状态仍是进行中,
// 说明推进它的进程没了(服务重启/容器重建),该 run 永远不会再变。
// 留 5 分钟余量,避免把正在收尾的 run 误判为死。
func (s *Server) researchRunStale(r *store.ResearchRun) bool {
	return time.Since(r.CreatedAt) > research.TimeoutTotal+5*time.Minute
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

// researchNames 批量取 code → 公司名(展示富化)。查库失败不阻断报告渲染:名称是锦上添花,
// 缺了退回代码即可,不该让整个请求失败(如实降级)。
func (s *Server) researchNames(ctx context.Context, rows []store.ResearchRun) map[string]string {
	codes := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].Code != "" {
			codes = append(codes, rows[i].Code)
		}
	}
	names, err := s.store.CompanyNamesByCodes(ctx, codes)
	if err != nil {
		return map[string]string{}
	}
	return names
}

func toAPIResearchRun(r *store.ResearchRun, name string) apiResearchRun {
	md := ""
	if r.Markdown != nil {
		md = *r.Markdown
	}
	return apiResearchRun{
		RunID: r.RunID, Code: r.Code, Symbol: r.Symbol, Name: name, Profile: r.Profile,
		AsOf: fmtDate(r.AsOf.In(cst)), Status: r.Status,
		Metrics: r.Metrics, Synthesis: r.Synthesis, Markdown: md,
		Lint: r.Lint, Gate: r.Gate, Evidence: r.Evidence,
		Error: orStr(r.Error, ""), Model: r.Model, Tokens: r.Tokens,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toSummary(r *store.ResearchRun, name string) apiResearchRunSummary {
	return apiResearchRunSummary{
		ID:    r.ID,
		RunID: r.RunID,
		Code:  r.Code, Symbol: r.Symbol, Name: name, Profile: r.Profile,
		AsOf: fmtDate(r.AsOf.In(cst)), Status: r.Status,
		LintOK: research.JSONPassed(r.Lint),
		GateOK: research.JSONPassed(r.Gate),
		Error:  orStr(r.Error, ""), Model: r.Model, Tokens: r.Tokens,
	}
}

// researchRunTitle 决策记录引用用的可读标题:有公司名 → 「名称(代码)」,否则退回代码(不臆测)。
func researchRunTitle(code, symbol, name, profile string) string {
	label := code
	if symbol != "" {
		label = symbol
	}
	if name != "" {
		label = name + "(" + code + ")"
	}
	if profile != "" {
		return label + " · " + profile
	}
	return label
}
