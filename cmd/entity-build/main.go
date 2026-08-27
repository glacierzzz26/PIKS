// entity-build 实体构建命令(迭代 3,设计 §3.1):种子源 zt_pool(零 AI)→ Company/Industry + belongs_to;
// 事件 affected 收割 → 名称匹配(含后缀剥离)/便宜档批量分类 → affects;未命中 → unknown 诚实落点。
// 幂等:全量派生,重跑零新增、零 DB churn(aliases/detail 全等即跳过写)。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"piks/internal/ai"
	"piks/internal/collector"
	"piks/internal/config"
	"piks/internal/entityextract"
	"piks/internal/model"
	"piks/internal/store"
)

// 实体类型常量(与 internal/entityextract 一致)。
const (
	TypeCompany  = "company"
	TypeIndustry = "industry"
	TypeConcept  = "concept"
	TypeTopic    = "topic"
	TypeUnknown  = "unknown"
)

// entityKey 内存工作计划的实体主键((type,name) 对应 DB UNIQUE)。
type entityKey struct {
	Type string
	Name string
}

// work 单次运行的内存工作计划:先收集全部贡献(别名/关系),再一次性 upsert,
// 避免"种子写别名 → 收割又改回"的来回 churn。
type work struct {
	// 需保证存在的实体 → 要合并进的别名 + detail(首贡献者定 detail,后贡献覆盖若不同由 UpsertEntity 去 churn)
	aliasAdds map[entityKey]map[string]bool
	detail    map[entityKey]json.RawMessage
	// affects:实体 ← 事件(term 为原始 affected 词,入 properties 供血缘)
	affects map[entityKey]map[string]string // eventID → term
	// belongs_to:company → industry(种子)
	belongs map[string]string // companyName → hybk
}

func main() {
	flag.Parse()
	cfg := config.Load()
	ctx := context.Background()

	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)
	// AI 配置权威源 = 数据库 app_config(不再读 PIKS_AI_* 环境变量)。
	if err := s.ApplyAppConfig(ctx, &cfg); err != nil {
		fatal("apply app config:", err)
	}

	runID, err := s.StartTaskRun(ctx, "entity-build")
	if err != nil {
		fatal("start task run:", err)
	}
	fail := func(err error) {
		_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
		fatal("entity-build:", err)
	}

	// 现有实体 → in-memory 索引(已存在即复用,保证规范名/别名累积合并)
	existing, err := s.ListAllEntities(ctx)
	if err != nil {
		fail(err)
	}
	index := buildEntityIndex(existing)

	// 1. 种子源(零 AI):近 30 交易日涨停池 → Company + Industry + belongs_to
	seeds := harvestSeeds(ctx, s, 30)
	w := &work{
		aliasAdds: map[entityKey]map[string]bool{},
		detail:    map[entityKey]json.RawMessage{},
		affects:   map[entityKey]map[string]string{},
		belongs:   map[string]string{},
	}
	for _, c := range seeds.companies {
		key := entityKey{TypeCompany, c.Name}
		w.aliasAdds[key] = map[string]bool{}
		w.detail[key] = json.RawMessage(fmt.Sprintf(`{"code":%q}`, c.Code))
		if c.Hybk != "" {
			w.belongs[c.Name] = c.Hybk
			ik := entityKey{TypeIndustry, c.Hybk}
			w.aliasAdds[ik] = map[string]bool{c.Hybk + "板块": true} // 东财 hybk 常见措辞变体
			w.detail[ik] = json.RawMessage(`{"source":"eastmoney"}`)
		}
	}

	// 2. 事件 affected 收割:词 → 事件 id 映射
	termEvents, err := s.ListAffectedTermEvents(ctx)
	if err != nil {
		fail(err)
	}
	terms := make([]string, 0, len(termEvents))
	for t := range termEvents {
		terms = append(terms, t)
	}
	sort.Strings(terms) // 确定性:分类输入/迭代顺序稳定

	var matched, unmatched []string
	for _, t := range terms {
		if ent := matchEntity(index, t); ent != nil {
			matched = append(matched, t)
			key := entityKey{ent.Type, ent.Name}
			if w.aliasAdds[key] == nil {
				w.aliasAdds[key] = map[string]bool{}
			}
			w.aliasAdds[key][t] = true // 原词补进别名(发布 wikilink 解析用)
			addAffects(w, key, termEvents[t], t)
		} else {
			unmatched = append(unmatched, t)
		}
	}

	// 3. AI 批量分类(便宜档,一次);护栏:超日预算则跳过,未匹配词全部 unknown(诚实)
	classifiedTerms := 0
	var unknownTerms []string
	if len(unmatched) > 0 && aiBudgetAvailable(ctx, s, cfg.AIDailyTokenBudget) {
		cl := entityextract.NewClassifier(newProvider(cfg))
		known := knownNames(index)
		cls, tokens, err := cl.Classify(ctx, unmatched, known)
		if err == nil {
			// 分类实体 → 原词反向映射(名称/别名精确或后缀剥离命中即归位;否则该词 unknown)
			clsIndex := classifyIndex(cls)
			for _, t := range unmatched {
				if e := matchClassified(clsIndex, t); e != nil {
					classifiedTerms++
					key := entityKey{e.Type, e.Name}
					if w.aliasAdds[key] == nil {
						w.aliasAdds[key] = map[string]bool{}
					}
					for _, a := range e.Aliases {
						w.aliasAdds[key][a] = true
					}
					w.aliasAdds[key][t] = true // 原词必补
					addAffects(w, key, termEvents[t], t)
				} else {
					unknownTerms = append(unknownTerms, t)
				}
			}
			// detail:分类来的概念/题材留空;company 无代码也留空(架构 §9.3 Unknown 允许)
			_ = tokens
		} else {
			unknownTerms = append(unknownTerms, unmatched...)
		}
	} else if len(unmatched) > 0 {
		unknownTerms = append(unknownTerms, unmatched...)
	}
	sort.Strings(unknownTerms)

	// 4. 执行:一次性 upsert 全部实体 → belongs_to → affects
	created := 0
	for _, key := range sortedKeys(w) {
		id, isNew, err := upsertEntity(ctx, s, index, key, w)
		if err != nil {
			fail(err)
		}
		if isNew {
			created++
		}
		_ = id
	}
	for _, c := range seeds.companies {
		if c.Hybk == "" {
			continue
		}
		from, ok := index[entityKey{TypeCompany, c.Name}]
		if !ok {
			continue
		}
		to, ok := index[entityKey{TypeIndustry, c.Hybk}]
		if !ok {
			continue
		}
		if err := s.CreateRelationship(ctx, &model.Relationship{
			FromType:   "entity", FromID: from.ID,
			ToType:     "entity", ToID: to.ID,
			RelType:    "belongs_to",
			Properties: json.RawMessage("{}"),
		}); err != nil {
			fail(err)
		}
	}
	relCount := 0
	for _, key := range sortedKeys(w) {
		ent, ok := index[key]
		if !ok {
			continue
		}
		for eventID := range w.affects[key] {
			if err := s.CreateRelationship(ctx, &model.Relationship{
				FromType: "event", FromID: eventID,
				ToType:   "entity", ToID: ent.ID,
				RelType:  "affects",
				Properties: json.RawMessage(fmt.Sprintf(`{"term":%q}`, w.affects[key][eventID])),
			}); err != nil {
				fail(err)
			}
			relCount++
		}
	}

	meta := map[string]any{
		"seed_companies":  len(seeds.companies),
		"seed_industries": len(seeds.industries),
		"affected_terms":  len(terms),
		"matched":         len(matched),
		"classified":      classifiedTerms,
		"unknown":         len(unknownTerms),
		"unknown_names":   unknownTerms,
		"entities_created": created,
		"affects_rels":     relCount,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fail(err)
	}
	fmt.Printf("entity-build: seed=%d/%d affected=%d matched=%d classified=%d unknown=%d created=%d affects=%d\n",
		len(seeds.companies), len(seeds.industries), len(terms), len(matched), classifiedTerms, len(unknownTerms), created, relCount)
}

