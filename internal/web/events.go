package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"piks/internal/model"
)

// ---- 视图模型 ----

// AffectedVM 事件"影响"节:受影响词 → 可链接实体(未命中保持纯文本,诚实)。
type AffectedVM struct {
	Word       string
	EntityID   string
	EntityName string
	EntityType string
	Linked     bool
}

// EvidenceVM 证据行。
type EvidenceVM struct {
	Claim       string
	Title       string
	URL         string
	SourceType  string
	Reliability string
	PublishedAt string
	Content     string
}

// EventVM 事件卡主体。
type EventVM struct {
	ID         string
	Title      string
	EventType  string
	Date       string
	Status     string
	Source     string
	Confidence float64
	Summary    string
	Facts      []string
}

// EventItem 事件流单条。
type EventItem struct {
	ID         string
	Title      string
	EventType  string
	Source     string
	Date       string
	Status     string
	Confidence float64
	Summary    string
	Affected   []AffectedVM
}

// DateGroup 按日分组。
type DateGroup struct {
	Date  string
	Items []EventItem
}

// EventsPage 事件流页数据。
type EventsPage struct {
	Common
	Groups []DateGroup
	Total  int
}

// EventPage 事件卡页数据。
type EventPage struct {
	Common
	Event        EventVM
	Affected     []AffectedVM
	Evidence     []EvidenceVM
	Understanding string // 我的理解(type='note',slug=event-<id>)
}

// ---- 事件流 ----

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	evs, err := s.store.ListEventsForPublishWithSource(ctx)
	if err != nil {
		s.fail(w, "events", &Common{Title: "事件流", Active: "events"}, err)
		return
	}
	ents, err := s.store.ListAllEntities(ctx)
	if err != nil {
		s.fail(w, "events", &Common{Title: "事件流", Active: "events"}, err)
		return
	}
	idx := newTermIndex(ents)

	byDate := map[string][]EventItem{}
	for _, ev := range evs {
		date := ev.CreatedAt.Format("2006-01-02")
		if ev.OccurredAt != nil {
			date = ev.OccurredAt.Format("2006-01-02")
		}
		item := EventItem{
			ID: ev.ID, Title: ev.Title, EventType: ev.EventType,
			Source: ev.SourceName, Date: date, Status: ev.Status,
			Confidence: ev.Confidence,
		}
		if ev.Summary != nil {
			item.Summary = strings.TrimSpace(*ev.Summary)
		}
		var affected []string
		if json.Unmarshal(ev.Affected, &affected) == nil {
			item.Affected = resolveAffected(idx, affected)
		}
		byDate[date] = append(byDate[date], item)
	}

	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	groups := make([]DateGroup, 0, len(dates))
	for _, d := range dates {
		groups = append(groups, DateGroup{Date: d, Items: byDate[d]})
	}
	s.render(w, "events", EventsPage{
		Common: Common{Title: "事件流 · PIKS", Active: "events"},
		Groups: groups, Total: len(evs),
	})
}

// ---- 事件卡 ----

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/events/")
	if strings.HasSuffix(path, "/understanding") {
		s.handleUnderstanding(w, r, strings.TrimSuffix(path, "/understanding"))
		return
	}
	id := path
	ctx := r.Context()
	ev, err := s.store.GetEventByID(ctx, id)
	if err != nil {
		s.render(w, "event", EventPage{Common: Common{Title: "事件 · PIKS", Active: "events", Err: "事件不存在: " + id}})
		return
	}
	ents, _ := s.store.ListAllEntities(ctx)
	idx := newTermIndex(ents)

	page := EventPage{Common: Common{Title: ev.Title + " · PIKS", Active: "events"}}
	page.Event = buildEventVM(ev)

	var affected []string
	if json.Unmarshal(ev.Affected, &affected) == nil {
		page.Affected = resolveAffected(idx, affected)
	}

	evs, err := s.store.ListEvidenceByEventID(ctx, id)
	if err != nil {
		s.fail(w, "event", &page.Common, err)
		return
	}
	for _, e := range evs {
		vm := EvidenceVM{
			Claim:       e.Claim,
			Title:       orStr(e.Title, ""),
			URL:         orStr(e.URL, ""),
			SourceType:  orStr(e.SourceType, ""),
			Reliability: orStr(e.Reliability, ""),
			Content:     orStr(e.Content, ""),
		}
		if e.PublishedAt != nil {
			vm.PublishedAt = e.PublishedAt.Format("2006-01-02 15:04")
		}
		page.Evidence = append(page.Evidence, vm)
	}
	if un, err := s.store.GetPersonalNoteBySlug(ctx, "note", "event-"+id); err == nil && un != nil {
		page.Understanding = orStr(un.Content, "")
	}
	s.render(w, "event", page)
}

