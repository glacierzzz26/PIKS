package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"piks/internal/ai"
)

// synthesisSchema LLM 输出约束:三段纯字符串(设计 §4.5)。
// 本地还会用 validate_synthesis 的等价规则再校验一次(空/过短即拒)
// ——provider 的 schema 只是提示,不能当保证。
var synthesisSchema = json.RawMessage(`{
  "type":"object",
  "properties":{
    "summary":{"type":"string"},
    "trend":{"type":"string"},
    "conclusion":{"type":"string"}
  },
  "required":["summary","trend","conclusion"]
}`)

// synthesis LLM 三段定性(= Opinion,与 metrics 的 Fact 严格分域)。
type synthesis struct {
	Summary    string `json:"summary"`
	Trend      string `json:"trend"`
	Conclusion string `json:"conclusion"`
}

// minSlotLen 与 research `validate_synthesis` 的长度下限一致(空/过短即视为无效合成)。
const minSlotLen = 20

// synthesize 用 PIKS 的 ai.Provider 生成三段定性(D-3)。
// System = research 产出的合成提示原文(含"只能引用指标卡数字"硬约束);
// 其后的 Number Lint(第 4 步)是第二道防幻觉闸门——LLM 编造的数字会被机检标出。
func (o *Orchestrator) synthesize(ctx context.Context, prompt string) (synthesis, ai.Usage, error) {
	var s synthesis
	if o.provider == nil {
		return s, ai.Usage{}, fmt.Errorf("AI 未配置(ai_service_base_url / ai_api_key / 模型),无法合成")
	}
	resp, err := o.provider.StructuredOutput(ctx, ai.StructuredRequest{
		System: prompt,
		User:   "请按提示要求输出三段定性分析(summary / trend / conclusion),严格只输出 JSON。",
		Schema: synthesisSchema,
	})
	if err != nil {
		return s, ai.Usage{}, fmt.Errorf("LLM 合成失败: %w", err)
	}
	if err := json.Unmarshal(resp.Data, &s); err != nil {
		return s, resp.Usage, fmt.Errorf("解析 LLM 输出失败: %w", err)
	}
	if err := s.validate(); err != nil {
		return s, resp.Usage, err
	}
	return s, resp.Usage, nil
}

// validate 与 research `validate_synthesis` 同口径:三槽位齐全且非过短。
func (s synthesis) validate() error {
	for slot, v := range map[string]string{
		"summary": s.Summary, "trend": s.Trend, "conclusion": s.Conclusion,
	} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("合成槽位 %s 为空", slot)
		}
		if len([]rune(strings.TrimSpace(v))) < minSlotLen {
			return fmt.Errorf("合成槽位 %s 过短(<%d 字)", slot, minSlotLen)
		}
	}
	return nil
}
