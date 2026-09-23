// Package cluster 事件语义去重聚类(迭代 1,设计 §3.1,D8/D9/D10)。
// 同一真实事件的多条报道 → 一簇,仅 canonical 事件被发布,其余 status='merged'。
// 策略:高置信纯规则直合(标题归一化全同+同类型)+ 中等置信便宜档 LLM 批量确认。
package cluster

import (
	"context"
	"encoding/json"
	"errors"
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

// NormalizeTitle 标题归一化:去掉内嵌标题前缀(【…】)、转小写、去空白与标点、保留汉字与字母数字。
func NormalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(stripBracketWrap(s))) {
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

// stripBracketWrap 抽取【…】内嵌标题前缀(跨源标题归一化的关键,issue #48 T2)。
//
// 各家对同一件事的标题写法不同:东财/新浪把**规范标题**放在【】里、后接公告/播报正文,
// 而财联社/金十/同花顺/富途直接给裸标题。原句全文比对会让相似度坍塌 —— 实测(2026-09-20,
// dev 库 163 条 6 源真实标题):东财「【天山生物：持股5%以上股东拟减持不超3%股份】天山生物
// (300313.SZ)公告称,…」与财联社「天山生物：持股5%以上股东拟减持不超3%股份」**同一事件**,
// 但全文 Jaccard 仅 **0.165**(东财正文段把【】里的标题稀释掉了),远低于 0.7 阈值 → 漏合并。
//
// 规则(【】内文字≥6 字才当标题,否则视为栏目标签、原样保留):
//   - 【标题】正文  → 取【】内(东财/新浪:规范标题在前 + 公告正文)
//   - 标题【正文】  → 取【】外(少数源:标题在前 + 括号内正文)
//   - 无【】、【】未闭合、或【】内不足 6 字 → 原样返回(不猜测)
//
// 为何用「长度」判据:实测两处栏目标签行(【电报解读】4 字、【风口研报·公司】7 字)在**任一
// 规则下**都不与其余 161 行产生 ≥0.5 的相似对(加固后实测最高仅 0.040 / 0.059),故抽取与否
// 都不影响判定;取长度判据是因为它只依赖本行内容,不需要「括号内外是否同源」这类跨段推断。
//
// 实测效果:0.7 阈值下真实重复对 36 → 65(基线 = 未加固的归一化,同一批 163 条标题),
// **新增对逐条人工核对均为同一真实事件**(天山生物/新华制药/立讯精密/良品铺子/盛和资源/
// 四方光电/埃夫特/永信至诚/深水海纳/日本地震/西班牙火灾 等),见设计文档校准表。
func stripBracketWrap(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "【")
	if start < 0 {
		return s
	}
	rest := s[start+len("【"):]
	end := strings.Index(rest, "】")
	if end < 0 {
		return s
	}
	inner := rest[:end]
	if len([]rune(inner)) < bracketTitleMinRunes {
		return s
	}
	if start == 0 {
		return inner // 【标题】正文 → 标题
	}
	return s[:start] // 标题【正文】 → 标题
}

// bracketTitleMinRunes 判定「【】内是标题而非栏目标签」所需的最小字数(按字符计)。
const bracketTitleMinRunes = 6

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

