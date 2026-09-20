package research

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"piks/internal/ai"
	"piks/internal/store"
)

// 状态机(设计 §4.4;status 列取值与前端轮询徽标一一对应)。
const (
	StatusPending      = "pending"
	StatusGathering    = "gathering"
	StatusSynthesizing = "synthesizing"
	StatusVerifying    = "verifying"
	StatusDone         = "done"
	StatusFailed       = "failed"
)

// Options 一次深研的运行参数(cmd/research-run 与 web 端点共用)。
type Options struct {
	Code    string // 6 位代码或 full_code(Go 侧归一)
	Profile string // complete-stock(默认) / short-term
	Days    int    // 覆盖展示窗口,0 = 用 profile 默认
	// RunID 非空 = 续跑该 run(断点重试:已产出的步骤跳过,仅重跑失败步)。
	// 空 = 新开一次(run_id 由 code+profile+时间戳生成)。
	RunID string
	// Force 丢弃产物、强制新开一次;与 RunID 同给时 Force 优先(不续跑旧的)。
	Force bool
	// OutDir 产物目录;空 = 按 run_id 落临时根(续跑时能凭 run_id 找回原目录)。
	OutDir string
	// RequireSynthesis 为 true(默认)时,合成步必须有 LLM 且成功,否则整轮 failed
	// (深研语义:无 AI 不产报告)。为 false(快速模式)时,LLM 缺失/失败/超预算不 fail:
	// 记空 synthesis 继续,markdown 保留骨架(含「待 AI 综合研判」占位)——用于「买入前速评」,
	// 结论取自确定性评分卡 + 风险规则,不被 AI 网关阻塞。
	RequireSynthesis bool
	// PriorRuns 引用几份既往 done 研报作合成输入(issue #8;0 = 关,新档默认关)。
	// 关时产物与改动前逐字节一致 —— 首次研报与独立 CLI 路径零回归。
	PriorRuns int
	// Claimed 由**队列认领方**(cmd/research-worker)置 true:该 run 已由认领写入
	// status=gathering(store.ClaimPendingResearchRun),故编排首个状态转移不再重复写。
	// 只有第一个转移需抑制 —— 其余四步(orchestrator 各自唯一)照常推进。
	// CLI 直跑与 web 进程内触发保持 false(无认领者,首个转移必须由编排自己写)。
	Claimed bool
}

// Orchestrator 深研编排:exec Python → LLM 合成 → 机检 → 落 PG。
type Orchestrator struct {
	store    *store.Store
	provider ai.Provider
	runner   cliRunner
	// budget 日 token 上限(0 = 关);合成前查 task_runs 当日累计。
	budget int64
	logf   func(format string, args ...any)
}

// New 构造编排器。provider 为 nil 时合成步骤会如实失败(不降级不编造)。
func New(s *store.Store, provider ai.Provider, budget int64) *Orchestrator {
	return &Orchestrator{
		store:    s,
		provider: provider,
		runner:   newRunner(),
		budget:   budget,
		logf:     log.Printf,
	}
}

// Result 一次编排的收口结果(供 CLI 打印与 web 端点返回)。
type Result struct {
	RunID  string
	Status string
	Error  string
	Gate   bool // 机检是否全过
	Lint   bool // Number Lint 是否通过
	Tokens int64
	Model  string
}

