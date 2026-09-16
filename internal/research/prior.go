package research

// issue #8:新一期研报把既往 done 研报作为合成输入(设计 research-merge.md:566「迭代 3」的起点)。
//
// 为什么需要这一层:LLM 若仅见本次指标卡,写不出"较上次 +12%"这类对比;
// 而一旦写出来,该数字不在**本次**指标卡 → Number Lint 判为编造 → markdown 回落骨架
// → AI 段落反而丢失。所以「喂历史材料」与「放开 lint 的 known 集」必须成对做,缺一即负收益:
//   - 本文件:Go 侧组装历史摘要(追加进 prompt)+ 落 prior_metrics.json(供 lint 取 known)
//   - Python 侧:cli.py `synthesize --prior-metrics` 把其中数字并入 known
//
// ⚠️ 数字提取不在 Go 侧重写 —— 交给 Python 既有的 collect_numbers_from_json。
// Go 只负责"挑哪几份、摘哪几个字段",避免同一套数字遍历逻辑两处维护。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PriorRunLimit 默认引用几份历史研报(web 触发路径的取值)。2 是刻意克制:控 token,
// 且"上次 + 上上次"足以看出趋势;再多边际信息很低,却显著抬高 prompt 成本与走偏风险。
const PriorRunLimit = 2

// priorSummary 一份历史研报的精简摘要(不塞整篇 markdown)。
type priorSummary struct {
	AsOf      string          `json:"as_of"`
	Profile   string          `json:"profile"`
	Score     json.RawMessage `json:"scorecard,omitempty"`
	Risk      string          `json:"risk_level,omitempty"`
	Synthesis json.RawMessage `json:"synthesis,omitempty"`
}

// loadPriorRuns 取同 code 既往 done 报告(排除本次)并组装精简摘要。
// limit<=0 = 关(Options.PriorRuns 未开),直接返回空 —— 首次研报与独立 CLI 的零回归路径。
// 任何失败都**不中断**合成 —— 历史材料是锦上添花,取不到就照常做(如实降级,不编造)。
// 返回 (摘要, 供 lint 用的原始 metrics 列表);两者均为空表示无历史(首次研报路径)。
func (o *Orchestrator) loadPriorRuns(ctx context.Context, code, excludeRunID string, limit int) ([]priorSummary, []json.RawMessage) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := o.store.ListPriorDoneResearchRuns(ctx, code, excludeRunID, limit)
	if err != nil {
		o.logf("取历史研报失败(不影响本次合成): %v", err)
		return nil, nil
	}
	if len(rows) == 0 {
		return nil, nil
	}

	summaries := make([]priorSummary, 0, len(rows))
	metrics := make([]json.RawMessage, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		summaries = append(summaries, priorSummary{
			AsOf:      r.AsOf.Format("2006-01-02"),
			Profile:   r.Profile,
			Score:     extractScorecard(r.Metrics),
			Risk:      extractRiskLevel(r.Metrics),
			Synthesis: r.Synthesis,
		})
		// 原样保留 metrics:lint 的 known 集由 Python 侧 collect_numbers_from_json 提取。
		if len(r.Metrics) > 0 {
			metrics = append(metrics, r.Metrics)
		}
	}
	return summaries, metrics
}

// extractScorecard 从 metrics 里摘 scorecard(overall/overall_label/dimensions)。
// 摘不到返回 nil —— 该份历史仍会被引用(as_of 与 synthesis 仍有用),只是少一个字段。
func extractScorecard(metrics json.RawMessage) json.RawMessage {
	if len(metrics) == 0 {
		return nil
	}
	var m struct {
		Scorecard json.RawMessage `json:"scorecard"`
	}
	if err := json.Unmarshal(metrics, &m); err != nil || len(m.Scorecard) == 0 {
		return nil
	}
	return m.Scorecard
}

