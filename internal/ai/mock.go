package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Mock 确定性 mock provider(测试用,不需要 API key):
// 根据关键词返回预置的结构化事件,验证管道逻辑用,不做真实语义。
type Mock struct{}

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Name() string { return "mock" }

func (m *Mock) HealthCheck(ctx context.Context) error { return nil }

func (m *Mock) StructuredOutput(ctx context.Context, req StructuredRequest) (StructuredResponse, error) {
	user := req.User
	var out struct {
		Events []struct {
			Title      string   `json:"title"`
			EventType  string   `json:"event_type"`
			Summary    string   `json:"summary"`
			Facts      []string `json:"facts"`
			Affected   []string `json:"affected"`
			OccurredAt string   `json:"occurred_at"`
			Confidence float64  `json:"confidence"`
		} `json:"events"`
	}
	switch {
	case strings.Contains(req.System, "A 股研究分析师"):
		// 深研合成(research-run):三段定性。数字刻意不写,避免 mock 编数触发 Number Lint。
		data, _ := json.Marshal(map[string]any{
			"summary":    "该股近期量价表现平稳,数据来自确定性计算引擎,基本面与事件面缺乏显著驱动。",
			"trend":      "从指标卡看,区间涨跌幅与波动率均处于常规区间,未见趋势性放量或缩量特征,资金面无异常信号。",
			"conclusion": "综合评分卡各项维度,当前呈中性倾向,建议持续跟踪后续量价变化与事件面更新,不构成任何投资建议。",
		})
		return StructuredResponse{Data: data, Usage: Usage{InputTokens: 800, OutputTokens: 200}}, nil
	case strings.Contains(req.System, "事件去重确认"):
		// 去重批量确认:按行解析 "#N: 事件A: X | 事件B: Y",同关键词 → 同事件。
		var results []map[string]any
		for _, ln := range strings.Split(user, "\n") {
			ln = strings.TrimSpace(ln)
			if !strings.HasPrefix(ln, "#") || !strings.Contains(ln, "事件A:") {
				continue
			}
			idxStr := ln[1:]
			if k := strings.Index(idxStr, ":"); k >= 0 {
				idxStr = idxStr[:k]
			}
			var idx int
			fmt.Sscanf(strings.TrimSpace(idxStr), "%d", &idx)
			parts := strings.SplitN(ln, "|", 2)
			if len(parts) != 2 {
				continue
			}
			ta := strings.TrimSpace(strings.SplitN(parts[0], "事件A:", 2)[1])
			tb := strings.TrimSpace(strings.SplitN(parts[1], "事件B:", 2)[1])
			jj := func(s string) bool { return strings.Contains(s, "降准") || strings.Contains(s, "存款准备金率") }
			same := false
			if jj(ta) && jj(tb) {
				same = true
			} else if strings.Contains(ta, "固态电池") && strings.Contains(tb, "固态电池") {
				same = true
			}
			results = append(results, map[string]any{
				"pair_index": idx,
				"is_same":    same,
			})
		}
		data, _ := json.Marshal(map[string]any{"results": results})
		return StructuredResponse{Data: data, Usage: Usage{InputTokens: 50, OutputTokens: 30}}, nil
	case strings.Contains(user, "降准"):
		out.Events = append(out.Events, struct {
			Title      string   `json:"title"`
			EventType  string   `json:"event_type"`
			Summary    string   `json:"summary"`
			Facts      []string `json:"facts"`
			Affected   []string `json:"affected"`
			OccurredAt string   `json:"occurred_at"`
			Confidence float64  `json:"confidence"`
		}{
			Title:      "央行宣布下调存款准备金率0.25个百分点",
			EventType:  "policy",
			Summary:    "中国人民银行宣布自9月1日起下调存款准备金率0.25个百分点。",
			Facts:      []string{"央行宣布下调金融机构存款准备金率0.25个百分点", "降准自2026年9月1日起实施", "央行表示此举旨在保持流动性合理充裕、加大对实体经济支持力度"},
			Affected:   []string{"银行", "房地产"},
			OccurredAt: "2026-08-26T09:30:00+08:00",
			Confidence: 0.9,
		})
	case strings.Contains(user, "固态电池"):
		out.Events = append(out.Events, struct {
			Title      string   `json:"title"`
			EventType  string   `json:"event_type"`
			Summary    string   `json:"summary"`
			Facts      []string `json:"facts"`
			Affected   []string `json:"affected"`
			OccurredAt string   `json:"occurred_at"`
			Confidence float64  `json:"confidence"`
		}{
			Title:      "星河新能源发布新一代固态电池技术路线图",
			EventType:  "tech",
			Summary:    "星河新能源公布新一代固态电池技术路线图,并公布2027年量产装车目标。",
			Facts:      []string{"星河新能源公布固态电池技术路线图", "公司公布2027年量产装车目标", "公司称新电池能量密度较现有产品提升40%"},
			Affected:   []string{"新能源"},
			OccurredAt: "2026-08-26T14:00:00+08:00",
			Confidence: 0.85,
		})
	default:
		return StructuredResponse{}, fmt.Errorf("mock: no fixture matched")
	}
	data, _ := json.Marshal(out)
	return StructuredResponse{Data: data, Usage: Usage{InputTokens: 100, OutputTokens: 50}}, nil
}