// seedResult 种子收割结果(按名排序,确定性)。
type seedResult struct {
	companies  []seedCompany
	industries []string
}

type seedCompany struct {
	Code string
	Name string
	Hybk string
}

// harvestSeeds 读最近 n 个交易日的 zt_pool,按 code 去重合并。
func harvestSeeds(ctx context.Context, s *store.Store, n int) seedResult {
	snaps, err := s.ListMarketSnapshots(ctx, n)
	if err != nil {
		return seedResult{}
	}
	byCode := map[string]collector.ZTItem{}
	for _, snap := range snaps {
		var pool []collector.ZTItem
		if err := json.Unmarshal(snap.ZTPool, &pool); err != nil {
			continue
		}
		for _, it := range pool {
			if it.Code == "" || it.Name == "" {
				continue
			}
			// 同一 code 若先前缺 hybk,本次补上(近 30 日合并去重)
			if prev, ok := byCode[it.Code]; !ok || (prev.Hybk == "" && it.Hybk != "") {
				byCode[it.Code] = it
			}
		}
	}
	var out seedResult
	for _, it := range byCode {
		out.companies = append(out.companies, seedCompany{it.Code, it.Name, it.Hybk})
		if it.Hybk != "" {
			out.industries = append(out.industries, it.Hybk)
		}
	}
	sort.Slice(out.companies, func(i, j int) bool {
		if out.companies[i].Name != out.companies[j].Name {
			return out.companies[i].Name < out.companies[j].Name
		}
		return out.companies[i].Code < out.companies[j].Code
	})
	seen := map[string]bool{}
	uniq := out.industries[:0]
	for _, h := range out.industries {
		if !seen[h] {
			seen[h] = true
			uniq = append(uniq, h)
		}
	}
	out.industries = uniq
	sort.Strings(out.industries)
	return out
}