// Run 执行完整编排(设计 §4.4 六步)。
// 任何一步失败都如实落 status=failed + error,不掩盖;机检未过 ≠ 失败(报告仍可用)。
func (o *Orchestrator) Run(ctx context.Context, opt Options) (*Result, error) {
	// 主体归一(P9):公司=6 位数字,行业=sw+6 位申万码,宏观=macro:<key>。
	subjectType, code := store.NormalizeSubject(opt.Code)
	if subjectType == "" {
		return nil, fmt.Errorf(
			"无法识别的主体码 %q(公司应为 6 位数字,行业应为 sw+6 位申万代码,宏观应为 macro:<维度> 如 macro:cn_cpi)",
			opt.Code)
	}
	if opt.Profile == "" {
		opt.Profile = "complete-stock"
	}

	symbol := SubjectFullCode(opt.Code)
	asOf := time.Now()

	// 运行身份(Identity)与产物目录绑定:续跑沿用同一 run_id + 同一目录,
	// 否则"断点重跑"会变成"用别人的产物冒充新 run"(设计 §4.4 断点重试)。
	var runID string
	resumingExisting := false
	switch {
	case opt.RunID != "" && !opt.Force:
		// 显式续跑:校验该 run 存在且非 done(done 无需重跑)。
		existing, err := o.store.GetResearchRun(ctx, opt.RunID)
		if err != nil {
			return nil, fmt.Errorf("查待续跑 run 失败: %w", err)
		}
		if existing == nil {
			return nil, fmt.Errorf("待续跑的 run %s 不存在", opt.RunID)
		}
		if existing.Status == StatusDone {
			o.logf("run %s 已 done,无需续跑", existing.RunID)
			return resultFrom(existing), nil
		}
		runID, resumingExisting = existing.RunID, true
	case opt.Force:
		// 强制新档:加 _f 后缀 + 新时间戳,旧 run 与旧产物都保留(历史可回溯)。
		runID = fmt.Sprintf("%s_%s_%s_f", symbol, opt.Profile, asOf.Format("20060102_150405"))
	default:
		runID = fmt.Sprintf("%s_%s_%s", symbol, opt.Profile, asOf.Format("20060102_150405"))
	}

	dir := opt.OutDir
	if dir == "" {
		// 产物根可覆盖:容器里挂命名卷,否则 /tmp 易失 → 断点重跑无从复用。
		root := os.Getenv("PIKS_RESEARCH_OUT_DIR")
		if root == "" {
			root = filepath.Join(os.TempDir(), "piks-research")
		}
		dir = filepath.Join(root, runID)
	}
	if opt.Force {
		_ = os.RemoveAll(dir) // --force 才清空既有产物(续跑必须保留,否则无可复用)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("建产物目录失败: %w", err)
	}

	if !resumingExisting {
		created, err := o.store.CreateResearchRun(ctx, &store.ResearchRun{
			RunID: runID, Code: code, Symbol: symbol, Profile: opt.Profile,
			AsOf: asOf, Status: StatusPending,
		})
		if err != nil {
			return nil, fmt.Errorf("建 run 失败: %w", err)
		}
		if !created {
			// 幂等:同秒重跑命中已存在 run → 续跑它,不重复采集(幂等键稳定)。
			existing, err := o.store.GetResearchRun(ctx, runID)
			if err != nil {
				return nil, err
			}
			if existing.Status == StatusDone {
				o.logf("run %s 已存在且 done,跳过", runID)
				return resultFrom(existing), nil
			}
			resumingExisting = true
			o.logf("run %s 已存在(status=%s),按其产物续跑", runID, existing.Status)
		}
	}

	res := &Result{RunID: runID, Status: StatusPending}
	if err := o.execute(ctx, opt, code, symbol, runID, dir, asOf, res); err != nil {
		// 失败如实落库:error 原样,不猜测、不补数据。
		_ = o.store.FinishResearchRun(ctx, runID, StatusFailed, err.Error(), nil)
		res.Status, res.Error = StatusFailed, err.Error()
		o.logf("run %s 失败: %v", runID, err)
		return res, nil // 编排已如实收口,返回 res 而非 error(CLI 按 status 判退出码)
	}
	return res, nil
}