// handleEventAPI 图谱点选面板用的事件详情 JSON。
// handleUnderstanding 事件卡「我的理解」保存(POST /events/{id}/understanding)。
// 落 personal_notes(type='note', slug='event-<id>')+ references 关系关联该事件。
func (s *Server) handleUnderstanding(w http.ResponseWriter, r *http.Request, eventID string) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/events/"+eventID, http.StatusSeeOther)
		return
	}
	ctx := r.Context()
	ev, err := s.store.GetEventByID(ctx, eventID)
	if err != nil {
		http.Redirect(w, r, "/events", http.StatusSeeOther)
		return
	}
	text := strings.TrimSpace(r.FormValue("understanding"))

	note, err := s.store.GetPersonalNoteBySlug(ctx, "note", "event-"+eventID)
	if err != nil {
		s.render(w, "event", EventPage{Common: Common{Title: "事件 · PIKS", Active: "events", Err: "读取我的理解失败: " + err.Error()}})
		return
	}
	if note == nil {
		title := ev.Title
		id, err := s.store.CreatePersonalNote(ctx, &model.PersonalNote{
			Type: "note", Slug: "event-" + eventID, Title: &title,
			Status: "active", Content: &text,
		})
		if err != nil {
			s.render(w, "event", EventPage{Common: Common{Title: "事件 · PIKS", Active: "events", Err: "保存我的理解失败: " + err.Error()}})
			return
		}
		_ = s.store.CreateRelationship(ctx, &model.Relationship{
			FromType: "personal_note", FromID: id,
			ToType: "event", ToID: eventID,
			RelType: "references", Source: strPtr("web-我的理解"),
		})
	} else {
		note.Content = &text
		note.UpdatedBy = strPtr("me")
		if err := s.store.UpdatePersonalNote(ctx, note); err != nil {
			s.render(w, "event", EventPage{Common: Common{Title: "事件 · PIKS", Active: "events", Err: "保存我的理解失败: " + err.Error()}})
			return
		}
	}
	http.Redirect(w, r, "/events/"+eventID+"#understanding", http.StatusSeeOther)
}

func (s *Server) handleEventAPI(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/events/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	ev, err := s.store.GetEventByID(ctx, id)
	if err != nil {
		s.writeJSON(w, map[string]string{"error": "not found"})
		return
	}
	ents, _ := s.store.ListAllEntities(ctx)
	idx := newTermIndex(ents)
	var affected []string
	_ = json.Unmarshal(ev.Affected, &affected)
	evs, _ := s.store.ListEvidenceByEventID(ctx, id)

	type apiEvidence struct {
		Claim, URL, Reliability, SourceType string
	}
	payload := map[string]any{
		"id":         ev.ID,
		"title":      ev.Title,
		"event_type": ev.EventType,
		"date":       eventDate(ev),
		"status":     ev.Status,
		"source":     "",
		"confidence": ev.Confidence,
		"summary":    orStr(ev.Summary, ""),
		"facts":      factsOf(ev.Facts),
		"affected":   resolveAffected(idx, affected),
		"evidence":   func() []apiEvidence { var o []apiEvidence; for _, e := range evs { o = append(o, apiEvidence{e.Claim, orStr(e.URL, ""), orStr(e.Reliability, ""), orStr(e.SourceType, "")}) }; return o }(),
	}
	s.writeJSON(w, payload)
}

// ---- 帮助 ----

func resolveAffected(idx *termIndex, words []string) []AffectedVM {
	out := make([]AffectedVM, 0, len(words))
	for _, a := range words {
		vm := AffectedVM{Word: a}
		if e, ok := idx.resolve(a); ok {
			vm.EntityID, vm.EntityName, vm.EntityType, vm.Linked = e.ID, e.Name, e.Type, true
		}
		out = append(out, vm)
	}
	return out
}

func buildEventVM(ev model.Event) EventVM {
	vm := EventVM{
		ID: ev.ID, Title: ev.Title, EventType: ev.EventType,
		Status: ev.Status, Confidence: ev.Confidence,
		Date: eventDate(ev),
	}
	if ev.Summary != nil {
		vm.Summary = strings.TrimSpace(*ev.Summary)
	}
	vm.Facts = factsOf(ev.Facts)
	return vm
}

func eventDate(ev model.Event) string {
	if ev.OccurredAt != nil {
		return ev.OccurredAt.Format("2006-01-02")
	}
	return ev.CreatedAt.Format("2006-01-02")
}

func factsOf(raw json.RawMessage) []string {
	var out []string
	if json.Unmarshal(raw, &out) == nil {
		return out
	}
	return nil
}
