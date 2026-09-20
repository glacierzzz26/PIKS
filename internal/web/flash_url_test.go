package web

import (
	"testing"
	"time"

	"piks/internal/store"
)

// TestToFlashCarriesURL 回归 issue #38:快讯的原文 URL 此前**未下发**
// (RawDocWithSource / apiFlash 都没有 url 字段),来源无从可点。
func TestToFlashCarriesURL(t *testing.T) {
	url := "https://finance.eastmoney.com/a/2026082630001.html"
	got := toFlash(store.RawDocWithSource{
		ID:      "f1",
		FlashAt: time.Date(2026, 8, 26, 14, 40, 0, 0, cst),
		Title:   "某快讯",
		Source:  "东财快讯",
		URL:     &url,
	})
	if got.URL != url {
		t.Errorf("url = %q, want %q", got.URL, url)
	}
}

// TestToFlashURLDegradesToEmpty 无 url 的老数据必须给空串 → 前端退化为纯文本,
// 不渲染空 href 死链(issue #38 边界)。
func TestToFlashURLDegradesToEmpty(t *testing.T) {
	got := toFlash(store.RawDocWithSource{
		ID:      "f2",
		FlashAt: time.Date(2026, 8, 26, 14, 40, 0, 0, cst),
		Title:   "无原文链接的老快讯",
		Source:  "东财快讯",
		URL:     nil,
	})
	if got.URL != "" {
		t.Errorf("nil url 应为空串,得 %q", got.URL)
	}
}
