// Package cluster 事件语义去重聚类(迭代 1,设计 §3.1,D8/D9/D10)。
// 同一真实事件的多条报道 → 一簇,仅 canonical 事件被发布,其余 status='merged'。
// 策略:高置信纯规则直合(标题归一化全同+同类型)+ 中等置信便宜档 LLM 批量确认。
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

// ---------- 规则与相似度 ----------

// NormalizeTitle 标题归一化:转小写、去空白与标点、保留汉字与字母数字。
func NormalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case unicode.IsSpace(r):
			continue
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case unicode.Is(unicode.Han, r):
			b.WriteRune(r)
		default: // 标点等跳过
		}
	}
	return b.String()
}

// Bigrams 字符二元组集合(中文相似度基础)。
func Bigrams(norm string) map[string]struct{} {
	m := make(map[string]struct{})
	rs := []rune(norm)
	for i := 0; i+1 < len(rs); i++ {
		m[string(rs[i:i+2])] = struct{}{}
	}
	if len(rs) == 1 {
		m[norm] = struct{}{}
	}
	return m
}

// Jaccard 二元组集合 Jaccard 相似度。
func Jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter, union := 0, len(a)
	for k := range b {
		if _, ok := a[k]; ok {
			inter++
		} else {
			union++
		}
	}
	return float64(inter) / float64(union)
}

func entityOverlap(a, b json.RawMessage) bool {
	var x, y []string
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	set := make(map[string]struct{}, len(x))
	for _, s := range x {
		set[s] = struct{}{}
	}
	for _, s := range y {
		if _, ok := set[s]; ok {
			return true
		}
	}
	return false
}

func withinDays(a, b *time.Time, days int) bool {
	if a == nil || b == nil {
		return false
	}
	d := a.Sub(*b)
	if d < 0 {
		d = -d
	}
	return d <= time.Duration(days)*24*time.Hour
}

// ---------- 候选生成 ----------

// Candidate 聚类候选:规则直合组件 + 需 LLM 确认的对。
type Candidate struct {
	Auto [][]int // 规则直合组件(事件索引组)
	LLM  [][]int // 中等置信候选对(事件索引对)
}

// GenCandidates 生成候选。Auto 组内的事件不再进 LLM。
func GenCandidates(events []model.Event) Candidate {
	norms := make([]string, len(events))
	big := make([]map[string]struct{}, len(events))
	for i := range events {
		norms[i] = NormalizeTitle(events[i].Title)
		big[i] = Bigrams(norms[i])
	}

	// 高置信:归一化标题全同 + 同类型 → 直合
	byKey := make(map[string][]int)
	for i := range events {
		key := norms[i] + "\x00" + events[i].EventType
		byKey[key] = append(byKey[key], i)
	}
	var auto [][]int
	autoIdx := make(map[int]bool)
	for _, g := range byKey {
		if len(g) >= 2 {
			auto = append(auto, g)
			for _, i := range g {
				autoIdx[i] = true
			}
		}
	}

	// 中等置信:标题 Jaccard≥0.7 或(实体交集≥1 且 occurred_at≤3天),且同类型
	var pool []int
	for i := range events {
		if !autoIdx[i] {
			pool = append(pool, i)
		}
	}
	var pairs [][]int
	for x := 0; x < len(pool); x++ {
		for y := x + 1; y < len(pool); y++ {
			i, j := pool[x], pool[y]
			if events[i].EventType != events[j].EventType {
				continue
			}
			if Jaccard(big[i], big[j]) >= 0.7 ||
				(entityOverlap(events[i].Affected, events[j].Affected) && withinDays(events[i].OccurredAt, events[j].OccurredAt, 3)) {
				pairs = append(pairs, []int{i, j})
			}
		}
	}
	return Candidate{Auto: auto, LLM: pairs}
}

// ---------- LLM 批量确认 ----------

// PairVerdict 一对事件的 LLM 判定。
type PairVerdict struct {
	PairIndex      int
	IsSame         bool
	CanonicalTitle string
}

const batchSystem = `你是事件去重确认助手。判断每一对事件是否描述同一件真实发生的事(同一主体 + 同一行为 + 同一时间范围)。
严格输出单个 JSON 对象,不要输出任何其他文字、注释或 Markdown 标记:
{"results":[{"pair_index":0,"is_same":true,"canonical_title":"更规范的事件标题"}, ...]}
规则:is_same=false 时 canonical_title 填空字符串;canonical_title 取覆盖面最广、最规范的那个标题。`

