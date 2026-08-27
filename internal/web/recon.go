package web

import (
	"context"
	"net/http"

	"piks/internal/store"
)

// ReconCategory 对账类别(名称 + 展示文案 + 异常项)。
type ReconCategory struct {
	Name  string
	Label string
	Items []store.ReconIssue
}

// ReconPage 对账页数据。
type ReconPage struct {
	Common
	Categories []ReconCategory
	Total      int
	Passed     bool
}

var reconCategories = []struct{ key, label string }{
	{"stale_raw", "孤儿 raw(滞留>7天)"},
	{"failed_raw", "抽取失败"},
	{"processed_no_event", "已处理但无事件"},
	{"orphan_event", "孤儿 event(无 raw)"},
	{"missing_evidence", "缺证据事件"},
	{"silent_source", "静默源(近24h无采集)"},
}

func (s *Server) handleRecon(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all := []store.ReconIssue{}
	for _, f := range []func(context.Context) ([]store.ReconIssue, error){
		s.store.ReconStaleRaw, s.store.ReconFailedRaw, s.store.ReconProcessedNoEvent,
		s.store.ReconOrphanEvent, s.store.ReconMissingEvidence, s.store.ReconSilentSources,
	} {
		items, err := f(ctx)
		if err != nil {
			common := Common{Title: "对账 · PIKS", Active: "recon"}
			s.fail(w, "recon", &common, err)
			return
		}
		all = append(all, items...)
	}
	byCat := map[string][]store.ReconIssue{}
	for _, it := range all {
		byCat[it.Category] = append(byCat[it.Category], it)
	}
	page := ReconPage{Common: Common{Title: "对账 · PIKS", Active: "recon"}, Total: len(all), Passed: len(all) == 0}
	for _, c := range reconCategories {
		page.Categories = append(page.Categories, ReconCategory{Name: c.key, Label: c.label, Items: byCat[c.key]})
	}
	s.render(w, "recon", page)
}
