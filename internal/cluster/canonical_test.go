package cluster

import (
	"testing"
	"time"

	"piks/internal/model"
)

// canonicalIndex 代表选取单测(issue #83 P-2)。
//
// 🔴 本比较器必须与迁移 `0021_event_pipeline_p2.sql` 回填 SQL 的
// `ORDER BY cluster_id, created_at ASC, confidence DESC` **逐字一致** ——
// 分叉则「存量回填」与「新簇选取」口径不一,同一逻辑两套结果。
// 这里的用例即该 SQL 语义的 Go 镜像。纯函数,不碰库。
func TestCanonicalIndex(t *testing.T) {
	t0 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	cases := []struct {
		name  string
		times []time.Time
		confs []float64
		want  int
	}{
		{
			name:  "最早创建当选",
			times: []time.Time{t1, t0, t1},
			confs: []float64{0.9, 0.5, 0.9},
			want:  1, // 下标 1 最早
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
		{
			name:  "最早压过更高置信(时间为先)",
			times: []time.Time{t0, t1},
			confs: []float64{0.2, 0.99},
			want:  0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			events := make([]model.Event, len(c.times))
			comp := make([]int, len(c.times))
			for i := range c.times {
				events[i] = model.Event{ID: string(rune('a' + i)), CreatedAt: c.times[i], Confidence: c.confs[i]}
				comp[i] = i
			}
			if got := canonicalIndex(events, comp); got != c.want {
				t.Errorf("canonicalIndex = %d, want %d", got, c.want)
			}
		})
	}
}