// execute 状态机主体:gathering → synthesizing → verifying → done。
func (o *Orchestrator) execute(ctx context.Context, opt Options, code, symbol, runID, dir string, asOf time.Time, res *Result) error {
	arts := &artifacts{dir: dir, code: code}

	// ---- 1. gathering:采集 + 确定性分析(断点重跑:产物已在则跳过) ----
	if !arts.exists(arts.metricsPath()) {
		// opt.Claimed:队列认领方已写过 gathering(claim 即首个转移),此处不再重复写。
		if !opt.Claimed {
			if err := o.store.UpdateResearchRunStatus(ctx, runID, StatusGathering, ""); err != nil {
				return err
			}
		}
		tr, err := o.startTask(ctx, "research-run:gather", map[string]any{"code": code, "profile": opt.Profile})
		if err != nil {
			return err
		}
		if _, err := o.runner.gather(ctx, code, opt.Profile, opt.Days, dir, runID); err != nil {
			o.finishTask(ctx, tr, "failed", err, nil)
			return fmt.Errorf("采集失败: %w", err)
		}
		o.finishTask(ctx, tr, "success", nil, nil)
	}

	// 契约校验(§4.10 G2):run_meta.json 存在 + contract 版本不高于本端支持。
	opened, err := openArtifacts(dir, code)
	if err != nil {
		return err
	}
	arts = opened
	// 用 Python 侧实际产出的 run_id 覆盖 Go 生成的(以 Python 产物为准,保持可追溯)。
	if arts.meta.RunID != "" && arts.meta.RunID != runID {
		o.logf("注意: run_meta.run_id=%s 与编排 run_id=%s 不一致,以产物为准", arts.meta.RunID, runID)
	}

	metrics, err := readJSON(arts.metricsPath())
	if err != nil {
		return fmt.Errorf("读指标卡失败: %w", err)
	}
	var meta struct {
		Meta struct {
			AsOf string `json:"as_of"`
		} `json:"meta"`
		Evidence json.RawMessage `json:"evidence"`
	}
	_ = json.Unmarshal(metrics, &meta)
	if meta.Meta.AsOf != "" {
		if d, err := time.Parse("2006-01-02", meta.Meta.AsOf); err == nil {
			asOf = d
		}
	}

	// 落采集成果(Fact + 溯源链);synthesis 待合成后补。
	if err := o.store.SaveResearchArtifacts(ctx, runID, &store.ResearchRun{
		Metrics: metrics, Evidence: meta.Evidence,
	}); err != nil {
		return fmt.Errorf("落指标卡失败: %w", err)
	}
	// as_of 以指标卡为准回写(防未来函数基准)。
	if err := o.store.UpdateResearchRunAsOf(ctx, runID, asOf); err != nil {
		return err
	}

	// ---- 2. synthesizing:PIKS ai.Provider 生成三段定性 ----
	if err := o.store.UpdateResearchRunStatus(ctx, runID, StatusSynthesizing, ""); err != nil {
		return err
	}
	if err := o.synthesizeStep(ctx, arts, runID, code, res, opt.RequireSynthesis, opt.PriorRuns); err != nil {
		return err
	}

	// ---- 3. verifying:六项 Quality Gate ----
	if err := o.store.UpdateResearchRunStatus(ctx, runID, StatusVerifying, ""); err != nil {
		return err
	}
	gateTR, err := o.startTask(ctx, "research-run:verify", map[string]any{"code": code})
	if err != nil {
		return err
	}
	if _, err := o.runner.gate(ctx, dir, code); err != nil {
		o.finishTask(ctx, gateTR, "failed", err, nil)
		return fmt.Errorf("机检执行失败: %w", err)
	}
	o.finishTask(ctx, gateTR, "success", nil, nil)
	gate, err := readJSON(arts.gatePath())
	if err != nil {
		return fmt.Errorf("读机检结果失败: %w", err)
	}
	res.Gate = gatePassed(gate)

	// ---- 4. done:机检未过也算 done(产物可用),问题清单原样落库 ----
	if err := o.store.FinishResearchRun(ctx, runID, StatusDone, "", &store.ResearchRun{Gate: gate}); err != nil {
		return err
	}
	res.Status = StatusDone
	o.logf("run %s 完成: gate=%v lint=%v tokens=%d", runID, res.Gate, res.Lint, res.Tokens)
	return nil
}

