package web

// 个股深研 API(research 并入,design research-merge.md §4.8)。
// POST 触发返回 run_id 立即响应(单次 10~60s,不阻塞 —— D-10),前端轮询 GET 取状态。

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	// P9-2 研报版面:主体判别 + 展示名。公司/行业/宏观共用一套版面,差异由此驱动
	// (design report-layout.md D-R2)。前端据此选报告类型 chip 与标题,不必猜 code 形态。
	SubjectType string `json:"subject_type"`
	DisplayName string `json:"display_name"`
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
	// 同 apiResearchRun:主体判别 + 展示名(P9-2)。
	SubjectType string `json:"subject_type"`
	DisplayName string `json:"display_name"`
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

// researchRunTrigger 触发一次深研:同步建 pending 行并入队,返回 run_id;
// 实际编排由 research 容器的常驻 worker 认领执行(拆分后 web 已无 python3),前端轮询 GET 取状态。
func (s *Server) researchRunTrigger(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code    string `json:"code"`
		Profile string `json:"profile"`
		Days    int    `json:"days"`
		// Quick 快速模式:合成可选(无 AI 也 done,结论取确定性评分卡+风险规则)。
		// 供「买入前速评」使用;默认 false = 深研(必须有 AI)。
		Quick bool `json:"quick"`
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
			"无法识别的主体(收到 %q;公司应为 6 位数字,行业应为 sw+6 位申万代码,"+
				"宏观应为 macro:<维度> 如 macro:cn_cpi)", strings.TrimSpace(req.Code)))
		return
	}
	profile := req.Profile
	if profile == "" {
		profile = "complete-stock"
	}

	// 重活护栏(P12 / issue #78):限流 + 预算。触发即入队跑整条 Python 深研 + LLM 合成,
	// 此前公网可被任意人反复触发烧钱。挡在入口(建行之前)。
	if !s.llmGuard(w, r) {
		return
	}

	// 触发侧防重(issue #7):同 code+profile 已有「进行中」的 run → 复用它,不落新行。
	// 同秒双击本由 run_id 的秒级时间戳 + ON CONFLICT 挡住,但隔几秒的重复触发挡不住
	// (实测 600519 prebuy 15:38:46 与 15:39:39 各落一行)。刷新页面/多标签也走这里。
	// ⚠️ 只复用进行中的:done/failed 不算 —— 已完成后再点「重新分析」是刻意保留的
	// 时间序列(决策记录 based_on 边指向 research_runs.id,不能被覆盖)。
	//
	// ⚠️ 拆分后「进行中」含 pending 排队态:worker 未跑时 run 会停在 pending,这正是
	// 诚实语义(前端徽标即「排队中」),不应被当成死 run 而另起一行。故这里**不再做**
	// 陈旧判定 —— 孤儿 run 的收口归 worker 的 reaper(它才确实知道谁在跑),见
	// store.ReapStuckResearchRuns。原来的 researchRunStale 启发式已随之删除。
	if active, err := s.store.FindActiveResearchRun(r.Context(), code, profile); err != nil {
		s.apiErr(w, "research-trigger", err)
		return
	} else if active != nil {
		s.writeJSON(w, map[string]any{
			"run_id": active.RunID, "status": active.Status, "reused": true,
		})
		return
	}

	// 先占位建行(同步),前端马上拿到 run_id;失败即报,不留悬挂行。
	// code 是**规范主体码**(公司=裸 6 位,行业=sw801010),它同时是产物文件名前缀与
	// Python CLI 入参 —— 三处必须是同一串(行业裸码会被 Python 当北交所股票)。
	runID := research.NewRunID(code, profile, time.Now())
	created, err := s.store.CreateResearchRun(r.Context(), &store.ResearchRun{
		RunID: runID, Code: code, Symbol: research.SubjectFullCode(code),
		Profile: profile, AsOf: time.Now(), Status: research.StatusPending,
		// Quick/Days 随行落库(迁移 0015):执行方是独立 worker,它只拿得到
		// research_runs 这一行 —— 不落库则认领后无从还原运行参数。
		Quick: req.Quick, Days: req.Days,
	})
	if err != nil {
		s.apiErr(w, "research-trigger", err)
		return
	}
	if !created {
		// 同秒重触发 → 复用该 run(幂等),前端拿同一 run_id 轮询。
		s.writeJSON(w, map[string]any{"run_id": runID, "status": research.StatusPending})
		return
	}

	// 入队通知:唤醒 worker 认领。失败不致命(worker 有轮询兜底,最多晚一个周期),
	// 故只记日志、不让触发失败 —— 行已落库,pending 状态本身就是队列。
	if err := s.store.NotifyResearchPending(r.Context()); err != nil {
		log.Printf("research-trigger: NOTIFY 失败(worker 轮询兜底,run %s 仍会跑): %v", runID, err)
	}

	w.WriteHeader(http.StatusAccepted)
	s.writeJSON(w, map[string]any{"run_id": runID, "status": research.StatusPending})
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
	subjectType, displayName := subjectPresentation(r, name)
	return apiResearchRun{
		RunID: r.RunID, Code: r.Code, Symbol: r.Symbol, Name: name, Profile: r.Profile,
		AsOf: fmtDate(r.AsOf.In(cst)), Status: r.Status,
		Metrics: r.Metrics, Synthesis: r.Synthesis, Markdown: md,
		Lint: r.Lint, Gate: r.Gate, Evidence: r.Evidence,
		Error: orStr(r.Error, ""), Model: r.Model, Tokens: r.Tokens,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   r.UpdatedAt.UTC().Format(time.RFC3339),
		SubjectType: subjectType, DisplayName: displayName,
	}
}

