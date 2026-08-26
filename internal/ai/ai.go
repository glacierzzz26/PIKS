// Package ai 定义 AIProvider 抽象。业务层只依赖本接口,不接触任何厂商 SDK。
package ai

import (
	"context"
	"encoding/json"
)

type Usage struct {
	InputTokens  int64
	OutputTokens int64
}

func (u Usage) Total() int64 { return u.InputTokens + u.OutputTokens }

type StructuredRequest struct {
	System string
	User   string
	// Schema 为输出约束的 JSON Schema(提示词注入 + 输出后本地校验)。
	Schema json.RawMessage
}

type StructuredResponse struct {
	Data  json.RawMessage
	Usage Usage
}

type Provider interface {
	Name() string
	StructuredOutput(ctx context.Context, req StructuredRequest) (StructuredResponse, error)
	HealthCheck(ctx context.Context) error
}
