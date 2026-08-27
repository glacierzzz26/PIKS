package web

// 笔记(personal_notes)页面:列表/新建/查看/编辑/归档;关联事件/实体(5-2)。

import (
	"fmt"
	"net/http"
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

func noteTypeBadge(t string) string { return "note-" + t }

// NotesPage 笔记列表。
type NotesPage struct {
	Common
	Total      int
	ActiveType string
	Types      []TypeChip
	Notes      []NoteItem
}

type TypeChip struct{ Key, Label string }

type NoteItem struct {
	ID, Type, TypeLabel, Title, Status, Content, Updated string
	Confidence                                           *float64
}

func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	activeType := r.URL.Query().Get("type")
	notes, err := s.store.ListPersonalNotes(r.Context(), activeType)
	if err != nil {
		s.render(w, "notes", NotesPage{Common: Common{Title: "个人笔记 · PIKS", Active: "notes", Err: err.Error()}})
		return
	}
	items := make([]NoteItem, 0, len(notes))
	for _, n := range notes {
		title := orStr(n.Title, n.Slug)
		content := orStr(n.Content, "")
		if len(content) > 90 {
			content = content[:90] + "…"
		}
		items = append(items, NoteItem{
			ID: n.ID, Type: n.Type, TypeLabel: noteTypeLabel[n.Type], Title: title, Status: n.Status,
			Content: content, Updated: fmtTime(n.UpdatedAt), Confidence: n.Confidence,
		})
	}
	types := []TypeChip{{Key: "", Label: "全部"}}
	for _, k := range []string{"belief", "case", "mistake", "note"} {
		types = append(types, TypeChip{Key: k, Label: noteTypeLabel[k]})
	}
	s.render(w, "notes", NotesPage{
		Common:     Common{Title: "个人笔记 · PIKS", Active: "notes"},
		Total:      len(items), ActiveType: activeType, Types: types, Notes: items,
	})
}

// noteOpt 关联选择器选项。
type noteOpt struct {
	ID, Label string
}

// NoteForm 新建/编辑表单。
type NoteForm struct {
	Common
	Editing        bool
	ID, Type, Slug string
	Title, Content string
	Status         string
	Confidence     string
	TypeChoices    []TypeChip
	StatusOptions  []string
	Events         []noteOpt
	Entities       []noteOpt
	SelEvents      []string
	SelEntities    []string
}

func (s *Server) noteFormBase(w http.ResponseWriter, r *http.Request, editing bool, id string) (*NoteForm, error) {
	events, err := s.store.ListEventsRecent(r.Context(), 150)
	if err != nil {
		return nil, err
	}
	ents, err := s.store.ListAllEntities(r.Context())
	if err != nil {
		return nil, err
	}
	f := &NoteForm{
		Common:        Common{Title: "笔记 · PIKS", Active: "notes"},
		Editing:       editing, ID: id,
		TypeChoices: []TypeChip{
			{Key: "belief", Label: "信念 Belief"},
			{Key: "case", Label: "案例 Case"},
			{Key: "mistake", Label: "错误 Mistake"},
			{Key: "note", Label: "我的理解 Note"},
		},
		StatusOptions: noteStatusOptions("belief"),
	}
	for _, e := range events {
		f.Events = append(f.Events, noteOpt{ID: e.ID, Label: e.Title})
	}
	for _, e := range ents {
		f.Entities = append(f.Entities, noteOpt{ID: e.ID, Label: e.Name + " (" + zhGo(e.Type) + ")"})
	}
	return f, nil
}

func (s *Server) handleNoteNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.createNote(w, r)
		return
	}
	f, err := s.noteFormBase(w, r, false, "")
	if err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "笔记 · PIKS", Active: "notes", Err: err.Error()}})
		return
	}
	f.Type = "belief"
	f.Status = "hypothesis"
	s.render(w, "note_form", f)
}

// NoteView 笔记详情。
type NoteView struct {
	Common
	Note      model.PersonalNote
	Refs      []store.NoteRefDetail
	TypeLabel string
	Updated   string
	StatusOpts []string
}

func (s *Server) handleNote(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/notes/"), "/")
	id := parts[0]
	switch {
	case len(parts) >= 2 && parts[1] == "edit":
		if r.Method == http.MethodPost {
			s.updateNote(w, r, id)
			return
		}
		s.editNoteForm(w, r, id)
	case len(parts) >= 2 && parts[1] == "delete":
		s.archiveNote(w, r, id)
	default:
		s.viewNote(w, r, id)
	}
}

