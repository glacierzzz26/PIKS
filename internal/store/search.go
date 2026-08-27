package store

// search 关键词知识检索(/chat grounding,迭代 5-3 设计 §4.1)。
// 中文无分词器:把用户问题拆成字符 n-gram(2/3 gram)作候选关键词,
// OR 命中 title/summary/facts 全文字段,Go 侧按命中 gram 数打分取 top。

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

// SearchKnowledge 按问题检索事件+实体(供 /chat 组装 grounding 上下文)。
// limit≤0 跳过该类型。返回已按相关度降序的候选。
func (s *Store) SearchKnowledge(ctx context.Context, q string, eventLimit, entityLimit int) (events []model.Event, entities []model.Entity, err error) {
	grams := queryGrams(q)
	if len(grams) == 0 {
		return nil, nil, nil
	}
	pats := make([]string, len(grams))
	for i, g := range grams {
		pats[i] = "%" + g + "%"
	}

	if eventLimit > 0 {
		rows, qerr := s.Pool.Query(ctx, `
			SELECT `+eventCols+` FROM events
			WHERE status IN ('extracted','verified','published')
			  AND (title ILIKE ANY($1) OR COALESCE(summary,'') ILIKE ANY($1) OR facts::text ILIKE ANY($1))
			LIMIT 300`, pats)
		if qerr != nil {
			return nil, nil, qerr
		}
		cands, cerr := pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
		if cerr != nil {
			return nil, nil, cerr
		}
		events = scoreEvents(cands, grams, eventLimit)
	}

	if entityLimit > 0 {
		rows, qerr := s.Pool.Query(ctx, `
			SELECT `+entityCols+` FROM entities
			WHERE status='active'
			  AND (name ILIKE ANY($1) OR aliases::text ILIKE ANY($1))
			LIMIT 200`, pats)
		if qerr != nil {
			return nil, nil, qerr
		}
		cands, cerr := pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
		if cerr != nil {
			return nil, nil, cerr
		}
		entities = scoreEntities(cands, grams, entityLimit)
	}
	return events, entities, nil
}

// queryGrams 从中文问题提取候选关键词:全文(短词)+ 2/3 字符 n-gram,去重。
// 英文单词(≥2 字母)整词保留。常见停用词剔除。上限 20 个,防 SQL/打分膨胀。
func queryGrams(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))

	// 先按字符类型切成连续段(连续中文整段 / 连续英文数字整段)。
	var parts []string
	var cur strings.Builder
	curIsCJK := false
	flushCur := func() {
		if cur.Len() > 0 {
			s := cur.String()
			if !curIsCJK && len(s) < 2 {
				s = "" // 单个英文字母/数字无检索价值
			}
			if s != "" {
				parts = append(parts, s)
			}
		}
		cur.Reset()
	}
	for _, r := range q {
		isCJK := r >= '一' && r <= '鿿'
		isWord := isCJK || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if isWord {
			if cur.Len() > 0 && curIsCJK != isCJK {
				flushCur()
			}
			curIsCJK = isCJK
			cur.WriteRune(r)
		} else {
			flushCur()
			curIsCJK = false
		}
	}
	flushCur()

	var grams []string
	seen := map[string]bool{}
	add := func(s string) {
		if s == "" || seen[s] || isStopword(s) {
			return
		}
		seen[s] = true
		grams = append(grams, s)
	}
	for _, p := range parts {
		if isStopword(p) {
			continue
		}
		runes := []rune(p)
		if len(runes) <= 4 {
			add(p) // 短词整词命中(如「降准」「宁德时代」)
		}
		for n := 2; n <= 3 && n <= len(runes); n++ {
			for i := 0; i+n <= len(runes); i++ {
				add(string(runes[i : i+n]))
			}
		}
	}
	if len(grams) > 20 {
		grams = grams[:20]
	}
	return grams
}

// scoreEvents 按命中 gram 数打分:全文短语优先,title 命中权重更高,再按时间新近。
func scoreEvents(cands []model.Event, grams []string, limit int) []model.Event {
	type scored struct {
		e model.Event
		s int
	}
	out := make([]scored, 0, len(cands))
	for _, e := range cands {
		sc := 0
		title := strings.ToLower(e.Title)
		body := strings.ToLower(joinStr(e.Summary) + " " + string(e.Facts))
		for _, g := range grams {
			if strings.Contains(title, g) {
				sc += 2
			} else if strings.Contains(body, g) {
				sc++
			}
		}
		if sc > 0 {
			out = append(out, scored{e, sc})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].s != out[j].s {
			return out[i].s > out[j].s
		}
		return evTime(out[i].e).After(evTime(out[j].e))
	})
	if len(out) > limit {
		out = out[:limit]
	}
	res := make([]model.Event, len(out))
	for i, x := range out {
		res[i] = x.e
	}
	return res
}

func scoreEntities(cands []model.Entity, grams []string, limit int) []model.Entity {
	type scored struct {
		e model.Entity
		s int
	}
	out := make([]scored, 0, len(cands))
	for _, en := range cands {
		sc := 0
		name := strings.ToLower(en.Name)
		body := strings.ToLower(string(en.Aliases) + " " + joinStr(en.Description))
		for _, g := range grams {
			if strings.Contains(name, g) {
				sc += 2
			} else if strings.Contains(body, g) {
				sc++
			}
		}
		if sc > 0 {
			out = append(out, scored{en, sc})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].s != out[j].s {
			return out[i].s > out[j].s
		}
		return out[i].e.UpdatedAt.After(out[j].e.UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	res := make([]model.Entity, len(out))
	for i, x := range out {
		res[i] = x.e
	}
	return res
}

func evTime(e model.Event) time.Time {
	if e.OccurredAt != nil {
		return *e.OccurredAt
	}
	return e.CreatedAt
}

func joinStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// isStopword 常见中文/英文疑问词与功能词,从候选关键词剔除(避免噪音命中)。
func isStopword(s string) bool {
	switch s {
	case "哪些", "什么", "怎么", "如何", "为什么", "请问", "相关", "内容", "板块", "影响",
		"的", "了", "是", "有", "和", "与", "及", "在", "对", "为", "中", "the", "what", "how", "why":
		return true
	}
	return false
}
