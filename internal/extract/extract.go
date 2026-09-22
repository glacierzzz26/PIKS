// Package extract 抽取管道:raw_documents → LLM 结构化输出 → Schema/业务校验 → events/evidences。
// 严格遵守:AI 只产 Fact 层;推测内容不进库(Fact≠Inference≠Belief)。
package extract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

// ⚠️ 事件类型枚举的**唯一真源是 `model.EventTypes`**(issue #61)。此处不再手抄第二份 ——
// 曾经这里一份 `allowedEventTypes`、下面 Schema 里再抄一份 `enum`,两处都与前端漂移过。
var allowedEventTypes = model.EventTypeCSV()

const jsonSchemaTmpl = `{"type":"object","properties":{"events":{"type":"array","maxItems":5,"items":{"type":"object","properties":{
"title":{"type":"string"},
"event_type":{"type":"string","enum":[%s]},
"summary":{"type":"string"},
"facts":{"type":"array","items":{"type":"string"}},
"affected":{"type":"array","items":{"type":"string"}},
"occurred_at":{"type":"string"},
"confidence":{"type":"number","minimum":0,"maximum":1}
},"required":["title","event_type","facts","confidence"]}}},"required":["events"]}`

// JSONSchema 由真源拼出 enum 字面量 —— 不会再与 allowedEventTypes 漂移。
var JSONSchema = fmt.Sprintf(jsonSchemaTmpl, eventTypeEnumJSON())

// eventTypeEnumJSON 把真源 key 拼成 JSON 字符串数组字面量:`"policy","earnings",…`。
func eventTypeEnumJSON() string {
	keys := model.EventTypeKeys()
	quoted := make([]string, 0, len(keys))
	for _, k := range keys {
		quoted = append(quoted, strconv.Quote(k))
	}
	return strings.Join(quoted, ",")
}

type ExtractedEvent struct {
	Title      string   `json:"title"`
	EventType  string   `json:"event_type"`
	Summary    string   `json:"summary"`
	Facts      []string `json:"facts"`
	Affected   []string `json:"affected"`
	OccurredAt string   `json:"occurred_at"`
	Confidence float64  `json:"confidence"`
}

type output struct {
	Events []ExtractedEvent `json:"events"`
}

// Extractor 负责:构建提示词 → 调模型(重试≤3)→ 校验 → 入库。
type Extractor struct {
	Provider ai.Provider
	Store    *store.Store
	Prompt   string // 系统提示词(含注入的 Schema)
	Model    string

	promptHash string
	loaded     bool
}

func NewExtractor(provider ai.Provider, s *store.Store, promptPath, model string) (*Extractor, error) {
	body, err := os.ReadFile(promptPath)
	if err != nil {
		return nil, fmt.Errorf("read prompt %s: %w", promptPath, err)
	}
	system := strings.ReplaceAll(string(body), "{schema}", JSONSchema)
	sum := sha256.Sum256([]byte(system))
	e := &Extractor{
		Provider:   provider,
		Store:      s,
		Prompt:     system,
		Model:      model,
		promptHash: hex.EncodeToString(sum[:])[:8],
		loaded:     true,
	}
	return e, nil
}

// PipelineVersion 抽取血缘:pipeline 提示词版本 + 模型。
func (e *Extractor) PipelineVersion() string { return "extract@" + e.promptHash + "-" + e.Model }

