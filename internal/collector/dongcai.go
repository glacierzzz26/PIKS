package collector

// dongcaiDriver 东方财富 7x24 快讯驱动(网页内部接口,非官方、无 SLA)。
// 端点与字段经 cmd/probe/dongcai 实测(迭代1 G1,设计 §3.5),勿凭想象改字段:
//   GET https://np-weblist.eastmoney.com/comm/web/getFastNewsZhibo
//   ?client=web&biz=web_724&sortEnd={游标}&pageSize={每页数}
//   → data.fastNewsList[].{code,title,showTime,summary,stockList,...}
//   data.sortEnd 为下一页游标;showTime 为北京时间字符串;详情页 URL 规律 /a/{code}.html。
// 归一化:ExternalID=code,URL=/a/{code}.html,Content=title[+summary],PublishedAt=解析 showTime。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const dongcaiFeedURL = "https://np-weblist.eastmoney.com/comm/web/getFastNewsZhibo"

type dongcaiDriver struct {
	client   *http.Client
	pageSize int
	maxPages int // 分页上限;time 游标(sortEnd)确保不重复拉取
}

func newDongcaiDriver() *dongcaiDriver {
	return &dongcaiDriver{
		client:   &http.Client{Timeout: 15 * time.Second},
		pageSize: 50,
		maxPages: 1, // 单次运行一页(约50条)足够;调大可拉更多历史
	}
}

func (d *dongcaiDriver) Name() string { return "dongcai" }

// dongcaiResp / dongcaiData / dongcaiItem 与 cmd/probe/dongcai.go 同一套真实 DTO。
type dongcaiResp struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Data    *dongcaiData `json:"data"`
}
type dongcaiData struct {
	SortEnd       string        `json:"sortEnd"`
	Index         int           `json:"index"`
	Total         int           `json:"total"`
	Size          int           `json:"size"`
	FastNewsList []dongcaiItem `json:"fastNewsList"`
}
type dongcaiItem struct {
	Summary    *string  `json:"summary"`
	Code       string   `json:"code"`
	RealSort   string   `json:"realSort"`
	ShowTime   string   `json:"showTime"`
	Title      string   `json:"title"`
	StockList  []string `json:"stockList"`
}

// Fetch 走 sortEnd 游标翻页,归一化到 RawNews。部分失败不中断:本页出错则终止并返回已取数据。
func (d *dongcaiDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	var out []RawNews
	sortEnd := ""
	for p := 0; p < d.maxPages; p++ {
		page, next, err := d.fetchPage(ctx, sortEnd)
		if err != nil {
			return out, err
		}
		out = append(out, page...)
		if next == "" || len(page) == 0 {
			break
		}
		sortEnd = next
	}
	return out, nil
}

func (d *dongcaiDriver) fetchPage(ctx context.Context, sortEnd string) ([]RawNews, string, error) {
	url := fmt.Sprintf("%s?client=web&biz=web_724&sortEnd=%s&pageSize=%d&req_trace=collector",
		dongcaiFeedURL, sortEnd, d.pageSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("dongcai: HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}
	var r dongcaiResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "", fmt.Errorf("dongcai: bad json: %v", err)
	}
	if r.Data == nil || len(r.Data.FastNewsList) == 0 {
		return nil, "", nil
	}
	return normalizeZhibo(r.Data.FastNewsList), r.Data.SortEnd, nil
}

// normalizeZhibo 真实 DTO → 归一化 RawNews(纯函数,可离线单测)。
func normalizeZhibo(items []dongcaiItem) []RawNews {
	out := make([]RawNews, 0, len(items))
	for _, it := range items {
		out = append(out, RawNews{
			ExternalID:  it.Code,
			URL:         "https://finance.eastmoney.com/a/" + it.Code + ".html",
			Title:       it.Title,
			Content:     dongcaiContent(it.Title, it.Summary),
			PublishedAt: parseCNTime(it.ShowTime),
		})
	}
	return out
}

// dongcaiContent 快讯列表只有标题(摘要常为 null),正文取 title[+summary] 作为抽取输入。
func dongcaiContent(title string, summary *string) string {
	if summary == nil || strings.TrimSpace(*summary) == "" {
		return title
	}
	return title + " " + strings.TrimSpace(*summary)
}

// parseCNTime 东财 showTime 为北京时间("2006-01-02 15:04:05"),固定 +08:00 解析为 UTC 存库。
func parseCNTime(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.FixedZone("CST", 8*3600))
	if err != nil {
		return nil
	}
	return &t
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n]
	}
	return s
}
