package collector

import (
	"os"
	"testing"
	"time"
)

// 归一化离线单测:字段取自 cmd/probe/dongcai 实测的真实响应(2026-08-26),勿臆造。
func TestNormalizeZhibo(t *testing.T) {
	items := []dongcaiItem{
		{
			Code:     "202608263855162531",
			Title:    "来伊份上半年净利亏损扩大至约9213.05万元，营收同比下降6.6%",
			ShowTime: "2026-08-26 22:35:31",
			RealSort: "1787754931062531",
			Summary:  nil,
		},
		{
			Code:     "202608263855102564",
			Title:    "美股三大指数小幅下跌 国际油价跌幅收窄",
			ShowTime: "2026-08-26 22:38:11",
			Summary:  ptrStr("国际油价跌约1%"),
		},
	}
	got := normalizeZhibo(items)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	a := got[0]
	if a.ExternalID != "202608263855162531" {
		t.Errorf("ExternalID=%q", a.ExternalID)
	}
	if a.URL != "https://finance.eastmoney.com/a/202608263855162531.html" {
		t.Errorf("URL=%q", a.URL)
	}
	if a.Title != items[0].Title {
		t.Errorf("Title=%q", a.Title)
	}
	if a.Content != items[0].Title { // summary=nil → 正文=标题
		t.Errorf("Content=%q", a.Content)
	}
	want, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-08-26 22:35:31", time.FixedZone("CST", 8*3600))
	if a.PublishedAt == nil || !a.PublishedAt.Equal(want) {
		t.Errorf("PublishedAt=%v want %v", a.PublishedAt, want)
	}
	if got[1].Content != "美股三大指数小幅下跌 国际油价跌幅收窄 国际油价跌约1%" {
		t.Errorf("Content(with summary)=%q", got[1].Content)
	}
}

func ptrStr(s string) *string { return &s }

// TestDongcaiLiveFetch 实况探针:验证真实驱动能联网拉到快讯并归一化。
// 联网测试,默认跳过;PIKS_LIVE_NET=1 时执行(如每日采集自检)。
func TestDongcaiLiveFetch(t *testing.T) {
	if os.Getenv("PIKS_LIVE_NET") != "1" {
		t.Skip("PIKS_LIVE_NET=1 未设置,跳过联网测试")
	}
	items, err := newDongcaiDriver().Fetch(t.Context())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("fetch returned 0 items")
	}
	first := items[0]
	if len(first.ExternalID) < 8 {
		t.Errorf("ExternalID 异常: %q", first.ExternalID)
	}
	if first.Title == "" {
		t.Error("title 为空")
	}
	if first.URL == "" {
		t.Error("url 为空")
	}
	if first.PublishedAt == nil {
		t.Error("publishedAt 为空")
	}
}