// ConfirmPairs 分批送 LLM 确认候选对,返回与 pairs 同序的判定。
// maxTokens>0 时作为本命令可用 token 上限,超出即停止确认(剩余对视为不同事件)。
func ConfirmPairs(ctx context.Context, p ai.Provider, events []model.Event, pairs [][]int, batch int, maxTokens int64) ([]PairVerdict, int64, error) {
	verdicts := make([]PairVerdict, len(pairs))
	for i := range verdicts {
		verdicts[i].PairIndex = i
	}
	if len(pairs) == 0 {
		return verdicts, 0, nil
	}
	total := int64(0)
	for start := 0; start < len(pairs); start += batch {
		if maxTokens > 0 && total >= maxTokens {
			break // 预算护栏:停止确认,剩余对保持"不同事件"
		}
		end := min(start+batch, len(pairs))
		user := buildPairPrompt(events, pairs[start:end], start)

		var resp ai.StructuredResponse
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			r, err := p.StructuredOutput(ctx, ai.StructuredRequest{System: batchSystem, User: user})
			total += r.Usage.Total()
			if err != nil {
				lastErr = err
				continue
			}
			resp = r
			break
		}
		if len(resp.Data) == 0 {
			return nil, total, fmt.Errorf("cluster confirm failed after 3 attempts: %w", lastErr)
		}
		var out struct {
			Results []struct {
				PairIndex      int    `json:"pair_index"`
				IsSame         bool   `json:"is_same"`
				CanonicalTitle string `json:"canonical_title"`
			} `json:"results"`
		}
		if err := json.Unmarshal(resp.Data, &out); err != nil {
			return nil, total, fmt.Errorf("cluster confirm invalid json: %w", err)
		}
		for _, r := range out.Results {
			if r.PairIndex < 0 || r.PairIndex >= len(verdicts) {
				continue
			}
			verdicts[r.PairIndex].IsSame = r.IsSame
			verdicts[r.PairIndex].CanonicalTitle = r.CanonicalTitle
		}
	}
	return verdicts, total, nil
}

func buildPairPrompt(events []model.Event, pairs [][]int, startOffset int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "待确认事件对(共 %d 对):\n", len(pairs))
	for k, pr := range pairs {
		a, c := events[pr[0]], events[pr[1]]
		fmt.Fprintf(&b, "#%d: 事件A: %s (类型:%s) | 事件B: %s (类型:%s)\n",
			startOffset+k, a.Title, a.EventType, c.Title, c.EventType)
	}
	return b.String()
}

// ---------- 应用 ----------

type uf struct{ p []int }

func newUF(n int) *uf {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &uf{p: p}
}
func (u *uf) find(x int) int {
	for u.p[x] != x {
		u.p[x] = u.p[u.p[x]]
		x = u.p[x]
	}
	return x
}
func (u *uf) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.p[rb] = ra
	}
}

// BuildComponents 由规则直合 + LLM 确认结果构建连通分量(每分量大小≥2 即一簇)。
func BuildComponents(n int, auto [][]int, verdicts []PairVerdict, pairs [][]int) [][]int {
	u := newUF(n)
	for _, g := range auto {
		for i := 1; i < len(g); i++ {
			u.union(g[0], g[i])
		}
	}
	for i, p := range pairs {
		if verdicts[i].IsSame {
			u.union(p[0], p[1])
		}
	}
	roots := make(map[int][]int)
	for i := 0; i < n; i++ {
		r := u.find(i)
		roots[r] = append(roots[r], i)
	}
	var comps [][]int
	for _, m := range roots {
		if len(m) >= 2 {
			comps = append(comps, m)
		}
	}
	return comps
}

// ApplyClusters 建簇入库:canonical = 最早创建(同则更高置信),其余 status='merged'。
// 返回被合并(merged)事件数。
func ApplyClusters(ctx context.Context, s *store.Store, events []model.Event, comps [][]int, verdicts []PairVerdict, pairs [][]int) (int, error) {
	merged := 0
	for _, comp := range comps {
		title := canonicalTitle(events, verdicts, pairs, comp)
		cid, err := s.CreateEventCluster(ctx, &model.EventCluster{Title: title})
		if err != nil {
			return merged, fmt.Errorf("create cluster: %w", err)
		}
		sorted := append([]int(nil), comp...)
		sort.Slice(sorted, func(a, b int) bool {
			if events[sorted[a]].CreatedAt.Equal(events[sorted[b]].CreatedAt) {
				return events[sorted[a]].Confidence > events[sorted[b]].Confidence
			}
			return events[sorted[a]].CreatedAt.Before(events[sorted[b]].CreatedAt)
		})
		canonical := sorted[0]
		for _, idx := range sorted[1:] {
			if err := s.SetEventCluster(ctx, events[idx].ID, cid, "merged"); err != nil {
				return merged, fmt.Errorf("merge member: %w", err)
			}
			merged++
		}
		// canonical 保留原状态(extracted/verified/published),并入簇,updated_at 变化触发增量发布
		if err := s.SetEventCluster(ctx, events[canonical].ID, cid, events[canonical].Status); err != nil {
			return merged, fmt.Errorf("set canonical: %w", err)
		}
	}
	return merged, nil
}

// canonicalTitle 优先取 LLM 确认的规范标题(对端都在该分量内),否则取最早事件标题。
func canonicalTitle(events []model.Event, verdicts []PairVerdict, pairs [][]int, comp []int) string {
	inComp := make(map[int]bool, len(comp))
	for _, i := range comp {
		inComp[i] = true
	}
	for i, v := range verdicts {
		if v.IsSame && v.CanonicalTitle != "" && inComp[pairs[i][0]] && inComp[pairs[i][1]] {
			return v.CanonicalTitle
		}
	}
	earliest := comp[0]
	for _, i := range comp[1:] {
		if events[i].CreatedAt.Before(events[earliest].CreatedAt) {
			earliest = i
		}
	}
	return events[earliest].Title
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
