// Package entityextract 实体分类(迭代 3,设计 §3.2):把事件 affected 里未匹配已知实体的词,
// 一次性便宜档分类为 company/industry/concept/topic。复用 internal/extract 的 AI 结构化输出模式。
package entityextract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"piks/internal/ai"
)

// 类型枚举(架构 §9.1 子集 + unknown 诚实落点)。
const (
	TypeCompany  = "company"
	TypeIndustry = "industry"
	TypeConcept  = "concept"
	TypeTopic    = "topic"
)

// 分类输出 Schema(与 internal/extract 同构,validator 复用)。
const JSONSchema = `{"type":"object","properties":{"entities":{"type":"array","maxItems":50,"items":{"type":"object","properties":{
"name":{"type":"string"},
"type":{"type":"string","enum":["company","industry","concept","topic"]},
"aliases":{"type":"array","items":{"type":"string"}}
},"required":["name","type"]}}},"required":["entities"]}`

// ClassifiedEntity 分类结果。
type ClassifiedEntity struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Aliases []string `json:"aliases"`
}

type result struct {
	Entities []ClassifiedEntity `json:"entities"`
}

// Classifier 实体分类器。零状态,线程安全。
type Classifier struct {
	Provider ai.Provider
	Prompt   string
}

// NewClassifier 走便宜档(provider 已绑 AIModelExtract,设计 §4)。
func NewClassifier(provider ai.Provider) *Classifier {
	return &Classifier{Provider: provider, Prompt: systemPrompt}
}

// StripSuffixes 后缀剥离候选:原词 + 依次去掉 板块/概念/指数/股/类 后的形式。
// 用于 affected 词 → 实体名匹配(设计 §3.1 步骤 2;发布器 wikilink 解析同用)。
func StripSuffixes(term string) []string {
	cands := []string{term}
	for _, suf := range []string{"板块", "概念", "指数", "股", "类"} {
		if strings.HasSuffix(term, suf) && len(term) > len(suf) {
			cands = append(cands, strings.TrimSuffix(term, suf))
		}
	}
	return cands
}

// Classify 对一批未匹配词一次性分类。known 为已知实体名(帮模型对齐规范名,可空)。
// 返回分类结果与 token 用量。全部被校验拒绝时返回 error(由调用方兜底为 unknown)。
func (c *Classifier) Classify(ctx context.Context, terms []string, known []string) ([]ClassifiedEntity, int64, error) {
	if len(terms) == 0 {
		return nil, 0, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "需要分类的实体词(%d 个):\n", len(terms))
	for i, t := range terms {
		fmt.Fprintf(&b, "%d. %s\n", i+1, t)
	}
	if len(known) > 0 {
		limit := len(known)
		if limit > 200 {
			limit = 200
		}
		fmt.Fprintf(&b, "\n已存在实体名(供规范名对齐,不是分类目标):%s\n", strings.Join(known[:limit], "、"))
	}

	var resp ai.StructuredResponse
	var lastErr error
	total := int64(0)
	for attempt := 1; attempt <= 3; attempt++ {
		r, err := c.Provider.StructuredOutput(ctx, ai.StructuredRequest{
			System: c.Prompt, User: b.String(), Schema: json.RawMessage(JSONSchema),
		})
		total += r.Usage.Total()
		if err != nil {
			lastErr = err
			continue
		}
		resp = r
		break
	}
	if len(resp.Data) == 0 {
		return nil, total, fmt.Errorf("classify failed after 3 attempts: %w", lastErr)
	}
	var out result
	if err := json.Unmarshal(resp.Data, &out); err != nil {
		return nil, total, fmt.Errorf("classify bad json: %w", err)
	}
	valid := out.Entities[:0]
	for _, e := range out.Entities {
		if strings.TrimSpace(e.Name) == "" {
			continue
		}
		switch e.Type {
		case TypeCompany, TypeIndustry, TypeConcept, TypeTopic:
			valid = append(valid, ClassifiedEntity{Name: strings.TrimSpace(e.Name), Type: e.Type, Aliases: cleanAliases(e.Aliases)})
		}
	}
	if len(valid) == 0 {
		return nil, total, fmt.Errorf("no valid classified entities")
	}
	return valid, total, nil
}

func cleanAliases(as []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, a := range as {
		a = strings.TrimSpace(a)
		if a != "" && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}

// systemPrompt 分类器立场。立场明确:绝不用 industry 指代公司(行业 vs 公司混淆是常见错误)。
const systemPrompt = `你是中国 A 股金融实体分类器。把每个实体词归类为以下四类之一,并给出常见别名。

类型定义:
- company: 公司/上市公司。如"宁德时代""中国平安""比亚迪"。
- industry: 行业/板块。如"银行""白酒""新能源""工业金属"。
- concept: 金融概念/指标/术语。如"LPR""存款准备金率""PE""净息差"。
- topic: 市场题材/热点。如"低空经济""AI算力""固态电池"。题材往往含行业但更主题化。

判断要点:
1. 词带行业后缀(板块/行业/概念/产业)时,规范名去掉后缀后按核心词分类,并把原词记入 aliases。
2. 上市公司名称(有"股份/科技/能源/银行/保险"等公司常见词,或可识别为知名上市公司)归类 company。
3. 宏观/货币/财务术语(利率、准备金、比率、指数类指标)归类 concept。
4. 拿不准时归类 topic,绝不猜成 industry。

只输出 JSON,不要任何解释。schema 已给出。`
