package collector

// futuDriver 富途牛牛(富途资讯)7x24 快讯驱动(纯 HTTP)。
//
// akshare 的 stock_info_global_futu 实测返回 4 列 [标题/内容/发布时间/链接],**含 url**;
// 但丢掉 quote(关联个股+实时行情)/relatedStocks/level/newsType 等。
// 按 issue #43 §红线,自建驱动读原始 JSON,关联个股与分级一并留档。
//
// 端点与字段经本次实测(2026-09-20):
//   GET https://news.futunn.com/news-site-api/main/get-flash-list?pageSize=50&lang=0
//   → data.data.news[].{id,title,content,time(unix 秒),detailUrl,level,newsType,relatedStocks,quote}
//   响应被包两层 data.data(实测)。
//
// ⚠️ 富途偏港股/美股,是**海外视角**的第二来源;A 股事件覆盖弱于金十/财联社,但
//    url 与关联个股完整,适合做「同事件是否存在海外报道」的对照源。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const futuFeedURL = "https://news.futunn.com/news-site-api/main/get-flash-list"

type futuDriver struct {
	http     *httpSource
	pageSize int
}

func newFutuDriver() *futuDriver {
	return &futuDriver{
		http:     newHTTPSource(15*time.Second, 2*time.Second, 3),
		pageSize: 50,
	}
}

func (d *futuDriver) Name() string { return "futu" }

// futuResp 真实 DTO(实测 2026-09-20):data.data.news[] 两层包装。
type futuResp struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    *futuOuterData `json:"data"`
}

type futuOuterData struct {
	Data *futuInnerData `json:"data"`
}

type futuInnerData struct {
	News []futuItem `json:"news"`
}

type futuItem struct {
	ID            string           `json:"id"`
	Title         string           `json:"title"`
	Content       string           `json:"content"`
	Time          string           `json:"time"`      // unix 秒(字符串)
	DetailURL     string           `json:"detailUrl"` // 原文链接
	Level         int              `json:"level"`     // 0 = 普通
	NewsType      int              `json:"newsType"`
	RelatedStocks []string         `json:"relatedStocks"`
	Quote         []map[string]any `json:"quote"` // 关联个股 + 实时行情
}

func (d *futuDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	url := fmt.Sprintf("%s?pageSize=%d&lang=0", futuFeedURL, d.pageSize)
	body, err := d.http.getJSON(ctx, url, map[string]string{
		"Referer": "https://news.futunn.com/main/live",
	})
	if err != nil {
		return nil, fmt.Errorf("futu: %w", err)
	}
	var r futuResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("futu: bad json: %w", err)
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("futu: code=%d msg=%s", r.Code, r.Message)
	}
	if r.Data == nil || r.Data.Data == nil {
		return nil, nil
	}
	return normalizeFutu(r.Data.Data.News), nil
}

// normalizeFutu 真实 DTO → 归一化 RawNews(纯函数,可离线单测)。
func normalizeFutu(items []futuItem) []RawNews {
	out := make([]RawNews, 0, len(items))
	for _, it := range items {
		content := NormalizeContent(it.Title + " " + it.Content)
		if content == "" {
			continue
		}
		out = append(out, RawNews{
			ExternalID:  it.ID,
			URL:         it.DetailURL,
			Title:       it.Title,
			Content:     content,
			PublishedAt: unixPtr(parseInt64(it.Time)),
			Extra: toRaw(map[string]any{
				"level":          it.Level,
				"news_type":      it.NewsType,
				"related_stocks": it.RelatedStocks,
				"quote":          it.Quote,
			}),
		})
	}
	return out
}
