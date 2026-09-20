package collector

// thsDriver 同花顺 7x24 快讯驱动(纯 HTTP)。
//
// akshare 的 stock_info_global_ths 实测返回 4 列 [标题/内容/发布时间/链接],**含 url**;
// 但**丢掉** tagInfo(主题标签+相关度)/stock(关联个股)/color(涨跌色)/nature 等。
// 按 issue #43 §红线「akshare 会丢字段」,仍自建驱动读原始 JSON,热度/分类信号一并留档。
//
// 端点与字段经本次实测(2026-09-20):
//   GET https://news.10jqka.com.cn/tapp/news/push/stock/?page=1&tag=&track=website&pagesize=20
//   → data.list[].{id,seq,title,digest,url,ctime,rtime,tag,tags,tagInfo,stock,color,nature,source}
//   url 为电脑版原文,shareUrl 为分享版;ctime/rtime 为 unix 秒。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const thsFeedURL = "https://news.10jqka.com.cn/tapp/news/push/stock/"

type thsDriver struct {
	http     *httpSource
	pageSize int
}

func newThsDriver() *thsDriver {
	return &thsDriver{
		http:     newHTTPSource(15*time.Second, 2*time.Second, 3),
		pageSize: 20,
	}
}

func (d *thsDriver) Name() string { return "ths" }

// thsResp 真实 DTO(实测 2026-09-20)。tagInfo/stock 用 map 承接以原样留档。
type thsResp struct {
	Code string   `json:"code"` // "200" 为成功(字符串!)
	Msg  string   `json:"msg"`
	Data *thsData `json:"data"`
}

type thsData struct {
	List []thsItem `json:"list"`
}

type thsItem struct {
	ID      string           `json:"id"`
	Seq     string           `json:"seq"`
	Title   string           `json:"title"`
	Digest  string           `json:"digest"`
	URL     string           `json:"url"`
	Share   string           `json:"shareUrl"`
	CTime   string           `json:"ctime"` // unix 秒(字符串)
	Source  string           `json:"source"`
	Tag     string           `json:"tag"` // 逗号分隔的分类("美股,港股,A股")
	Tags    []map[string]any `json:"tags"`
	TagInfo []map[string]any `json:"tagInfo"` // 主题标签 + 相关度 score
	Stock   []map[string]any `json:"stock"`   // 关联个股
	Nature  string           `json:"nature"`
	Color   string           `json:"color"`
}

func (d *thsDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	url := fmt.Sprintf("%s?page=1&tag=&track=website&pagesize=%d", thsFeedURL, d.pageSize)
	body, err := d.http.getJSON(ctx, url, map[string]string{
		"Referer": "https://news.10jqka.com.cn/realtimenews.html",
	})
	if err != nil {
		return nil, fmt.Errorf("ths: %w", err)
	}
	var r thsResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("ths: bad json: %w", err)
	}
	if r.Code != "200" {
		return nil, fmt.Errorf("ths: code=%s msg=%s", r.Code, r.Msg)
	}
	if r.Data == nil {
		return nil, nil
	}
	return normalizeThs(r.Data.List), nil
}

// normalizeThs 真实 DTO → 归一化 RawNews(纯函数,可离线单测)。
func normalizeThs(items []thsItem) []RawNews {
	out := make([]RawNews, 0, len(items))
	for _, it := range items {
		content := NormalizeContent(it.Title + " " + it.Digest)
		if content == "" {
			continue
		}
		out = append(out, RawNews{
			ExternalID:  it.ID,
			URL:         it.URL, // 实测含 url(akshare 也有,但本驱动同时留 tagInfo/stock)
			Title:       it.Title,
			Content:     content,
			PublishedAt: unixPtr(parseInt64(it.CTime)),
			Extra: toRaw(map[string]any{
				"tag":      it.Tag,
				"tags":     it.Tags,
				"tag_info": it.TagInfo,
				"stock":    it.Stock,
				"color":    it.Color,
				"nature":   it.Nature,
				"source":   it.Source,
				"share":    it.Share,
			}),
		})
	}
	return out
}

// parseInt64 宽松解析十进制字符串(unix 秒);非法返回 0(→ unixPtr 得 nil,不造时间)。
func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}
