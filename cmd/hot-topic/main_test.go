package main

import (
	"testing"
	"time"
)

// 时段闸与源清单单测(issue #68 D 层)。与 cmd/collector 同构 —— 但本命令频率不同(30m),
// 时段闸逻辑逐字一致,故测试也逐字对应(两命令各自独立,不跨包共享)。

func TestInSessionHotTopic(t *testing.T) {
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
}

func TestParseSessionHotTopic(t *testing.T) {
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