// synthesizeStep 执行合成步。requireSynth=true(深研语义):LLM 缺失/失败/超预算 → 返回 error,
// 整轮 failed,不降级不编造。requireSynth=false(快速模式):上述情况记空 synthesis 继续,
// markdown 用骨架(含「待 AI 综合研判」占位),让确定性分析(评分卡/风险/量价形态)照常产出。
// priorRuns>0 时把最近几份 done 研报作合成输入(issue #8),并落 {code}_prior_metrics.json 供 lint 放开 known 集。
func (o *Orchestrator) synthesizeStep(ctx context.Context, arts *artifacts, runID, code string, res *Result, requireSynth bool, priorRuns int) error {
	dir := arts.dir

	prompt, err := readText(arts.promptPath())
	if err != nil || prompt == "" {
		return fmt.Errorf("读合成提示失败(指标卡不可得?): %v", err)
	}

	// issue #8:既往 done 研报作合成输入。用 Python 的 prompt 原文 + 追加历史摘要段,
	// 并把这些 metrics 落成 {code}_prior_metrics.json 供 lint 放开 known 集。
	// 无历史(priorRuns=0 或首次研报)→ 下面三步全是 no-op,产物与改动前逐字节一致。
	priors, priorMetrics := o.loadPriorRuns(ctx, code, runID, priorRuns)
	prompt += priorPromptBlock(priors)
	priorMetricsPath, err := writePriorMetrics(dir, code, priorMetrics)
	if err != nil {
		return fmt.Errorf("写历史研报数字失败: %w", err)
	}
	// 预算护栏:今日已用 ≥ 预算 → 深研如实失败;快速模式改走骨架。
	if err := o.checkBudget(ctx); err != nil {
		if requireSynth {
			return err
		}
		return o.synthFallback(ctx, arts, runID, "预算护栏: "+err.Error())
	}

	synthTR, err := o.startTask(ctx, "research-run:synth", map[string]any{"code": code})
	if err != nil {
		return err
	}
	syn, usage, err := o.synthesize(ctx, prompt)
	if err != nil {
		if requireSynth {
			o.finishTask(ctx, synthTR, "failed", err, nil)
			return err
		}
		// 快速模式:LLM 不可得不算失败,记因由后走骨架。
		o.finishTask(ctx, synthTR, "skipped", err, nil)
		return o.synthFallback(ctx, arts, runID, err.Error())
	}
	model := ""
	if o.provider != nil {
		model = o.provider.Name()
	}
	o.finishTask(ctx, synthTR, "success", nil, map[string]any{"ai_tokens": usage.Total()})

	// 写 LLM 输出供 `synthesize` 子命令读,再渲染 + Number Lint。
	if err := writeJSON(arts.synthPath(), syn); err != nil {
		return fmt.Errorf("写合成结果失败: %w", err)
	}
	// 渲染进报告 + Number Lint(未过退出码 2,已落产物)。
	// priorMetricsPath 为空(synthesize 自动跳过)或指向历史 metrics → lint 把历史数字并入 known。
	if _, err := o.runner.synthesize(ctx, dir, code, arts.synthPath(), priorMetricsPath); err != nil {
		return fmt.Errorf("渲染/机检失败: %w", err)
	}
	lint, err := readJSON(arts.lintPath())
	if err != nil {
		return fmt.Errorf("读 lint 结果失败: %w", err)
	}
	finalMD, err := readText(arts.finalPath())
	if err != nil {
		return err
	}
	// 机检未过不静默:markdown 回落确定性骨架报告(设计 §4.4 机检失败处置)。
	markdown := finalMD
	if lintFailed(lint) {
		if skeleton, err := readText(arts.skeletonPath()); err == nil && skeleton != "" {
			markdown = skeleton
		}
		o.logf("run %s: Number Lint 未通过,markdown 回落骨架报告", runID)
	}
	synthJSON, _ := json.Marshal(syn)
	if err := o.store.SaveResearchArtifacts(ctx, runID, &store.ResearchRun{
		Synthesis: synthJSON, Markdown: &markdown, Lint: lint,
		Model: model, Tokens: usage.Total(),
	}); err != nil {
		return fmt.Errorf("落合成/机检成果失败: %w", err)
	}
	// issue #8:把"参考了 N 份历史研报"标进 metrics.meta(零 schema;前端可如实展示)。
	// 无历史 → annotatePriorRuns 是 no-op,metrics 保持 Python 产物原样。
	if len(priorMetrics) > 0 {
		// metrics 是 Python 产物文件(合成步内自取 —— 抽取 synthesizeStep 后不再有
		// execute 作用域里的同名变量)。
		metrics, err := readJSON(arts.metricsPath())
		if err != nil {
			return fmt.Errorf("读指标卡失败(无法标注历史研报引用): %w", err)
		}
		if err := o.store.SaveResearchArtifacts(ctx, runID, &store.ResearchRun{
			Metrics: annotatePriorRuns(metrics, len(priorMetrics)),
		}); err != nil {
			return fmt.Errorf("标注历史研报引用失败: %w", err)
		}
	}
	res.Tokens, res.Model = usage.Total(), model
	res.Lint = !lintFailed(lint)
	return nil
}

// synthFallback 快速模式下的合成降级:用骨架报告作 markdown,落 lint=未过(含 skipped 原因),
// 不写 synthesis(前端据此如实显示「AI 综合研判未生成」)。确定性指标卡早已落库,不受影响。
func (o *Orchestrator) synthFallback(ctx context.Context, arts *artifacts, runID, reason string) error {
	o.logf("run %s: 快速模式跳过 AI 合成(%s),使用骨架报告", runID, reason)
	skeleton, err := readText(arts.skeletonPath())
	if err != nil || skeleton == "" {
		return fmt.Errorf("读骨架报告失败(合成不可得且无兜底): %v", err)
	}
	lint := json.RawMessage(`{"scanned":0,"matched":0,"ignored":0,"passed":false,"issues":[]}`)
	if err := o.store.SaveResearchArtifacts(ctx, runID, &store.ResearchRun{
		Markdown: &skeleton, Lint: lint,
	}); err != nil {
		return fmt.Errorf("落骨架报告失败: %w", err)
	}
	return nil
}

// checkBudget 日 token 护栏(与 weekly.go / entity-build 同口径)。
func (o *Orchestrator) checkBudget(ctx context.Context) error {
	if o.budget <= 0 {
		return nil
	}
	today, err := o.store.TokensSince(ctx, time.Now().Truncate(24*time.Hour))
	if err != nil {
		return nil // 记账查询失败不拦合成(与现有实现一致)
	}
	if today >= o.budget {
		return fmt.Errorf("AI 日 token 预算已耗尽(%d/%d),不调用 LLM", today, o.budget)
	}
	return nil
}

