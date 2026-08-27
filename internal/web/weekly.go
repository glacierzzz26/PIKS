package web

// 周报聚合页(/weekly):本周行情快照 × 本周事件 × 本周个人笔记(替代 vault 周报)。

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// WeeklyPage 周报页数据。
type WeeklyPage struct {
	Common
	Week       string
	Range      string
	Offset     int
	PrevOffset int
	NextOffset int
	Snaps      []WeeklySnap
	Events     []WeeklyEvent
	Notes      []WeeklyNote
}

type WeeklySnap struct {
	Date     string
	Weekday  string
	Emotion  string // 情绪(英文,模板 zh)
	LimitUp  int
	LimitDown int
	Turnover string
	Judgment string
}

type WeeklyEvent struct {
	ID, Title, Date, EventType string
}

type WeeklyNote struct {
	ID, Title, Type, TypeLabel, Updated string
}

var cnWeekday = [7]string{"一", "二", "三", "四", "五", "六", "日"}

func (s *Server) handleWeekly(w http.ResponseWriter, r *http.Request) {
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, _ = strconv.Atoi(v)
	}
	ctx := r.Context()
	start, end, week, rng := weekRange(time.Now().In(cst), offset)

	page := WeeklyPage{
		Common:     Common{Title: "周报 · PIKS", Active: "weekly"},
		Week:       week, Range: rng, Offset: offset,
		PrevOffset: offset + 1, NextOffset: offset - 1,
	}

	// 本周交易日快照
	snaps, err := s.store.ListMarketSnapshots(ctx, 30)
	if err != nil {
		s.fail(w, "weekly", &page.Common, err)
		return
	}
	for _, sn := range snaps {
		day := sn.TradeDate.In(cst)
		if day.Before(start) || !day.Before(end) {
			continue
		}
		ws := WeeklySnap{
			Date:     day.Format("2006-01-02"),
			Weekday:  "周" + cnWeekday[(int(day.Weekday())+6)%7],
			Emotion:  orStr(sn.EmotionState, "—"),
		}
		if sn.LimitUpCount != nil {
			ws.LimitUp = *sn.LimitUpCount
		}
		if sn.LimitDownCount != nil {
			ws.LimitDown = *sn.LimitDownCount
		}
		if sn.TurnoverAmt != nil {
			ws.Turnover = fmt.Sprintf("%.0f 亿", *sn.TurnoverAmt)
		} else {
			ws.Turnover = "—"
		}
		ws.Judgment = orStr(sn.MyJudgment, "")
		page.Snaps = append(page.Snaps, ws)
	}

	// 本周事件
	evs, err := s.store.ListEventsBetween(ctx, start, end)
	if err != nil {
		s.fail(w, "weekly", &page.Common, err)
		return
	}
	for _, e := range evs {
		d := e.CreatedAt.In(cst).Format("01-02")
		if e.OccurredAt != nil {
			d = e.OccurredAt.In(cst).Format("01-02")
		}
		page.Events = append(page.Events, WeeklyEvent{ID: e.ID, Title: e.Title, Date: d, EventType: e.EventType})
	}

	// 本周笔记(created/updated)
	notes, err := s.store.ListPersonalNotesBetween(ctx, start, end)
	if err != nil {
		s.fail(w, "weekly", &page.Common, err)
		return
	}
	for _, n := range notes {
		page.Notes = append(page.Notes, WeeklyNote{
			ID: n.ID, Title: orStr(n.Title, n.Slug),
			Type: n.Type, TypeLabel: noteTypeLabel[n.Type], Updated: fmtTime(n.UpdatedAt),
		})
	}

	s.render(w, "weekly", page)
}

// weekRange 当前周(北京时区)的 [start, end)。offset 表示往前 N 周。
func weekRange(now time.Time, offset int) (start, end time.Time, label, rng string) {
	t := now.AddDate(0, 0, -7*offset)
	wd := (int(t.Weekday()) + 6) % 7 // 周一=0
	mon := t.AddDate(0, 0, -wd)
	start = time.Date(mon.Year(), mon.Month(), mon.Day(), 0, 0, 0, 0, cst)
	end = start.AddDate(0, 0, 7)
	y, w := mon.ISOWeek()
	label = fmt.Sprintf("%04d-W%02d", y, w)
	rng = start.Format("01-02") + " ~ " + end.AddDate(0, 0, -1).Format("01-02")
	return
}
