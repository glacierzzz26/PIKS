// entities 实体卡渲染与 affected 词 wikilink 解析(迭代 3,设计 §3.3)。
// 03-Entities/{type}/ 属 Generated 分域,内容全由 DB 派生 → 重跑零提交(实体不变则逐字节相同)。
package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"piks/internal/entityextract"
	"piks/internal/model"
	"piks/internal/store"
)

// EntityPath 实体卡路径:03-Entities/{type}/{name}.md
func EntityPath(vault, entityType, name string) string {
	return filepath.Join(vault, "03-Entities", entityType, name+".md")
}

// TermResolver affected 词 → 实体 wikilink 目标(设计 §3.3)。发布器构建一次,跨事件复用。
// 命中 → "entity-{short8}";未命中 → ok=false(事件卡保持纯文本,诚实不假造链接)。
type TermResolver struct {
	index map[string]*model.Entity // key = type + "\x00" + (name|alias)
}

// NewTermResolver 从全量实体建索引。别名撞规范名时规范名优先(首个写入胜出)。
func NewTermResolver(entities []model.Entity) *TermResolver {
	idx := map[string]*model.Entity{}
	for i := range entities {
		e := &entities[i]
		key := e.Type + "\x00" + e.Name
		if _, ok := idx[key]; !ok {
			idx[key] = e
		}
		var as []string
		if json.Unmarshal(e.Aliases, &as) == nil {
			for _, a := range as {
				k := e.Type + "\x00" + a
				if _, ok := idx[k]; !ok {
					idx[k] = e
				}
			}
		}
	}
	return &TermResolver{index: idx}
}

// Resolve 精确 + 后缀剥离匹配。命中返回 wikilink 目标,未命中 ok=false。
func (r *TermResolver) Resolve(term string) (string, bool) {
	for _, c := range entityextract.StripSuffixes(term) {
		for _, typ := range []string{"company", "industry", "concept", "topic", "unknown"} {
			if e := r.index[typ+"\x00"+c]; e != nil {
				return "entity-" + shortID(e.ID), true
			}
		}
	}
	return "", false
}

// EntityRef 实体引用(wikilink 用:name 展示 + id 定位)。
type EntityRef struct {
	ID   string
	Name string
}

// EntityCardData 实体卡渲染数据(publisher 经 BuildEntityCardData 组装)。
type EntityCardData struct {
	Entity     *model.Entity
	Industries []EntityRef // company: belongs_to 行业
	Companies  []EntityRef // industry: 属于它的公司
	Events     []store.EventRef
	ZTDates    []string // company 涨停日期(东财 zt_pool 回溯)
	Pipeline   string
}

// BuildEntityCardData 从 DB 组装一张实体卡的全部派生数据。
func BuildEntityCardData(ctx context.Context, s *store.Store, ent *model.Entity, pipeline string) (EntityCardData, error) {
	d := EntityCardData{Entity: ent, Pipeline: pipeline}

	rels, err := s.ListEntityRelationships(ctx, ent.ID)
	if err != nil {
		return d, err
	}
	var partnerIDs []string
	for _, r := range rels {
		if r.RelType != "belongs_to" {
			continue
		}
		if r.FromID == ent.ID && r.ToType == "entity" {
			partnerIDs = append(partnerIDs, r.ToID)
		}
		if r.ToID == ent.ID && r.FromType == "entity" {
			partnerIDs = append(partnerIDs, r.FromID)
		}
	}
	if len(partnerIDs) > 0 {
		partners, err := s.ListEntitiesByIDs(ctx, partnerIDs)
		if err != nil {
			return d, err
		}
		for _, p := range partners {
			ref := EntityRef{p.ID, p.Name}
			if ent.Type == "company" {
				d.Industries = append(d.Industries, ref)
			} else {
				d.Companies = append(d.Companies, ref)
			}
		}
		sort.Slice(d.Industries, func(i, j int) bool { return d.Industries[i].Name < d.Industries[j].Name })
		sort.Slice(d.Companies, func(i, j int) bool { return d.Companies[i].Name < d.Companies[j].Name })
	}

	events, err := s.ListEventsAffectingEntities(ctx, []string{ent.ID})
	if err != nil {
		return d, err
	}
	d.Events = events

	if ent.Type == "company" {
		var detail map[string]any
		if json.Unmarshal(ent.Detail, &detail) == nil {
			if code, ok := detail["code"].(string); ok && code != "" {
				dates, err := s.ListZTAppearances(ctx, code)
				if err != nil {
					return d, err
				}
				for _, t := range dates {
					d.ZTDates = append(d.ZTDates, t.Format("2006-01-02"))
				}
			}
		}
	}
	return d, nil
}

