package web

import "testing"

// gradeMatch 级别过滤(issue #68 A 层)。
//
// 红线:未分级的行(grade=NULL,即本迁移前的历史公告)**不得从任何级别视图里消失**。
// 后端把 NULL 按「常规」对待,与前端把 NULL 显示为常规一致 —— 宁可多显示,不可误隐藏。
func TestGradeMatch(t *testing.T) {
	cases := []struct {
		row, filter string
		want        bool
		why         string
	}{
		{"must", "must", true, "同级别匹配"},
		{"must", "routine", false, "不同级别不匹配"},
		{"routine", "routine", true, "常规匹配常规"},
		{"", "routine", true, "未分级(NULL)按常规对待 —— 历史行不能在常规视图里消失"},
		{"", "must", false, "未分级不冒充必读"},
		{"", "noise", false, "未分级不冒充噪音(避免被默认折叠)"},
		// 空过滤由调用方短路(handleAPIAnnouncements 里 grd=="" 直接放行),
		// 故 gradeMatch 不负责「空=不过滤」;此处只钉「非空行对空过滤不匹配」。
		{"important", "", false, "空过滤不由本函数承担(调用方短路),故返回 false"},
	}
	for _, c := range cases {
		got := gradeMatch(c.row, c.filter)
		if got != c.want {
			t.Errorf("gradeMatch(%q,%q) = %v, want %v —— %s", c.row, c.filter, got, c.want, c.why)
		}
	}
}

// TestGradeMatchNeverHidesUngraded 把红线单独钉一条:遍历所有具体级别过滤器,
// 未分级行必须**在且仅在** routine 视图里可见(即不会被"藏起来")。
func TestGradeMatchNeverHidesUngraded(t *testing.T) {
	visible := 0
	for _, f := range []string{"must", "important", "routine", "noise"} {
		if gradeMatch("", f) {
			visible++
			if f != "routine" {
				t.Errorf("未分级行在 %q 视图可见,但只应在 routine 视图可见", f)
			}
		}
	}
	if visible != 1 {
		t.Errorf("未分级行应恰在 1 个级别视图可见, got %d —— 若为 0 则历史公告永久消失", visible)
	}
}