func toSummary(r *store.ResearchRun, name string) apiResearchRunSummary {
	subjectType, displayName := subjectPresentation(r, name)
	return apiResearchRunSummary{
		ID:    r.ID,
		RunID: r.RunID,
		Code:  r.Code, Symbol: r.Symbol, Name: name, Profile: r.Profile,
		AsOf: fmtDate(r.AsOf.In(cst)), Status: r.Status,
		LintOK: research.JSONPassed(r.Lint),
		GateOK: research.JSONPassed(r.Gate),
		Error:  orStr(r.Error, ""), Model: r.Model, Tokens: r.Tokens,
		SubjectType: subjectType, DisplayName: displayName,
	}
}

// subjectPresentation 主体判别 + 展示名(P9-2 研报版面)。
//
// subject_type 由 code 形态判定(`SubjectTypeOf` 复用 #10 的归一规则),前端据此选
// 报告类型 chip 与标题,不必猜 code 形态。
//
// display_name 的取法与 name 同源、分主体:
//   - 公司:entities.detail.code → name 富化(name 参数),缺则空(不臆测 —— 前端退回代码)
//   - 行业:研报正文的 `industry_index.ref.name`(如「农林牧渔」)。**不查申万表** ——
//     那是 research 侧的知识,Go 侧复制一份就违反了 D-11 独立性;指标卡里已有此字段,直读即可。
//   - 宏观(issue #13):研报正文的 `macro.ref.name`(如「居民消费价格指数（CPI）」)。
//     同理**不建维度表** —— `macro:<key>` 的键→名映射属于 research 侧知识(D-M2)。
//
// 读 metrics 失败(旧产物/无 metrics)一律退回空串,不阻断报告渲染 —— 展示名是锦上添花。
func subjectPresentation(r *store.ResearchRun, name string) (subjectType, displayName string) {
	subjectType = research.SubjectTypeOf(r.Code)
	switch subjectType {
	case store.SubjectIndustry, store.SubjectMacro:
		// 两者的展示名都在各自指标卡的 ref.name 里;主体类型不同 → 键不同。
		// len==0 的判定与 industry 一致:空 metrics 无从取名,退回空串。
	default:
		return subjectType, name // 公司:沿用实体富化名
	}
	if len(r.Metrics) == 0 {
		return subjectType, ""
	}
	if subjectType == store.SubjectMacro {
		var m struct {
			Macro struct {
				Ref struct {
					Name string `json:"name"`
				} `json:"ref"`
			} `json:"macro"`
		}
		if err := json.Unmarshal(r.Metrics, &m); err != nil {
			return subjectType, ""
		}
		return subjectType, m.Macro.Ref.Name
	}
	var m struct {
		IndustryIndex struct {
			Ref struct {
				Name string `json:"name"`
			} `json:"ref"`
		} `json:"industry_index"`
	}
	if err := json.Unmarshal(r.Metrics, &m); err != nil {
		return subjectType, ""
	}
	return subjectType, m.IndustryIndex.Ref.Name
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
