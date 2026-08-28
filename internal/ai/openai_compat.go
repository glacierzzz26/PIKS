package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAICompat 兼容 OpenAI /chat/completions 协议:覆盖 DeepSeek 官方 API、
// OpenAI、OpenRouter 及各类 OpenAI 兼容订阅源。换源只改 baseURL + model。
type OpenAICompat struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAICompat(baseURL, apiKey, model string) *OpenAICompat {
	return &OpenAICompat{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *OpenAICompat) Name() string { return "openai-compat:" + p.model }

// ListModels 获取 provider 可用模型 id 列表(GET /models,OpenAI 兼容)。settings 页下拉选择用。
func (p *OpenAICompat) ListModels(ctx context.Context) ([]string, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("api key 未配置")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("parse models: %w", err)
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

func (p *OpenAICompat) HealthCheck(ctx context.Context) error {
	if p.apiKey == "" {
		return fmt.Errorf("PIKS_AI_API_KEY not set")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check status %d", resp.StatusCode)
	}
	return nil
}

// ImagePart 截图上传的图片二进制(进 image_url data URI)。
type ImagePart struct {
	Data []byte
	MIME string // 如 image/png
}

// ChatOptions /chat 问答入参。Image 非空 = vision 请求(OpenAI 兼容 image_url 格式)。
type ChatOptions struct {
	System string
	User   string
	Image  *ImagePart
}

// ChatResponse 问答回复。
type ChatResponse struct {
	Content string
	Usage   Usage
}

// Chat 普通补全(非 JSON mode),/chat 知识库问答用。支持带图(截图识别)。
// 与 G7 探针一致:image_url 走 OpenAI 兼容格式;provider 不支持图片时由调用方降级。
func (p *OpenAICompat) Chat(ctx context.Context, opts ChatOptions) (ChatResponse, error) {
	if p.apiKey == "" {
		return ChatResponse{}, fmt.Errorf("api key 未配置")
	}
	var content any
	if opts.Image != nil {
		content = []map[string]any{
			{"type": "text", "text": opts.User},
			{"type": "image_url", "image_url": map[string]string{
				"url": "data:" + opts.Image.MIME + ";base64," + base64.StdEncoding.EncodeToString(opts.Image.Data),
			}},
		}
	} else {
		content = opts.User
	}
	payload := map[string]any{
		"model": p.model,
		"messages": []map[string]any{
			{"role": "system", "content": opts.System},
			{"role": "user", "content": content},
		},
		"temperature": 0.7,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ChatResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("api status %d: %s", resp.StatusCode, truncate(string(respBody), 400))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return ChatResponse{}, fmt.Errorf("parse response: %w", err)
	}
	if len(out.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("no choices in response")
	}
	return ChatResponse{
		Content: strings.TrimSpace(out.Choices[0].Message.Content),
		Usage: Usage{
			InputTokens:  out.Usage.PromptTokens,
			OutputTokens: out.Usage.CompletionTokens,
		},
	}, nil
}

func (p *OpenAICompat) StructuredOutput(ctx context.Context, req StructuredRequest) (StructuredResponse, error) {
	system := req.System
	if len(req.Schema) > 0 {
		system += "\n\n输出 JSON Schema:\n" + string(req.Schema)
	}
	// Image 非空 → user content 变 [text, image_url] 数组(与 Chat 一致,探针实测 vision+json_object 兼容)。
	var userContent any = req.User
	if req.Image != nil {
		userContent = []map[string]any{
			{"type": "text", "text": req.User},
			{"type": "image_url", "image_url": map[string]string{
				"url": "data:" + req.Image.MIME + ";base64," + base64.StdEncoding.EncodeToString(req.Image.Data),
			}},
		}
	}
	payload := map[string]any{
		"model": p.model,
		"messages": []map[string]any{
			{"role": "system", "content": system},
			{"role": "user", "content": userContent},
		},
		"temperature":     0,
		"response_format": map[string]any{"type": "json_object"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return StructuredResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return StructuredResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return StructuredResponse{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return StructuredResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return StructuredResponse{}, fmt.Errorf("api status %d: %s", resp.StatusCode, truncate(string(respBody), 400))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return StructuredResponse{}, fmt.Errorf("parse response: %w", err)
	}
	if len(out.Choices) == 0 {
		return StructuredResponse{}, fmt.Errorf("no choices in response")
	}
	content := strings.TrimSpace(out.Choices[0].Message.Content)
	if !json.Valid([]byte(content)) {
		return StructuredResponse{}, fmt.Errorf("model output is not valid JSON: %s", truncate(content, 200))
	}
	return StructuredResponse{
		Data: json.RawMessage(content),
		Usage: Usage{
			InputTokens:  out.Usage.PromptTokens,
			OutputTokens: out.Usage.CompletionTokens,
		},
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
