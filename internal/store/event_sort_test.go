package store

import (
	"strings"
	"testing"
)

// TestEventOrderByTimeIsDescendingCOALESCE 回归 issue #37 缺陷:
// 旧查询 `ORDER BY occurred_at NULLS LAST, created_at` 是**升序** —— 消息页
// 「重要消息」首行是最老的一条(最新沉底)。修复后默认必须时间倒序,
// 且口径统一为 COALESCE(occurred_at, created_at)(occurred_at 可为空)。
func TestEventOrderByTimeIsDescendingCOALESCE(t *testing.T) {
	for _, s := range []string{"", EventSortTime} {
		got := eventOrderBy(s)
		if !strings.Contains(got, "COALESCE(e.occurred_at, e.created_at) DESC") {
			t.Errorf("sort=%q 应为时间倒序 + COALESCE 口径,得 %q", s, got)
		}
		if strings.Contains(got, "NULLS LAST") {
			t.Errorf("sort=%q 不应再依赖 NULLS LAST(空值须按 created_at 参与排序),得 %q", s, got)
		}
	}
}

// TestEventOrderByConfidence 置信度排序必须倒序,且带时间兜底键保证分页稳定。
func TestEventOrderByConfidence(t *testing.T) {
	got := eventOrderBy(EventSortConfidence)
	if !strings.HasPrefix(got, "ORDER BY e.confidence DESC") {
		t.Errorf("置信度排序应倒序,得 %q", got)
	}
	if !strings.Contains(got, "COALESCE(e.occurred_at, e.created_at) DESC") {
		t.Errorf("置信度排序应带时间兜底键(同分稳定),得 %q", got)
	}
}

// TestEventOrderByUnknownFallsBackToTime 未知/空 sort 一律回落时间倒序,不拼接任意输入。
func TestEventOrderByUnknownFallsBackToTime(t *testing.T) {
	if got, want := eventOrderBy("'; DROP TABLE events; --"), eventOrderBy(EventSortTime); got != want {
		t.Errorf("未知 sort 应回落默认,得 %q want %q", got, want)
	}
}

// TestFlashOrderBy 默认时间倒序;important 以「已被抽取成事件」为代理优先。
func TestFlashOrderBy(t *testing.T) {
	if got := flashOrderBy(""); !strings.Contains(got, "flash_at DESC") {
		t.Errorf("快讯默认应时间倒序,得 %q", got)
	}
	got := flashOrderBy(FlashSortImportant)
	if !strings.Contains(got, "(event_id IS NOT NULL) DESC") {
		t.Errorf("重要优先应以 event_id 非空作代理,得 %q", got)
	}
	// 重要优先内部仍按时间倒序,否则每天内顺序随机。
	if !strings.Contains(got, "flash_at DESC") {
		t.Errorf("重要优先应保留时间倒序兜底键,得 %q", got)
	}
}
