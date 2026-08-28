package ai

// G8 语义检索(方案 B,2026-08-28):query 端同义扩展。
// Zen 无 embeddings 端点(探针:POST /embeddings → 404),故用现有 extract 档模型
// 把用户问题扩展为「原文 + 同义/近义改写」检索词集,并入 n-gram 检索(低权重)。
// 失败由调用方降级为纯关键词检索,页面如实标注,不报错不编造。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ExpandQuery 用 extract 档模型把问题扩展为检索词集(G8 方案 B)。
// 返回:第 1 个元素恒为问题原文,后随 LLM 扩展的同义/近义表达;最多 15 个。
// JSON mode 输出 {"terms": [...]}(temperature 0,确定性)。任何失败返回错误。
func (p *OpenAICompat) ExpandQuery(ctx context.Context, q string) ([]string, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("api key 未配置")
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, fmt.Errorf("问题为空")
	}
	system := `你是 A 股投资知识库的检索词扩展器。给定用户问题,输出与问题语义相关的检索词列表。
规则:
- 第 1 个词必须是问题原文;
- 输出同义词、近义表达、常见改写(如「降准」→「下调存款准备金率」「存款准备金下调」「法定存款准备金率」);
- 只输出名词/动词短语,不输出停用词、疑问词;
- 5~15 个,中文为主,可含英文缩写/数字;
- 仅输出 JSON,结构为 {"terms": ["降准","下调存款准备金率","存款准备金下调"]}。`
	resp, err := p.StructuredOutput(ctx, StructuredRequest{
		System: system,
		User:   "用户问题:" + q,
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Terms []string `json:"terms"`
	}
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return nil, fmt.Errorf("解析扩展词: %w", err)
	}
	// 原文恒置首位(模型不守则也保证),去重/去空,上限 15。
	cleaned := []string{q}
	seen := map[string]bool{q: true}
	for _, t := range out.Terms {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		cleaned = append(cleaned, t)
		if len(cleaned) >= 15 {
			break
		}
	}
	return cleaned, nil
}