// RenderEntityCard 按设计 §3.3 模板渲染实体卡。
func RenderEntityCard(d EntityCardData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: entity-%s\ntype: entity\nentity_type: %s\nname: %s\nstatus: %s\n",
		shortID(d.Entity.ID), d.Entity.Type, d.Entity.Name, orStr(d.Entity.Status, "active"))
	if d.Pipeline != "" {
		fmt.Fprintf(&b, "pipeline: %s\n", d.Pipeline)
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n", d.Entity.Name)

	b.WriteString("\n## 基本信息\n")
	renderEntityBasics(&b, d)

	b.WriteString("\n## 相关事件\n")
	if len(d.Events) == 0 {
		b.WriteString("_暂无_\n")
	}
	for _, ev := range d.Events {
		fmt.Fprintf(&b, "- [[event-%s]] %s\n", shortID(ev.ID), ev.Title)
	}

	b.WriteString("\n## 相关实体\n")
	switch d.Entity.Type {
	case "company":
		if len(d.Industries) == 0 {
			b.WriteString("_暂无_\n")
		}
		for _, in := range d.Industries {
			fmt.Fprintf(&b, "- [[entity-%s|%s]]\n", shortID(in.ID), in.Name)
		}
	case "industry":
		if len(d.Companies) == 0 {
			b.WriteString("_暂无_\n")
		}
		for _, c := range d.Companies {
			fmt.Fprintf(&b, "- [[entity-%s|%s]]\n", shortID(c.ID), c.Name)
		}
	default:
		b.WriteString("_暂无_\n")
	}

	b.WriteString("\n## 涨停记录\n")
	if len(d.ZTDates) == 0 {
		b.WriteString("_无(仅东财涨停池可回溯,或非公司实体)_\n")
	}
	for _, dt := range d.ZTDates {
		fmt.Fprintf(&b, "- %s 涨停\n", dt)
	}
	return b.String()
}

func renderEntityBasics(b *strings.Builder, d EntityCardData) {
	var detail map[string]any
	_ = json.Unmarshal(d.Entity.Detail, &detail)
	switch d.Entity.Type {
	case "company":
		code, hasCode := detail["code"].(string)
		if hasCode && code != "" {
			fmt.Fprintf(b, "- 代码: `%s`\n", code)
		} else {
			b.WriteString("- 代码: _未知(AI 识别,未在涨停池出现)_\n")
		}
		if len(d.Industries) > 0 {
			var names []string
			for _, in := range d.Industries {
				names = append(names, in.Name)
			}
			fmt.Fprintf(b, "- 行业: %s\n", strings.Join(names, " / "))
		} else {
			b.WriteString("- 行业: _未知_\n")
		}
	case "industry":
		if src, ok := detail["source"].(string); ok && src != "" {
			fmt.Fprintf(b, "- 来源: %s\n", src)
		} else {
			b.WriteString("- 来源: _未知(非东财种子,AI 识别)_\n")
		}
	default:
		fmt.Fprintf(b, "- 类型: %s(暂无专属字段,架构 §9.3 Unknown 允许)\n", d.Entity.Type)
	}
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
