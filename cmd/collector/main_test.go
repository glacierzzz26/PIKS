package main

import (
	"errors"
	"testing"
	"time"
)

// 盘中时段闸:工作日 + HH:MM 区间;周末一律不采。
func TestInSession(t *testing.T) {
	// 2026-09-21 是周一(工作日);2026-09-19 是周六。
	at := func(y int, mo time.Month, d, hh, mm int) time.Time {
		return time.Date(y, mo, d, hh, mm, 0, 0, cst)
	}
	start, end, has := parseSession("09:15-15:05")
	if !has {
		t.Fatal("09:15-15:05 should parse as valid session")
	}

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"交易时段内 10:00", at(2026, 9, 21, 10, 0), true},
		{"开盘前 09:14", at(2026, 9, 21, 9, 14), false},
		{"开盘整点 09:15", at(2026, 9, 21, 9, 15), true},
		{"收盘整点 15:05", at(2026, 9, 21, 15, 5), true},
		{"收盘后 15:06", at(2026, 9, 21, 15, 6), false},
		{"周末(周六)10:00", at(2026, 9, 19, 10, 0), false},
	}
	for _, c := range cases {
		if got := inSession(c.now, start, end, has); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}

	// has=false:只判工作日,不限时段。
	if !inSession(at(2026, 9, 21, 3, 0), 0, 0, false) {
		t.Error("no-session: weekday any hour should pass")
	}
	if inSession(at(2026, 9, 19, 10, 0), 0, 0, false) {
		t.Error("no-session: weekend should still skip")
	}
}

func TestParseSession(t *testing.T) {
	for _, s := range []string{"", "bogus", "9-17"} {
		if _, _, ok := parseSession(s); ok {
			t.Errorf("parseSession(%q): want invalid", s)
		}
	}
	s, e, ok := parseSession("09:15-15:05")
	if !ok || s != 9*60+15 || e != 15*60+5 {
		t.Errorf("parseSession: got (%d,%d,%v)", s, e, ok)
	}
}

// TestResolveSpecsNewsExcludesAnnouncement 常驻模式只跑快讯源(不含公告);all 含公告。
func TestResolveSpecsNewsExcludesAnnouncement(t *testing.T) {
	news, err := resolveSpecs("news", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(news) != 6 {
		t.Fatalf("news group should have 6 sources, got %d", len(news))
	}
	for _, sp := range news {
		if sp.SourceType != "news" {
			t.Fatalf("news group leaked non-news source: %s/%s", sp.Driver, sp.Name)
		}
	}
	all, err := resolveSpecs("all", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 7 {
		t.Fatalf("all group should have 7 sources (6 news + 1 announcement), got %d", len(all))
	}
}

// TestClassifyInsert 钉住 #64 的记账分类(尤其「err 优先于 ok」的次序)。
//
// 缺陷形态:`ok=false, err!=nil` 若先看 ok 会落进「重复」桶 —— 真实插入错误被
// 静默计成 dup,于是 123/123 全失败也算「无失败」→ success / exit 0。
func TestClassifyInsert(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
		err  error
		want insertOutcome
	}{
		{"新入库", true, nil, insertNew},
		{"命中去重", false, nil, insertDup},
		{"插入报错", false, errors.New("boom"), insertFail},
		// 关键反证:带错误的插入必须判失败,不能被 ok 值带偏。
		{"报错且 ok=false(不得当重复)", false, errors.New("boom"), insertFail},
	}
	for _, c := range cases {
		if got := classifyInsert(c.ok, c.err); got != c.want {
			t.Errorf("%s: classifyInsert(%v, %v) = %d, want %d", c.name, c.ok, c.err, got, c.want)
		}
	}
}
