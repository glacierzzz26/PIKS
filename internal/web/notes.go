package web

// 笔记(personal_notes)业务核心:类型映射/校验/构建/关联。
// 页面由 React SPA 提供,Go 侧通过 /api/v1/notes 读写。

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

var noteTypeLabel = map[string]string{
	"belief":  "信念",
	"case":    "案例",
	"mistake": "错误",
	"note":    "我的理解",
}

// zhGo Go 侧枚举中文化(等价模板 zh;事件/实体选择器标签用)。
func zhGo(v string) string {
	m := map[string]string{
		"company": "公司", "earnings": "业绩", "industry": "行业", "macro": "宏观",
		"policy": "政策", "tech": "科技", "concept": "概念", "topic": "主题",
		"active": "活跃", "extracted": "已抽取", "merged": "已合并",
	}
	if c, ok := m[v]; ok {
		return c + "(" + v + ")"
	}
	return v
}

func noteStatusOptions(t string) []string {
	if t == "belief" {
		return []string{"hypothesis", "active", "confirmed", "questioned", "rejected"}
	}
	return []string{"active", "archived"}
}

// noteInput 笔记创建/编辑输入(HTML form 与 JSON API 共用)。
type noteInput struct {
	Type       string   `json:"type"`
	Slug       string   `json:"slug"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Confidence string   `json:"confidence"`
	Content    string   `json:"content"`
	SelEvents  []string `json:"sel_events"`
	SelEnts    []string `json:"sel_entities"`
}

// buildNote 从输入构建笔记模型(校验类型/状态/置信度;slug 留空自动生成)。
func buildNote(in noteInput) (model.PersonalNote, []store.NoteRef, error) {
	slug := in.Slug
	if slug == "" {
		slug = slugify(in.Title)
		if slug == "" {
			slug = fmt.Sprintf("note-%d", time.Now().UnixNano())
		}
	}
	status := in.Status
	if status == "" {
		status = "hypothesis"
	}
	valid := map[string]bool{}
	for _, s := range noteStatusOptions(in.Type) {
		valid[s] = true
	}
	if !valid[status] {
		return model.PersonalNote{}, nil, fmt.Errorf("状态不合法: %s(该类型可选 %v)", status, noteStatusOptions(in.Type))
	}
	n := model.PersonalNote{
		Type: in.Type, Slug: slug, Title: &in.Title,
		Status: status, Content: &in.Content,
	}
	if in.Confidence != "" {
		v, err := strconv.ParseFloat(in.Confidence, 64)
		if err != nil || v < 0 || v > 1 {
			return model.PersonalNote{}, nil, fmt.Errorf("置信度必须是 0~1 的数字")
		}
		n.Confidence = &v
	}
	var refs []store.NoteRef
	for _, id := range in.SelEvents {
		refs = append(refs, store.NoteRef{ToType: "event", ToID: id})
	}
	for _, id := range in.SelEnts {
		refs = append(refs, store.NoteRef{ToType: "entity", ToID: id})
	}
	return n, refs, nil
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		keep := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r > 127
		if keep {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func strPtr(s string) *string { return &s }
