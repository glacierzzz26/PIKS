package web

import (
	"encoding/json"
	"testing"
	"time"

	"piks/internal/store"
)

// TestToAPIReviewKeepsMistakes 回归 P6-1 缺陷 #1:
// 旧实现用 risks+**mistakes** 合计算 state,却只投影 risks —— 复盘点在诊断页永久丢失。
// 断言:mistakes 必须原样出现在响应,且 state 仍由二者合计决定。
func TestToAPIReviewKeepsMistakes(t *testing.T) {
	raw := json.RawMessage(`{
		"review": "本期诊断",
		"risks":    [{"title": "持仓集中", "content": "单票占比过高"}],
		"mistakes": [{"title": "追高买入", "content": "未按纪律等待回调"}],
		"refs": {"events": [{"id":"e1"}], "entities": [], "notes": []}
	}`)
	got := toAPIReview(store.PositionReview{
		SnapshotDate: time.Date(2026, 9, 12, 0, 0, 0, 0, cst),
		Review:       raw,
	})

	if len(got.Risks) != 1 {
		t.Fatalf("risks 应 1 条,得 %d", len(got.Risks))
	}
	if len(got.Mistakes) != 1 {
		t.Fatalf("mistakes 应 1 条(缺陷 #1 回归:曾被丢弃),得 %d", len(got.Mistakes))
	}
	if got.Mistakes[0].Title != "追高买入" {
		t.Errorf("mistake 标题 = %q, want 追高买入", got.Mistakes[0].Title)
	}
	// state 由 risks+mistakes 合计(=2)决定 → negative
	if got.State != "negative" {
		t.Errorf("state = %q, want negative (risks 1 + mistakes 1 = 2)", got.State)
	}
	if got.Date != "2026-09-12" {
		t.Errorf("date = %q, want 2026-09-12", got.Date)
	}
	if got.Refs != 1 {
		t.Errorf("refs = %d, want 1", got.Refs)
	}
}

// TestToAPIReviewOnlyMistakes:仅有复盘点、无风险点时 state 也应据此升级,且 mistakes 非空。
func TestToAPIReviewOnlyMistakes(t *testing.T) {
	raw := json.RawMessage(`{
		"review": "只有复盘点",
		"mistakes": [{"title": "过早止盈", "content": "未让利润奔跑"}]
	}`)
	got := toAPIReview(store.PositionReview{Review: raw, SnapshotDate: time.Now()})
	if len(got.Risks) != 0 {
		t.Errorf("risks 应空,得 %d", len(got.Risks))
	}
	if len(got.Mistakes) != 1 {
		t.Fatalf("mistakes 应 1 条,得 %d", len(got.Mistakes))
	}
	if got.State != "neutral" {
		t.Errorf("state = %q, want neutral (1 点)", got.State)
	}
}
