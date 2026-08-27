package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"piks/internal/publish"
)

// EntityRefVM 实体引用(相关行业/公司)。
type EntityRefVM struct {
	ID, Name, Type string
}

// EventRefVM 事件引用(相关事件)。
type EventRefVM struct {
	ID, Title string
}

// EntityPage 实体卡页数据。
type EntityPage struct {
	Common
	Entity      EntityVM
	Industries  []EntityRefVM // company: 所属行业
	Companies   []EntityRefVM // industry: 属于它的公司
	Events      []EventRefVM  // affects 它的事件
	ZTDates     []string      // company 涨停日期
	HasZTField  bool
}

// EntityVM 实体主体。
type EntityVM struct {
	ID, Name, Type, Status string
	Description            string
	Code                   string // company 股票代码
	IndustryNames          string // company 所属行业(逗号分隔)
	Source                 string // industry 来源
}

func (s *Server) handleEntity(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/entities/")
	if id == "" {
		http.Redirect(w, r, "/graph", http.StatusFound)
		return
	}
	ctx := r.Context()
	ent, err := s.store.GetEntityByID(ctx, id)
	if err != nil {
		s.render(w, "entity", EntityPage{Common: Common{Title: "实体 · PIKS", Active: "graph", Err: "实体不存在: " + id}})
		return
	}
	d, err := publish.BuildEntityCardData(ctx, s.store, ent, "")
	if err != nil {
		common := Common{Title: ent.Name + " · PIKS", Active: "graph"}
		s.fail(w, "entity", &common, err)
		return
	}

	page := EntityPage{
		Common: Common{Title: ent.Name + " · PIKS", Active: "graph"},
		Entity: EntityVM{ID: ent.ID, Name: ent.Name, Type: ent.Type, Status: orStr(&ent.Status, "active")},
	}
	if ent.Description != nil {
		page.Entity.Description = *ent.Description
	}
	var detail map[string]any
	_ = json.Unmarshal(ent.Detail, &detail)
	switch ent.Type {
	case "company":
		if code, ok := detail["code"].(string); ok {
			page.Entity.Code = code
		}
		var names []string
		for _, in := range d.Industries {
			page.Industries = append(page.Industries, EntityRefVM{in.ID, in.Name, in.Type})
			names = append(names, in.Name)
		}
		page.Entity.IndustryNames = strings.Join(names, " / ")
		page.HasZTField = true
	case "industry":
		if src, ok := detail["source"].(string); ok {
			page.Entity.Source = src
		}
		for _, c := range d.Companies {
			page.Companies = append(page.Companies, EntityRefVM{c.ID, c.Name, c.Type})
		}
	}
	for _, ev := range d.Events {
		page.Events = append(page.Events, EventRefVM{ev.ID, ev.Title})
	}
	page.ZTDates = d.ZTDates
	s.render(w, "entity", page)
}

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