// startTask 记 task_runs 开始(设计 §4.4 记账)。失败不阻断编排(记账不是业务)。
func (o *Orchestrator) startTask(ctx context.Context, command string, meta map[string]any) (int64, error) {
	id, err := o.store.StartTaskRun(ctx, command)
	if err != nil {
		o.logf("task_runs 开始记录失败(不阻断): %v", err)
		return 0, nil
	}
	_ = meta
	return id, nil
}

func (o *Orchestrator) finishTask(ctx context.Context, id int64, status string, err error, meta map[string]any) {
	if id == 0 {
		return
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if meta == nil {
		meta = map[string]any{}
	}
	if err := o.store.FinishTaskRun(ctx, id, status, msg, meta); err != nil {
		o.logf("task_runs 收尾记录失败(不阻断): %v", err)
	}
}

// NewRunID 生成一次深研的 run_id(主体 full_code + profile + 时间戳)。
// 触发端(web)先建 pending 行并立即返回它,后台 goroutine 再按同一 id 续跑 —— 见 §4.8。
// P9:主体感知 —— 行业主体为 sw801010_industry_...,不再被误加 bj 前缀。
func NewRunID(code, profile string, t time.Time) string {
	return fmt.Sprintf("%s_%s_%s", SubjectFullCode(code), profile, t.Format("20060102_150405"))
}

// ToFullCode 6 位代码 → research full_code(与 src/models/symbol.py resolve_symbol 同规则)。
// ⚠️ 只用于**股票**:它把 8/43/83/87/88 开头当北交所,会把申万行业码 850111 误标成 bj850111。
// 非股票主体走 SubjectFullCode。
func ToFullCode(code string) string {
	switch {
	case hasPrefix(code, "600", "601", "603", "605", "688", "689"):
		return "sh" + code
	case hasPrefix(code, "000", "001", "002", "003", "300", "301"):
		return "sz" + code
	case hasPrefix(code, "8", "43", "83", "87", "88"):
		return "bj" + code
	}
	return code
}

// SubjectFullCode 主体码 → research full_code(P9 / issue #10,主体轴泛化)。
// 行业/宏观的规范码已自带前缀(sw801010 / macro:cpi),原样返回 ——
// 它们不是股票,**不得**加交易所前缀(加 sh/sz/bj 会造出 bj801010 这种错码,
// 且 Python 侧 resolve_symbol 会把行业码按北交所给 30% 涨跌停,实测)。
// 公司走既有 ToFullCode,逐字节不变。
func SubjectFullCode(symbol string) string {
	subjectType, code := store.NormalizeSubject(symbol)
	switch subjectType {
	case store.SubjectIndustry, store.SubjectMacro:
		return code // 规范码自带前缀
	default:
		return ToFullCode(code) // 公司(或不可识别时原样,与既有行为一致)
	}
}

// SubjectTypeOf 主体类型(company / industry / macro);不可识别返回 ""。
// 供 web 层与 DTO 做主体感知分支,不改变任何既有行为。
func SubjectTypeOf(symbol string) string {
	st, _ := store.NormalizeSubject(symbol)
	return st
}

func hasPrefix(s string, ps ...string) bool {
	for _, p := range ps {
		if len(p) <= len(s) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// lintFailed / gatePassed 读机检 JSON 的 passed 字段(缺字段按未过? 否——缺视为不可判定,
// 如实返回 false,由前端展示问题清单)。
func lintFailed(lint json.RawMessage) bool { return JSONPassed(lint) == false && len(lint) > 0 }

func gatePassed(gate json.RawMessage) bool { return JSONPassed(gate) }

// JSONPassed 读机检产物({..._lint.json / {..._gate.json})的 passed 字段。
// 导出供 web 列表页取机检徽标;缺失/不可解析 → false(不可判定不当通过)。
func JSONPassed(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var v struct {
		Passed *bool `json:"passed"`
	}
	if err := json.Unmarshal(raw, &v); err != nil || v.Passed == nil {
		return false
	}
	return *v.Passed
}

func resultFrom(r *store.ResearchRun) *Result {
	res := &Result{RunID: r.RunID, Status: r.Status, Model: r.Model, Tokens: r.Tokens}
	if r.Error != nil {
		res.Error = *r.Error
	}
	res.Gate = gatePassed(r.Gate)
	res.Lint = !lintFailed(r.Lint)
	return res
}