// entityOverlap 实体交集:任一方向包含(较短者≥2 字符)即视为重叠。
// 真实 provider 抽取的实体措辞有方差("银行" vs "银行板块"),精确相等会漏掉重复事件对(#17 真实验证暴露)。
// pre-filter 只求召回,精确判定交给 LLM 确认;中文实体名互相包含通常意味着同属一个主体,误报成本低。
func entityOverlap(a, b json.RawMessage) bool {
	var x, y []string
	_ = json.Unmarshal(a, &x)
	_ = json.Unmarshal(b, &y)
	for _, s := range x {
		if len(s) < 2 {
			continue
		}
		for _, t := range y {
			if len(t) < 2 {
				continue
			}
			if strings.Contains(s, t) || strings.Contains(t, s) {
				return true
			}
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
// 拆成 autoGroups / llmPairs 两个纯函数,便于离线校准跨源阈值(issue #48 T2)—— 校准要能
// 直接对「给定一批事件,哪些对会进 LLM 确认」取证,而不是只看最终合并结果。
func GenCandidates(events []model.Event) Candidate {
	auto, autoIdx := autoGroups(events)
	return Candidate{Auto: auto, LLM: llmPairs(events, autoIdx)}
}

// autoGroups 高置信:归一化标题全同 + 同类型 → 直合。
func autoGroups(events []model.Event) ([][]int, map[int]bool) {
	byKey := make(map[string][]int)
	for i := range events {
		key := NormalizeTitle(events[i].Title) + "\x00" + events[i].EventType
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
	return auto, autoIdx
}

// llmPairs 中等置信候选对:同类型 + (标题 Jaccard≥0.7 或 (实体交集≥1 且 occurred_at≤3天))。
//
// ⚠️ 阈值 0.7 **不随多源下调**(issue #48 红线「扩候选池不降门槛」)。跨源差异的修法在
// NormalizeTitle 的内嵌前缀抽取(把因措辞差异坍塌的相似度还原),不在放宽门槛。
//
// 实测(2026-09-20,163 条 6 源真实标题,见 docs/phase11/design/event-cross-source.md):
// 抽取前缀后 0.7 阈值下的真实重复对 36 → 65,**同一批标题在 0.6 阈值下也只是 51 → 80** ——
// 降门槛并不增加「只有降门槛才能捞到」的召回,反而把候选对整体推高、白花 LLM token。
// 判别边界(同股不同批次药品注册证书 = 不同事件)实测 Jaccard ≤0.515,距 0.7 有 0.185 余量。
// ⚠️ 注意 Jaccard 只是两个 OR 分支之一,实体交集分支不受本阈值影响。
func llmPairs(events []model.Event, autoIdx map[int]bool) [][]int {
	norms := make([]string, len(events))
	big := make([]map[string]struct{}, len(events))
	for i := range events {
		norms[i] = NormalizeTitle(events[i].Title)
		big[i] = Bigrams(norms[i])
	}
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
	return pairs
}

// ---------- LLM 批量确认 ----------

// PairVerdict 一对事件的 LLM 判定。
//
// ⚠️ **无 `CanonicalTitle` 字段**(issue #83 P-3):旧实现让 LLM 在确认同事件时顺带重写一个
// 规范标题,再由 `canonicalTitle` 取「第一个命中 pair」。该标题措辞在不同 pair 间可能发散、
// 且是 Inference 而非 Fact ⇒ P3 改为**无 LLM 规则**选簇标题(成员原文里重合度最高者),
// 本字段随之成为死字段,一并删除(prompt 少要一个字段,省一点输出 token)。
type PairVerdict struct {
	PairIndex int
	IsSame    bool
}

const batchSystem = `你是事件去重确认助手。判断每一对事件是否描述同一件真实发生的事(同一主体 + 同一行为 + 同一时间范围)。
严格输出单个 JSON 对象,不要输出任何其他文字、注释或 Markdown 标记:
{"results":[{"pair_index":0,"is_same":true}, ...]}
规则:is_same 为 true 表示同一事件,false 表示不同事件。`

// retryBackoff 重试退避基数(issue #75)。包级变量便于测试注入(设为 0 即不等待)。
// 退避序列:base×1、base×2(base=0 时不等待)。加抖动由 jitter 控制。
var retryBackoff = time.Second

// confirmAttempts 单批确认的最大尝试次数(失败退避后重试)。
const confirmAttempts = 3

// backoff 等待第 attempt 次失败后的退避时长(1s、2s…),并响应 ctx 取消。
// attempt 从 1 起:第 1 次失败后等 base×1,第 2 次后等 base×2。
func backoff(ctx context.Context, attempt int) {
	d := retryBackoff * time.Duration(attempt)
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// ConfirmPairs 分批送 LLM 确认候选对,返回与 pairs 同序的判定。
// maxTokens>0 时作为本命令可用 token 上限,超出即停止确认(剩余对视为不同事件)。
//
// 重试语义(issue #75):
//   - **429/5xx 可重试** —— 每次失败后按退避(base×attempt)等待再试,不再瞬间烧完 3 次;
//   - **其它 4xx 确定性** —— 立即放弃(重试无意义,只是白花时间与配额);
//   - ⚠️ **只在成功时累加 token**(修账本盲区):旧实现 `total += r.Usage.Total()` 对
//     报错的那次也加,而网关抽风时 `Usage` 恒为 0、真实花费记成 0,同时把「失败次数」
//     混进花费口径。失败不再计入。
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
		for attempt := 1; attempt <= confirmAttempts; attempt++ {
			r, err := p.StructuredOutput(ctx, ai.StructuredRequest{System: batchSystem, User: user})
			if err != nil {
				lastErr = err
				var apiErr *ai.APIError
				if errors.As(err, &apiErr) && !apiErr.Retryable() {
					return nil, total, fmt.Errorf("cluster confirm deterministic failure (status %d): %w", apiErr.Status, err)
				}
				if attempt < confirmAttempts {
					backoff(ctx, attempt)
				}
				continue
			}
			total += r.Usage.Total() // 只在成功时计入
			resp = r
			break
		}
		if len(resp.Data) == 0 {
			return nil, total, fmt.Errorf("cluster confirm failed after %d attempts: %w", confirmAttempts, lastErr)
		}
		var out struct {
			Results []struct {
				PairIndex int  `json:"pair_index"`
				IsSame    bool `json:"is_same"`
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

// UnmatchedIndices 返回「本轮进了池、最终不属于任何分量」的事件下标(issue #75)。
// 这些事件本轮确实被比对过、且无对端 ⇒ 调用方据此盖扫描水位(cluster_scanned_at),
// 下轮不再进池。⚠️ 只在**未截断**时可用:limit 截断时池外事件没被比对过,
// 给它们盖水位会永久漏召回(见 cmd/cluster 的 truncated 守卫)。
func UnmatchedIndices(n int, comps [][]int) []int {
	inComp := make([]bool, n)
	for _, c := range comps {
		for _, i := range c {
			inComp[i] = true
		}
	}
	var out []int
	for i := 0; i < n; i++ {
		if !inComp[i] {
			out = append(out, i)
		}
	}
	return out
}

// memberMeta 分量内成员的**建簇期元数据**(issue #83 P6),与事件下标平行。
//
// 为何要单独传:`ApplyClusters` 拿到的 `[]model.Event` 只有 `RawDocumentID` 外键,没有 url/content;
// 而 P6 代表选取规则「有直接链接 > 无链接 → 来源独立性强(非转载) > 弱 → 首发时间最早」的前两级
// 依赖这两样。调用方在建簇前一次性从 raw 层取回(`store.ListEventDocMeta` + `ReprintFlags` 派生)。
type memberMeta struct {
	hasURL    bool // raw 文档有直接外链(rd.url 非空)—— 无链接源照样列出,但优先选有链接的当代表
	isReprint bool // 与分量内另一成员正文近逐字(转载,P-1 指纹) —— 独立性弱,不当代表
}

// ApplyClusters 建簇入库:canonical 按 P6 规则选(有链接 > 非转载 > 最早,同则高置信),
// 其余 status='merged'。返回被合并(merged)事件数。
//
// canonical 的**事件 id**(issue #83 P-2)随簇同一次 INSERT 落 `event_clusters.canonical_event_id`
// —— 它是簇的详情入口(P8 / /events/:id)。选定后**冻结**:reexamine 把别的簇并入本簇时不重选。
//
// ⚠️ **存量与新簇口径分叉**(issue #83 P-3 决定):本函数用 **P6 新规则**选新簇代表;存量簇的
// `canonical_event_id` **不回填**(保「代表冻结」纪律,避免 `based_on` 决策边随重算漂移)。
// 故迁移 `0021` 回填 SQL(旧口径「最早非 merged」)与新簇选取**有意不再逐字一致** ——
// 见 `canonicalIndex` 注释与 `docs/phase11/design/event-pipeline-p3.md`。
func ApplyClusters(ctx context.Context, s *store.Store, events []model.Event, comps [][]int) (int, error) {
	// 建簇前一次性取回多成员分量各成员的 (url, content),派生 P6 代表选取所需的 memberMeta。
	// 单成员分量代表恒为自己,不必取数。
	meta, err := loadMemberMeta(ctx, s, events, comps)
	if err != nil {
		return 0, err
	}

	merged := 0
	for _, comp := range comps {
		title := canonicalTitle(events, comp)
		canonical := canonicalIndex(events, comp, meta)
		canonicalID := events[canonical].ID
		cid, err := s.CreateEventCluster(ctx, &model.EventCluster{
			Title:            title,
			CanonicalEventID: &canonicalID,
		})
		if err != nil {
			return merged, fmt.Errorf("create cluster: %w", err)
		}
		// 除 canonical 外的分量成员:按「最早创建,同则更高置信」排序并入(顺序只影响 merged 的写入次序)。
		sorted := append([]int(nil), comp...)
		sort.Slice(sorted, func(a, b int) bool {
			if events[sorted[a]].CreatedAt.Equal(events[sorted[b]].CreatedAt) {
				return events[sorted[a]].Confidence > events[sorted[b]].Confidence
			}
			return events[sorted[a]].CreatedAt.Before(events[sorted[b]].CreatedAt)
		})
		for _, idx := range sorted {
			if idx == canonical {
				continue
			}
			if err := s.SetEventCluster(ctx, events[idx].ID, cid, "merged"); err != nil {
				return merged, fmt.Errorf("merge member: %w", err)
			}
			merged++
		}
		// canonical 保留原状态(extracted/verified/published)并入簇。
		// 用 NoTouch:仅设 cluster_id,不动 status/updated_at。
		// 未发布 canonical 靠 published_at IS NULL 被发布查询选中;已发布 canonical 因 updated_at 未变不会被重选,
		// 避免聚类引发无谓卡片重写与 git 噪音。
		if err := s.SetEventClusterNoTouch(ctx, events[canonical].ID, cid); err != nil {
			return merged, fmt.Errorf("set canonical: %w", err)
		}
	}
	return merged, nil
}

// loadMemberMeta 取分量内各成员的建簇期元数据(P6 代表选取用)。
//
// 只对**多成员分量**的成员取数:`hasURL` 来自 raw 文档 url;`isReprint` 由分量内各成员**正文**
// 经 P-1 指纹分组派生(`ReprintFlags`,原发下标传 -1 ⇒ 组内最小下标为原发,确定性)。
// 单成员分量不必取(代表恒为自己)。
func loadMemberMeta(ctx context.Context, s *store.Store, events []model.Event, comps [][]int) ([]memberMeta, error) {
	meta := make([]memberMeta, len(events))
	ids := make([]string, 0, len(events))
	seen := make(map[int]bool, len(events))
	for _, comp := range comps {
		if len(comp) < 2 {
			continue
		}
		for _, i := range comp {
			if !seen[i] {
				seen[i] = true
				ids = append(ids, events[i].ID)
			}
		}
	}
	if len(ids) == 0 {
		return meta, nil
	}
	docs, err := s.ListEventDocMeta(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list event doc meta: %w", err)
	}
	for i := range events {
		if m, ok := docs[events[i].ID]; ok {
			meta[i].hasURL = m.URL != nil && *m.URL != ""
		}
	}
	// isReprint 按**每个分量**就地判定(转载是分量相对属性,跨分量无意义)。
	for _, comp := range comps {
		if len(comp) < 2 {
			continue
		}
		contents := make([]string, len(comp))
		for k, i := range comp {
			if m, ok := docs[events[i].ID]; ok && m.Content != nil {
				contents[k] = *m.Content
			}
		}
		flags := ReprintFlags(contents, -1)
		for k, i := range comp {
			meta[i].isReprint = flags[k]
		}
	}
	return meta, nil
}

// canonicalIndex 返回分量 comp 内代表(canonical)的下标(issue #83 P6 规则):
//
//	有直接链接 > 无链接 → 来源独立性强(非转载) > 弱 → 首发时间最早(同则更高置信)
//
// ⚠️ **不用「渠道数」选代表** —— 那是**簇级**属性,不是成员级(P6 红线)。
//
// ⚠️ **与迁移 `0021_event_pipeline_p2.sql` 回填 SQL 有意分叉**(issue #83 P-3):迁移回填用旧口径
// 「最早非 merged」,**存量簇冻结不回填**;本函数是**新簇**口径。二者不再逐字一致是**已批准的
// 设计决定**(见 `ApplyClusters` 注释与 `docs/phase11/design/event-pipeline-p3.md`),不是漂移。
// 单测 `canonical_test.go` 锁死本函数;`betterCanonical` 的比较次序即规则的实现。
func canonicalIndex(events []model.Event, comp []int, meta []memberMeta) int {
	best := comp[0]
	for _, i := range comp[1:] {
		if betterCanonical(events, meta, i, best) {
			best = i
		}
	}
	return best
}

// betterCanonical 报告候选 a 是否比当前最优 b 更适合当簇代表(P6 规则次序):
// ① 有链接胜无链接;② 非转载胜转载;③ 创建更早;④ 置信更高;⑤ 全等则保留 b(不替换 ⇒ 确定性)。
func betterCanonical(events []model.Event, meta []memberMeta, a, b int) bool {
	if meta[a].hasURL != meta[b].hasURL {
		return meta[a].hasURL
	}
	if meta[a].isReprint != meta[b].isReprint {
		return !meta[a].isReprint
	}
	if !events[a].CreatedAt.Equal(events[b].CreatedAt) {
		return events[a].CreatedAt.Before(events[b].CreatedAt)
	}
	return events[a].Confidence > events[b].Confidence
}

// canonicalTitle 取分量内**与其他成员标题重合度最高**的成员**原文标题**(issue #83 P6)。
//
// 🔴 **无 LLM**(issue #83 P-3 决定):旧实现取「第一个命中 pair 的 LLM 重写标题」,措辞在不同
// pair 间可能发散、且标题是 Inference 而非 Fact。P3 改为**确定性规则**:对每个成员算它与其余成员
// 归一化标题的 Jaccard 之和,取最大者的**原文标题** —— 它是**成员自己的话**(Fact)、可复现、零 token,
// 契合「系统只做归因」与 #86 减量方向。
//
// 平票 → 创建最早;再平 → 分量内先到者(不替换 ⇒ 确定性)。复现 `NormalizeTitle`/`Bigrams`/`Jaccard`
// (与聚类同一把尺子)。重合度全为 0(措辞毫无共性)时退化为「最早一条」,与旧回退一致。
func canonicalTitle(events []model.Event, comp []int) string {
	best := comp[0]
	bestScore := -1.0
	for _, i := range comp {
		ti := Bigrams(NormalizeTitle(events[i].Title))
		score := 0.0
		for _, j := range comp {
			if j == i {
				continue
			}
			score += Jaccard(ti, Bigrams(NormalizeTitle(events[j].Title)))
		}
		switch {
		case score > bestScore:
			best, bestScore = i, score
		case score == bestScore && events[i].CreatedAt.Before(events[best].CreatedAt):
			// 平票取更早创建者;仍平则保留先到者(comp 顺序确定 ⇒ 确定性)。
			best = i
		}
	}
	return events[best].Title
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
