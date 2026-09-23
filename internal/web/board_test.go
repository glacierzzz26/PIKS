package web

import (
	"testing"
	"time"
)

// boardWindow 早/晚档窗口单测(issue #83 P-3)。
// 硬约束:两档衔接**无重叠、无缝隙**(issue P1)。
func TestBoardWindow(t *testing.T) {
	d := time.Date(2026, 9, 22, 0, 0, 0, 0, cst) // 2026-09-22 北京

	lateS, lateE := boardWindow(d, BoardStageLate)
	if lateS.Hour() != 9 || lateS.Minute() != 15 {
		t.Errorf("晚盘起应为 09:15, got %s", lateS.Format("15:04"))
	}
	if lateE.Hour() != 18 || lateE.Minute() != 30 {
		t.Errorf("晚盘止应为 18:30, got %s", lateE.Format("15:04"))
	}
	if lateS.Day() != 22 || lateE.Day() != 22 {
		t.Errorf("晚盘应落在当日, got %s ~ %s", lateS, lateE)
	}

	earlyS, earlyE := boardWindow(d, BoardStageEarly)
	if earlyE.Hour() != 9 || earlyE.Minute() != 15 {
		t.Errorf("早盘止应为 09:15, got %s", earlyE.Format("15:04"))
	}
	if earlyS.Hour() != 18 || earlyS.Minute() != 30 || earlyS.Day() != 21 {
		t.Errorf("早盘起应为前一日 18:30, got %s", earlyS.Format("01-02 15:04"))
	}

	// 无缝无叠:early(d).end == late(d).start 且 late(d).end == early(d+1).start。
	if !earlyE.Equal(lateS) {
		t.Errorf("早盘止(%s) 应等于晚盘起(%s) —— 有缝/重叠", earlyE, lateS)
	}
	nextEarlyS, _ := boardWindow(d.AddDate(0, 0, 1), BoardStageEarly)
	if !lateE.Equal(nextEarlyS) {
		t.Errorf("晚盘止(%s) 应等于次日早盘起(%s) —— 有缝/重叠", lateE, nextEarlyS)
	}
}

// boardStageNow 默认档:>=18:30 → 晚盘,否则早盘(红线定稿)。
func TestBoardStageNow(t *testing.T) {
	cases := []struct {
		h, m int
		want string
	}{
		{9, 14, BoardStageEarly}, // 09:15 前
		{9, 15, BoardStageEarly}, // 晚盘起,但「现在」仍属早盘时段
		{17, 59, BoardStageEarly},
		{18, 29, BoardStageEarly},
		{18, 30, BoardStageLate}, // 晚盘时刻起
		{23, 0, BoardStageLate},
	}
	for _, c := range cases {
		now := time.Date(2026, 9, 22, c.h, c.m, 0, 0, cst)
		if got := boardStageNow(now); got != c.want {
			t.Errorf("boardStageNow(%02d:%02d) = %s, want %s", c.h, c.m, got, c.want)
		}
	}
}

// boardScore 归一化加权:Σ wᵢ·normᵢ;w=0 的信号项不贡献;max=0 防除零。
func TestBoardScore(t *testing.T) {
	max := Signals{CrossChannel: 4}
	w := Weights{CrossChannel: 1, Entity: 0, Watch: 0}

	if got := boardScore(Signals{CrossChannel: 4}, max, w); got != 1.0 {
		t.Errorf("满值应得 1.0, got %v", got)
	}
	if got := boardScore(Signals{CrossChannel: 2}, max, w); got != 0.5 {
		t.Errorf("一半应得 0.5, got %v", got)
	}
	// w=0 的信号即使非 0 也不贡献。
	got := boardScore(Signals{CrossChannel: 0, Entity: 99, Watch: 1}, max, w)
	if got != 0 {
		t.Errorf("w=0 项不应贡献, got %v", got)
	}
	// max=0(窗口内无信号)须防除零。
	if got := boardScore(Signals{}, Signals{}, w); got != 0 {
		t.Errorf("max=0 应返回 0, got %v", got)
	}
}
