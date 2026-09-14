package web

import (
	"testing"
	"time"

	"piks/internal/store"
)

// P6-3 自选富化回归:按实体分组 + 每组按日期倒序。
// 原缺陷方向:若不分组倒序,首页「最近消息」会取到最旧那条。
func TestGroupWatchEventsOrdersByDateDesc(t *testing.T) {
	day := func(s string) time.Time {
		tt, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatal(err)
		}
		return tt
	}
	refs := []store.WatchEventRef{
		{EntityID: "e1", EventID: "ev-old", Title: "旧闻", OccurredAt: day("2026-08-01")},
		{EntityID: "e1", EventID: "ev-new", Title: "新消息", OccurredAt: day("2026-08-26")},
		{EntityID: "e2", EventID: "ev-other", Title: "别家", OccurredAt: day("2026-08-10")},
	}
	got := groupWatchEvents(refs)

	if len(got["e1"]) != 2 {
		t.Fatalf("e1 应聚 2 条, got %d", len(got["e1"]))
	}
	if got["e1"][0].LatestID != "ev-new" {
		t.Errorf("e1 首条应为最新(ev-new), got %q", got["e1"][0].LatestID)
	}
	if got["e1"][0].Date != "2026-08-26" {
		t.Errorf("e1 首条日期应为 2026-08-26, got %q", got["e1"][0].Date)
	}
	if len(got["e2"]) != 1 || got["e2"][0].LatestID != "ev-other" {
		t.Errorf("e2 只应含自身事件, got %+v", got["e2"])
	}
	if _, ok := got["e3"]; ok {
		t.Error("无事件的实体不应出现在分组里")
	}
}

// 待办排序:有新消息 → 未深研 → 其余,越小越靠前。
func TestWatchNeedRank(t *testing.T) {
	withNews := apiWatchItem{Code: "600371", HasResearch: true, LatestEvent: &apiWatchEvent{Title: "x"}}
	noResearch := apiWatchItem{Code: "600371"}
	researched := apiWatchItem{Code: "600371", HasResearch: true}
	noCode := apiWatchItem{} // 非公司实体(无代码)不算「未深研」

	if watchNeedRank(withNews) >= watchNeedRank(noResearch) {
		t.Error("有新消息应排在未深研之前")
	}
	if watchNeedRank(noResearch) >= watchNeedRank(researched) {
		t.Error("未深研应排在其他之前")
	}
	if watchNeedRank(noCode) != watchNeedRank(researched) {
		t.Error("无代码实体不应被算作「未深研」")
	}
}
