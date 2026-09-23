package cluster

import (
	"testing"
	"time"

	"piks/internal/model"
)

// canonicalIndex 代表选取单测(issue #83 P6)。
//
// 规则次序:`有直接链接 > 无链接 → 来源独立性强(非转载) > 弱 → 首发时间最早(同则更高置信)`。
//
// ⚠️ 本比较器**与迁移 `0021_event_pipeline_p2.sql` 的回填 SQL 有意分叉**(P2 回填 = 旧口径
// 「最早非 merged」,**存量簇冻结不回填**;本函数 = **新簇**口径)。这是 P-3 的**批准设计决定**,
// 不是漂移 —— 见 `canonicalIndex` 注释与 `docs/phase11/design/event-pipeline-p3.md`。
func TestCanonicalIndex(t *testing.T) {
	t0 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	cases := []struct {
		name     string
		times    []time.Time
		confs    []float64
		hasURL   []bool
		isReprin []bool
		want     int
	}{
		{
			name:   "有链接胜无链接(压过最早)",
			times:  []time.Time{t0, t1},
			confs:  []float64{0.9, 0.5},
			hasURL: []bool{false, true},
			want:   1, // 下标 1 更晚但有链接
		},
		{
			name:     "非转载胜转载(同链接级)",
			times:    []time.Time{t0, t1},
			confs:    []float64{0.9, 0.5},
			hasURL:   []bool{true, true},
			isReprin: []bool{true, false},
			want:     1, // 下标 0 是转载,排除
		},
		{
			name:     "链接优先于独立性(无链接原发 vs 有链接转载)",
			times:    []time.Time{t0, t1},
			confs:    []float64{0.9, 0.5},
			hasURL:   []bool{false, true},
			isReprin: []bool{false, true},
			want:     1, // 有链接 > 无链接,先于独立性比较
		},
		{
			name:  "同级按最早创建",
			times: []time.Time{t1, t0},
			confs: []float64{0.9, 0.5},
			want:  1,
		},
		{
			name:  "同时间取更高置信",
			times: []time.Time{t0, t0},
			confs: []float64{0.5, 0.9},
			want:  1,
		},
		{
			name:  "同时间同置信取首个(确定性,不抖动)",
			times: []time.Time{t0, t0},
			confs: []float64{0.8, 0.8},
			want:  0,
		},
		{
			name:  "单成员分量即自己",
			times: []time.Time{t0},
			confs: []float64{0.1},
			want:  0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			events := make([]model.Event, len(c.times))
			meta := make([]memberMeta, len(c.times))
			comp := make([]int, len(c.times))
			for i := range c.times {
				events[i] = model.Event{ID: string(rune('a' + i)), CreatedAt: c.times[i], Confidence: c.confs[i]}
				if c.hasURL != nil {
					meta[i].hasURL = c.hasURL[i]
				}
				if c.isReprin != nil {
					meta[i].isReprint = c.isReprin[i]
				}
				comp[i] = i
			}
			if got := canonicalIndex(events, comp, meta); got != c.want {
				t.Errorf("canonicalIndex = %d, want %d", got, c.want)
			}
		})
	}
}

// canonicalTitle 无 LLM 规则单测(issue #83 P3):取「与其他成员标题重合度最高」的成员**原文标题**。
func TestCanonicalTitleOverlap(t *testing.T) {
	t0 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	t.Run("最高重合成员胜(非最早)", func(t *testing.T) {
		events := []model.Event{
			{Title: "毫不相关的第一条", CreatedAt: t0}, // 最早,但与谁都不重合
			{Title: "央行宣布下调存款准备金率0.25个百分点", CreatedAt: t0.Add(time.Hour)},
			{Title: "央行宣布下调存款准备金率0.25个百分点", CreatedAt: t0.Add(2 * time.Hour)},
		}
		comp := []int{0, 1, 2}
		// {1,2} 标题全等 ⇒ J=1.0,远高于与 0 的重合 ⇒ 从 {1,2} 里取更早的 1。
		if got := canonicalTitle(events, comp); got != events[1].Title {
			t.Errorf("canonicalTitle = %q, want %q", got, events[1].Title)
		}
	})

	t.Run("单成员即自己", func(t *testing.T) {
		events := []model.Event{{Title: "唯一标题", CreatedAt: t0}}
		if got := canonicalTitle(events, []int{0}); got != "唯一标题" {
			t.Errorf("canonicalTitle = %q", got)
		}
	})

	t.Run("无共性退化为最早一条", func(t *testing.T) {
		events := []model.Event{
			{Title: "完全不同的甲", CreatedAt: t0.Add(time.Hour)},
			{Title: "毫不相干的乙", CreatedAt: t0},
		}
		// 两两重合度同为 0 ⇒ 平票 ⇒ 取创建更早的下标 1。
		if got := canonicalTitle(events, []int{0, 1}); got != "毫不相干的乙" {
			t.Errorf("canonicalTitle = %q, want 最早一条", got)
		}
	})
}
