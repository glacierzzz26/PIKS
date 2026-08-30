package web

import (
	"net/http"
	"strings"

	"piks/internal/publish"
)

// handleEntityAPI 图谱点选面板用实体详情 JSON。
func (s *Server) handleEntityAPI(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/entities/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	ent, err := s.store.GetEntityByID(ctx, id)
	if err != nil {
		s.writeJSON(w, map[string]string{"error": "not found"})
		return
	}
	d, err := publish.BuildEntityCardData(ctx, s.store, ent, "")
	if err != nil {
		s.writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	type ref struct{ ID, Name, Type string }
	type evref struct{ ID, Title string }
	payload := map[string]any{
		"id":          ent.ID,
		"name":        ent.Name,
		"type":        ent.Type,
		"status":      orStr(&ent.Status, "active"),
		"description": orStr(ent.Description, ""),
		"industries":  func() []ref { var o []ref; for _, x := range d.Industries { o = append(o, ref{x.ID, x.Name, x.Type}) }; return o }(),
		"companies":   func() []ref { var o []ref; for _, x := range d.Companies { o = append(o, ref{x.ID, x.Name, x.Type}) }; return o }(),
		"events":      func() []evref { var o []evref; for _, x := range d.Events { o = append(o, evref{x.ID, x.Title}) }; return o }(),
		"zt_dates":    d.ZTDates,
	}
	s.writeJSON(w, payload)
}
