package web

import (
	"net/http"
	"strings"
	"time"
)

// DayVM 复盘索引单日。
type DayVM struct {
	Date              string
	Emotion           string
	Score             string
	LimitUp, LimitDown int
	Has               bool
}

// ReviewsPage 复盘索引页。
type ReviewsPage struct {
	Common
	Days []DayVM
}

// ReviewPage 单日复盘页(继承 daily-review 12 节,HTML 呈现)。
type ReviewPage struct {
	Common
	Snap SnapVM
	Link string // 原 Markdown 复盘页路径(信息),可空
}

func (s *Server) handleReviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	snaps, err := s.store.ListMarketSnapshots(ctx, 10)
	if err != nil {
		s.fail(w, "reviews", &Common{Title: "复盘 · PIKS", Active: "reviews"}, err)
		return
	}
	page := ReviewsPage{Common: Common{Title: "每日复盘 · PIKS", Active: "reviews"}}
	for _, snap := range snaps {
		vm := parseSnap(&snap)
		page.Days = append(page.Days, DayVM{
			Date: vm.Date, Emotion: vm.Emotion, Score: vm.Score,
			LimitUp: vm.LimitUp, LimitDown: vm.LimitDown, Has: true,
		})
	}
	s.render(w, "reviews", page)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	dateStr := strings.TrimPrefix(r.URL.Path, "/reviews/")
	if dateStr == "" {
		http.Redirect(w, r, "/reviews", http.StatusFound)
		return
	}
	day, err := time.ParseInLocation("2006-01-02", dateStr, cst)
	if err != nil {
		s.render(w, "review", ReviewPage{Common: Common{Title: "复盘 · PIKS", Active: "reviews", Err: "日期格式错误,应为 2006-01-02"}})
		return
	}
	ctx := r.Context()
	snap, err := s.store.GetMarketSnapshotByDate(ctx, day)
	if err != nil {
		common := Common{Title: "复盘 · PIKS", Active: "reviews"}
		s.fail(w, "review", &common, err)
		return
	}
	page := ReviewPage{Common: Common{Title: "复盘 " + dateStr + " · PIKS", Active: "reviews"}}
	page.Snap = parseSnap(snap)
	s.render(w, "review", page)
}