// Extract 对单条 raw_document 抽取并入库。返回新建事件数与 token 用量。
// 全部事件被校验拒绝时返回错误(由 worker 标记 failed)。
func (e *Extractor) Extract(ctx context.Context, doc *model.RawDocument) (int, int64, error) {
	if !e.loaded {
		return 0, 0, fmt.Errorf("extractor not loaded")
	}
	user := buildUserPrompt(doc)

	var resp ai.StructuredResponse
	var lastErr error
	total := int64(0)
	for attempt := 1; attempt <= 3; attempt++ {
		r, err := e.Provider.StructuredOutput(ctx, ai.StructuredRequest{System: e.Prompt, User: user, Schema: json.RawMessage(JSONSchema)})
		if err != nil {
			// ⚠️ issue #75 账本盲区:只在**成功**时累加 token。旧实现对报错的调用也加
			// `r.Usage.Total()`,而失败时 Usage 恒为 0,等于把失败次数混进花费口径。
			lastErr = err
			continue
		}
		total += r.Usage.Total()
		resp = r
		break
	}
	if len(resp.Data) == 0 {
		return 0, total, fmt.Errorf("extract failed after 3 attempts: %w", lastErr)
	}

	events, err := validate(resp.Data)
	if err != nil {
		return 0, total, fmt.Errorf("validation: %w", err)
	}

	pv := e.PipelineVersion()
	count := 0
	for _, ev := range events {
		eventID, err := e.Store.CreateEvent(ctx, &model.Event{
			RawDocumentID:   &doc.ID,
			Title:           ev.Title,
			EventType:       normalizeEventType(ev.EventType),
			Summary:         strPtrIf(ev.Summary),
			Facts:           mustJSONArray(ev.Facts),
			Affected:        mustJSONArray(ev.Affected),
			OccurredAt:      parseTime(ev.OccurredAt),
			Confidence:      clamp01(ev.Confidence),
			Status:          "extracted",
			PipelineVersion: &pv,
			SourceID:        &doc.SourceID,
		})
		if err != nil {
			return count, total, fmt.Errorf("create event: %w", err)
		}
		if _, err := e.Store.CreateEvidence(ctx, &model.Evidence{
			EventID:     &eventID,
			Claim:       ev.Title,
			SourceID:    &doc.SourceID,
			SourceType:  strPtrIf("news"),
			URL:         doc.URL,
			Title:       doc.Title,
			Content:     strPtrIf(doc.Content),
			PublishedAt: doc.PublishedAt,
			Reliability: strPtrIf("medium"),
		}); err != nil {
			return count, total, fmt.Errorf("create evidence: %w", err)
		}
		count++
	}
	if count == 0 {
		return 0, total, fmt.Errorf("no valid events after validation")
	}
	return count, total, nil
}

func buildUserPrompt(doc *model.RawDocument) string {
	published := ""
	if doc.PublishedAt != nil {
		published = doc.PublishedAt.Format(time.RFC3339)
	}
	title := ""
	if doc.Title != nil {
		title = *doc.Title
	}
	return fmt.Sprintf("标题:%s\n发布时间:%s\n正文:\n%s", title, published, doc.Content)
}

func validate(data json.RawMessage) ([]ExtractedEvent, error) {
	var out output
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	if len(out.Events) == 0 {
		return nil, fmt.Errorf("empty events array")
	}
	valid := make([]ExtractedEvent, 0, len(out.Events))
	for _, ev := range out.Events {
		if strings.TrimSpace(ev.Title) == "" || len(ev.Facts) == 0 {
			continue // 业务校验:title 非空、facts 非空
		}
		ev.EventType = normalizeEventType(ev.EventType)
		ev.Confidence = clamp01(ev.Confidence)
		valid = append(valid, ev)
		if len(valid) >= 5 {
			break
		}
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("no event passed business validation")
	}
	return valid, nil
}

func normalizeEventType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if model.IsEventType(t) {
		return t
	}
	return model.EventTypeFallback
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func parseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// mustJSONArray 把字符串切片序列化为 **JSON 数组**,nil / 空切片 → 字面量 `[]`。
//
// ⚠️ 不能用裸 `json.Marshal`:它对 nil 切片返回字面量 `null` 且 **err == nil**
// (issue #71 生产事故的根因),于是 events.affected 落成 JSON null,而消费方
// `jsonb_array_elements_text(e.affected)` 遇标量即报 SQLSTATE 22023 →
// entity-build 每轮必崩。故此处**必须先判 nil/空**,不能依赖 err。
func mustJSONArray(v []string) json.RawMessage {
	if len(v) == 0 {
		return json.RawMessage("[]")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("[]")
	}
	return b
}

func strPtrIf(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
