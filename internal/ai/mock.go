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
