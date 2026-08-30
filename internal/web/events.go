package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"piks/internal/model"
)

// AffectedVM 事件详情 JSON 的"影响"项:受影响词 → 可链接实体(未命中保持纯文本,诚实)。
type AffectedVM struct {
	Word       string
	EntityID   string
	EntityName string
	EntityType string
	Linked     bool
}

// handleEventAPI 图谱点选面板用的事件详情 JSON。
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
