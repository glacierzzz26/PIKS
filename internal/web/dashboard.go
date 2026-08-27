package web

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"piks/internal/store"
)

// TrendLabel 趋势线上方标注(日期+分数)。
type TrendLabel struct {
	X, Y  int
	Text  string
	Score float64
}

// TrendVM 情绪趋势 SVG(服务端算好点,模板直出)。
type TrendVM struct {
	Points string
	Max    float64
	Labels []TrendLabel
	Note   string
}

// DashboardPage 看板页数据。
type DashboardPage struct {
	Common
	Snaps     []SnapVM
	Trend     TrendVM
	Stats     store.Counts
	Generated string
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	snaps, err := s.store.ListMarketSnapshots(ctx, 6)
	if err != nil {
		s.fail(w, "dashboard", &Common{Title: "看板 · PIKS", Active: "dashboard"}, err)
		return
	}
	counts, err := s.store.Counts(ctx)
	if err != nil {
		s.fail(w, "dashboard", &Common{Title: "看板 · PIKS", Active: "dashboard"}, err)
		return
	}

	page := DashboardPage{
		Common:    Common{Title: "看板 · PIKS", Active: "dashboard"},
		Stats:     counts,
		Generated: time.Now().In(cst).Format("2006-01-02 15:04"),
	}
	for i := range snaps {
		page.Snaps = append(page.Snaps, parseSnap(&snaps[i]))
	}
	page.Trend = buildTrend(page.Snaps)
	s.render(w, "dashboard", page)
}

// buildTrend 情绪分 → SVG polyline 点 + 标注 + 两日对比说明。
func buildTrend(snaps []SnapVM) TrendVM {
	var t TrendVM
	if len(snaps) == 0 {
		return t
	}
	var maxScore float64
	for _, s := range snaps {
		if v := scoreOf(s.Score); v > maxScore {
			maxScore = v
		}
	}
	if maxScore <= 0 {
		maxScore = 1
	}
	t.Max = maxScore
	n := len(snaps)
	for i, s := range snaps {
		score := scoreOf(s.Score)
		x := 200
		if n > 1 {
			x = 10 + i*(380/(n-1))
		}
		y := 110 - int((score/maxScore)*90)
		t.Points += fmt.Sprintf("%d,%d ", x, y)
		t.Labels = append(t.Labels, TrendLabel{X: x, Y: y - 8, Text: s.Date + " " + s.Score, Score: score})
	}
	if len(snaps) >= 2 {
		a, b := snaps[len(snaps)-2], snaps[len(snaps)-1]
		t.Note = fmt.Sprintf("情绪分 %s → %s。涨停 %d→%d,跌停 %d→%d,炸板 %d→%d,最高板 %d→%d。",
			a.Score, b.Score, a.LimitUp, b.LimitUp, a.LimitDown, b.LimitDown, a.Broken, b.Broken, a.MaxBoard, b.MaxBoard)
	}
	return t
}

func scoreOf(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
