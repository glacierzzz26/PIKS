package research

import (
	"context"
	"encoding/json"
	"errors"
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
}

// Orchestrator 深研编排:exec Python → LLM 合成 → 机检 → 落 PG。
type Orchestrator struct {
	store    *store.Store
	provider ai.Provider
	runner   *runner
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
	code := store.NormalizeCode(opt.Code)
	if code == "" {
		return nil, errors.New("股票代码为空")
	}
	if opt.Profile == "" {
		opt.Profile = "complete-stock"
	}

	symbol := toFullCode(code)
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
		dir = filepath.Join(os.TempDir(), "piks-research", runID)
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
		if err := o.store.UpdateResearchRunStatus(ctx, runID, StatusGathering, ""); err != nil {
			return err
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
	prompt, err := readText(arts.promptPath())
	if err != nil || prompt == "" {
		return fmt.Errorf("读合成提示失败(指标卡不可得?): %v", err)
	}
	// 预算护栏:今日已用 ≥ 预算 → 如实失败,不降级不编造(设计 §4.4)。
	if err := o.checkBudget(ctx); err != nil {
		return err
	}
	synthTR, err := o.startTask(ctx, "research-run:synth", map[string]any{"code": code})
	if err != nil {
		return err
	}
	syn, usage, err := o.synthesize(ctx, prompt)
	if err != nil {
		o.finishTask(ctx, synthTR, "failed", err, nil)
		return err
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
	// 第 4 步:渲染进报告 + Number Lint(未过退出码 2,已落产物)。
	if _, err := o.runner.synthesize(ctx, dir, code, arts.synthPath()); err != nil {
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
	res.Tokens, res.Model = usage.Total(), model
	res.Lint = !lintFailed(lint)

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

// toFullCode 6 位代码 → research full_code(与 src/models/symbol.py resolve_symbol 同规则)。
func toFullCode(code string) string {
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
func lintFailed(lint json.RawMessage) bool { return jsonPassed(lint) == false && len(lint) > 0 }

func gatePassed(gate json.RawMessage) bool { return jsonPassed(gate) }

func jsonPassed(raw json.RawMessage) bool {
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