func (s *Server) viewNote(w http.ResponseWriter, r *http.Request, id string) {
	n, err := s.store.GetPersonalNote(r.Context(), id)
	if err != nil {
		s.render(w, "note", NoteView{Common: Common{Title: "笔记 · PIKS", Active: "notes", Err: "笔记不存在或读取失败: " + err.Error()}})
		return
	}
	refs, err := s.store.ListNoteRefs(r.Context(), id)
	if err != nil {
		s.render(w, "note", NoteView{Common: Common{Title: "笔记 · PIKS", Active: "notes", Err: err.Error()}})
		return
	}
	s.render(w, "note", NoteView{
		Common:     Common{Title: orStr(n.Title, n.Slug) + " · PIKS", Active: "notes"},
		Note:       *n,
		Refs:       refs,
		TypeLabel:  noteTypeLabel[n.Type],
		Updated:    fmtTime(n.UpdatedAt),
		StatusOpts: noteStatusOptions(n.Type),
	})
}

func (s *Server) editNoteForm(w http.ResponseWriter, r *http.Request, id string) {
	n, err := s.store.GetPersonalNote(r.Context(), id)
	if err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "编辑笔记 · PIKS", Active: "notes", Err: "笔记不存在: " + err.Error()}})
		return
	}
	f, err := s.noteFormBase(w, r, true, id)
	if err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "编辑笔记 · PIKS", Active: "notes", Err: err.Error()}})
		return
	}
	f.Type = n.Type
	f.Slug = n.Slug
	f.Title = orStr(n.Title, "")
	f.Status = n.Status
	f.Content = orStr(n.Content, "")
	if n.Confidence != nil {
		f.Confidence = strconv.FormatFloat(*n.Confidence, 'f', -1, 64)
	}
	f.StatusOptions = noteStatusOptions(n.Type)
	refs, _ := s.store.ListNoteRefs(r.Context(), id)
	for _, ref := range refs {
		if ref.ToType == "event" {
			f.SelEvents = append(f.SelEvents, ref.ToID)
		} else if ref.ToType == "entity" {
			f.SelEntities = append(f.SelEntities, ref.ToID)
		}
	}
	s.render(w, "note_form", f)
}

func parseNoteForm(r *http.Request) (noteInput, string) {
	r.ParseForm()
	in := noteInput{
		Type:       strings.TrimSpace(r.FormValue("type")),
		Slug:       strings.TrimSpace(r.FormValue("slug")),
		Title:      strings.TrimSpace(r.FormValue("title")),
		Status:     strings.TrimSpace(r.FormValue("status")),
		Confidence: strings.TrimSpace(r.FormValue("confidence")),
		Content:    strings.TrimSpace(r.FormValue("content")),
		SelEvents:  r.Form["sel_events"],
		SelEnts:    r.Form["sel_entities"],
	}
	if _, ok := noteTypeLabel[in.Type]; !ok {
		return in, "类型不合法: " + in.Type
	}
	if in.Title == "" {
		return in, "标题必填"
	}
	if in.Content == "" {
		return in, "内容必填"
	}
	return in, ""
}

type noteInput struct {
	Type, Slug, Title, Status, Confidence, Content string
	SelEvents, SelEnts                             []string
}

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	in, errMsg := parseNoteForm(r)
	if errMsg != "" {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "笔记 · PIKS", Active: "notes", Err: errMsg}})
		return
	}
	n, refs, err := buildNote(in)
	if err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "笔记 · PIKS", Active: "notes", Err: err.Error()}})
		return
	}
	id, err := s.store.CreatePersonalNote(r.Context(), &n)
	if err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "笔记 · PIKS", Active: "notes", Err: "创建失败: " + err.Error()}})
		return
	}
	if err := s.store.ReplaceNoteRefs(r.Context(), id, refs); err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "笔记 · PIKS", Active: "notes", Err: "关联失败: " + err.Error()}})
		return
	}
	http.Redirect(w, r, "/notes/"+id, http.StatusSeeOther)
}

func (s *Server) updateNote(w http.ResponseWriter, r *http.Request, id string) {
	in, errMsg := parseNoteForm(r)
	if errMsg != "" {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "编辑笔记 · PIKS", Active: "notes", Err: errMsg}})
		return
	}
	n, refs, err := buildNote(in)
	if err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "编辑笔记 · PIKS", Active: "notes", Err: err.Error()}})
		return
	}
	n.ID = id
	n.UpdatedBy = strPtr("me")
	if err := s.store.UpdatePersonalNote(r.Context(), &n); err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "编辑笔记 · PIKS", Active: "notes", Err: "保存失败: " + err.Error()}})
		return
	}
	if err := s.store.ReplaceNoteRefs(r.Context(), id, refs); err != nil {
		s.render(w, "note_form", NoteForm{Common: Common{Title: "编辑笔记 · PIKS", Active: "notes", Err: "关联失败: " + err.Error()}})
		return
	}
	http.Redirect(w, r, "/notes/"+id, http.StatusSeeOther)
}

func (s *Server) archiveNote(w http.ResponseWriter, r *http.Request, id string) {
	if err := s.store.ArchivePersonalNote(r.Context(), id); err != nil {
		s.render(w, "notes", NotesPage{Common: Common{Title: "个人笔记 · PIKS", Active: "notes", Err: "归档失败: " + err.Error()}})
		return
	}
	http.Redirect(w, r, "/notes", http.StatusSeeOther)
}

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
