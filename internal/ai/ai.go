// Package ai 定义 AIProvider 抽象。业务层只依赖本接口,不接触任何厂商 SDK。
package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// APIError 是 provider 返回非 200 时的**类型化**错误(issue #75)。
//
// 动机:此前 429/5xx 只能靠字符串包含 "api status 429" 识别,调用方无法区分
// 「限流(退避后可重试)」与「4xx 确定性错误(立刻重试也白搭)」,于是重试策略只能一刀切
// 或干脆不退避 —— 网关持续 429 时每轮把 3 次重试瞬间烧完。
//
// Status 语义:
//   - 429 / 5xx → **可重试**(Retryable()==true),调用方应指数退避后再试;
//   - 其它 4xx  → **确定性**(Retryable()==false),重试无意义,应立即放弃。
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api status %d: %s", e.Status, e.Body)
}

// Retryable 报告该错误是否值得退避后重试(429 限流 / 5xx 服务端瞬时故障)。
func (e *APIError) Retryable() bool {
	return e.Status == 429 || e.Status >= 500
}

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
	// Image 非空 = 视觉 + json_object(截图结构化抽取,如同花顺交易/持仓截图)。
	// 兼容性由 provider 决定;探针实测 deepseek-v4-flash-vision-exp 接受(2026-08-28)。
	Image *ImagePart
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