// extractRiskLevel 从 metrics.risk.overall_level 摘风险等级(P7 起 risk 上屏)。
func extractRiskLevel(metrics json.RawMessage) string {
	if len(metrics) == 0 {
		return ""
	}
	var m struct {
		Risk struct {
			OverallLevel string `json:"overall_level"`
		} `json:"risk"`
	}
	if err := json.Unmarshal(metrics, &m); err != nil {
		return ""
	}
	return m.Risk.OverallLevel
}

// priorPromptBlock 把历史摘要与硬约束拼成追加到 prompt 末尾的一段文本。
// Go 读的是 Python 产出的 {code}_synthesis_prompt.txt,此处**追加**——
// 不改 Python 的 prompt 生成逻辑(D-11 独立性:只经产物与 CLI 交互)。
func priorPromptBlock(summaries []priorSummary) string {
	if len(summaries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n---\n\n")
	b.WriteString("## 此前研报要点(供对比参考)\n\n")
	b.WriteString("以下是该股票**既往**的研报要点。你可以与之对比,但必须遵守下列约束:\n\n")
	for _, s := range summaries {
		b.WriteString(fmt.Sprintf("### 数据截止 %s", s.AsOf))
		if s.Profile != "" {
			b.WriteString("(档案:" + s.Profile + ")")
		}
		b.WriteString("\n")
		if len(s.Score) > 0 {
			b.WriteString("- 评分卡:" + compactJSON(s.Score) + "\n")
		}
		if s.Risk != "" {
			b.WriteString("- 风险等级:" + s.Risk + "\n")
		}
		if len(s.Synthesis) > 0 {
			b.WriteString("- 上次定性:" + compactJSON(s.Synthesis) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("**对比时的硬性约束(与上文同等优先):**\n")
	b.WriteString("- 本次报告的数字仍**只能**取自**本次**指标卡;**不得**把历史数值当作当前值陈述。\n")
	b.WriteString("- 引用历史数值时**必须**显式标明出处,写法如「较上次(2026-09-12)+12%」或「此前报告显示…」。\n")
	b.WriteString("- 不得因为有了历史材料就放松溯源:无法标明出处的数字**一律不写**。\n")
	return b.String()
}

// compactJSON 把 JSON 压成单行(减少 prompt 里的无谓换行)。
func compactJSON(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

// writePriorMetrics 把历史 metrics 原样落成 {code}_prior_metrics.json(JSON 数组)。
// 文件名登记在 research/README.md 契约表;Go 只写不读(Python synthesize --prior-metrics 读)。
// 无历史 → 不落文件(保持"首次研报产物与改动前逐字节一致")。
func writePriorMetrics(dir, code string, metrics []json.RawMessage) (string, error) {
	if len(metrics) == 0 {
		return "", nil
	}
	p := filepath.Join(dir, code+"_prior_metrics.json")
	b, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// annotatePriorRuns 在 metrics 的 meta 里补一个 prior_runs 计数(issue #8,零 schema)。
//
// ⚠️ 这是**有意的 Go 侧补充**:落库的 metrics 因此与 Python 产物文件 {code}_metrics.json
// 不再逐字节相同(多一个 meta.prior_runs)。代价可接受 —— 换来"这份报告参考过几份历史"
// 可被回读,不必新增列走 migration;而 Python 侧从不读这个键(lint 只用原始数值集)。
// metrics 不可解析(空/损坏)时原样返回,不阻断落库。
func annotatePriorRuns(metrics json.RawMessage, n int) json.RawMessage {
	if n <= 0 || len(metrics) == 0 {
		return metrics
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(metrics, &root); err != nil {
		return metrics
	}
	var meta map[string]json.RawMessage
	if raw, ok := root["meta"]; ok {
		if err := json.Unmarshal(raw, &meta); err != nil {
			return metrics
		}
	}
	if meta == nil {
		meta = map[string]json.RawMessage{}
	}
	cnt, _ := json.Marshal(n)
	meta["prior_runs"] = cnt
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return metrics
	}
	root["meta"] = metaJSON
	out, err := json.Marshal(root)
	if err != nil {
		return metrics
	}
	return out
}