// buildEntityIndex (type,name) → 实体。含 name + 全部别名精确键。
func buildEntityIndex(list []model.Entity) map[entityKey]*model.Entity {
	idx := map[entityKey]*model.Entity{}
	for i := range list {
		e := &list[i]
		idx[entityKey{e.Type, e.Name}] = e
		var as []string
		if json.Unmarshal(e.Aliases, &as) == nil {
			for _, a := range as {
				idx[entityKey{e.Type, a}] = e
			}
		}
	}
	return idx
}

// matchEntity 已知实体匹配:name/alias 精确,或剥离后缀后精确。
func matchEntity(idx map[entityKey]*model.Entity, term string) *model.Entity {
	for _, c := range entityextract.StripSuffixes(term) {
		for _, typ := range []string{TypeCompany, TypeIndustry, TypeConcept, TypeTopic, TypeUnknown} {
			if e := idx[entityKey{typ, c}]; e != nil {
				return e
			}
		}
	}
	return nil
}

// matchClassified 原词 → 分类实体:名称/别名精确或后缀剥离精确。
func matchClassified(idx map[entityKey]*entityextract.ClassifiedEntity, term string) *entityextract.ClassifiedEntity {
	for _, c := range entityextract.StripSuffixes(term) {
		if e := idx[entityKey{TypeCompany, c}]; e != nil {
			return e
		}
		if e := idx[entityKey{TypeIndustry, c}]; e != nil {
			return e
		}
		if e := idx[entityKey{TypeConcept, c}]; e != nil {
			return e
		}
		if e := idx[entityKey{TypeTopic, c}]; e != nil {
			return e
		}
	}
	return nil
}

// classifyIndex 分类结果 → (type,name/alias) 索引。
func classifyIndex(cls []entityextract.ClassifiedEntity) map[entityKey]*entityextract.ClassifiedEntity {
	idx := map[entityKey]*entityextract.ClassifiedEntity{}
	for i := range cls {
		e := &cls[i]
		idx[entityKey{e.Type, e.Name}] = e
		for _, a := range e.Aliases {
			idx[entityKey{e.Type, a}] = e
		}
	}
	return idx
}

func addAffects(w *work, key entityKey, eventIDs []string, term string) {
	if w.affects[key] == nil {
		w.affects[key] = map[string]string{}
	}
	for _, id := range eventIDs {
		w.affects[key][id] = term
	}
}

// upsertEntity 合并现有别名 + 本次贡献,一次性写。返回 (id, created)。
func upsertEntity(ctx context.Context, s *store.Store, idx map[entityKey]*model.Entity, key entityKey, w *work) (string, bool, error) {
	aliases := map[string]bool{}
	if e, ok := idx[key]; ok {
		var as []string
		if json.Unmarshal(e.Aliases, &as) == nil {
			for _, a := range as {
				aliases[a] = true
			}
		}
	}
	for a := range w.aliasAdds[key] {
		aliases[a] = true
	}
	var as []string
	for a := range aliases {
		as = append(as, a)
	}
	sort.Strings(as)
	// 空别名必须序列化为 [],不能是 null(json.Marshal(nil) 会产出 "null")
	aliasJSON := json.RawMessage(`[]`)
	if len(as) > 0 {
		b, _ := json.Marshal(as)
		aliasJSON = b
	}

	e := &model.Entity{
		Type:    key.Type,
		Name:    key.Name,
		Aliases: aliasJSON,
		Detail:  w.detail[key],
	}
	id, created, err := s.UpsertEntity(ctx, e)
	if err != nil {
		return "", false, err
	}
	// 更新内存索引,供后续关系引用(新实体也登记)
	idx[key] = &model.Entity{ID: id, Type: key.Type, Name: key.Name}
	return id, created, nil
}

func sortedKeys(w *work) []entityKey {
	keys := make([]entityKey, 0, len(w.aliasAdds))
	for k := range w.aliasAdds {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Type != keys[j].Type {
			return keys[i].Type < keys[j].Type
		}
		return keys[i].Name < keys[j].Name
	})
	return keys
}

func knownNames(idx map[entityKey]*model.Entity) []string {
	seen := map[string]bool{}
	var out []string
	for k := range idx {
		if !seen[k.Name] {
			seen[k.Name] = true
			out = append(out, k.Name)
		}
	}
	sort.Strings(out)
	return out
}

func newProvider(cfg config.Config) ai.Provider {
	if os.Getenv("PIKS_AI_PROVIDER") == "mock" {
		return ai.NewMock()
	}
	return ai.NewOpenAICompat(cfg.AIServiceBaseURL, cfg.AIAPIKey, cfg.AIModelExtract)
}

// aiBudgetAvailable 日 token 护栏(0=关)。预算已耗尽 → 不再调 AI,未匹配词走 unknown。
func aiBudgetAvailable(ctx context.Context, s *store.Store, budget int64) bool {
	if budget <= 0 {
		return true
	}
	today, err := s.TokensSince(ctx, time.Now().UTC().Truncate(24*time.Hour))
	if err != nil {
		return true
	}
	return today < budget
}

func fatal(v ...any) {
	_, _ = fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}
