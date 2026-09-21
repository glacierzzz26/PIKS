package collector

// sinaDriver 新浪财经 7x24 直播驱动(纯 HTTP,非 akshare)。
//
// akshare 的 zhibo 封装只留 [时间/内容],**丢掉 docurl 与 like_nums**(issue #43 §重大修正),
// 故自建驱动读原始 JSON。
//
// 端点与字段经本次实测(2026-09-20):
//   GET https://zhibo.sina.com.cn/api/zhibo/feed?page=1&page_size=20&zhibo_id=152&tag_id=0&dire=f&dpc=1
//   → result.data.feed.list[].{id,rich_text,create_time,docurl,like_nums,tag,ext}
//   ext 为**字符串化 JSON**,内含 docurl(finance.sina.com.cn 版)与 docid。
//
// 上游独有信号(issue #43):docurl(原文链接)、like_nums(点赞=弱热度信号)、tag(分类)。
// ⚠️ 正文是 rich_text,含 HTML 片段与多余空白,入库前须归一。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const sinaFeedURL = "https://zhibo.sina.com.cn/api/zhibo/feed"

type sinaDriver struct {
	http     *httpSource
	pageSize int
}

func newSinaDriver() *sinaDriver {
	return &sinaDriver{
		http:     newHTTPSource(15*time.Second, 2*time.Second, 3),
		pageSize: 20,
	}
}

func (d *sinaDriver) Name() string { return "sina" }

// sinaResp 真实 DTO(实测 2026-09-20)。tag/ext 用 map / string 承接以原样留档。
type sinaResp struct {
	Result *sinaResult `json:"result"`
}

type sinaResult struct {
	Status *sinaStatus `json:"status"`
	Data   *sinaData   `json:"data"`
}

type sinaStatus struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

type sinaData struct {
	Feed *sinaFeed `json:"feed"`
}

type sinaFeed struct {
	List []sinaItem `json:"list"`
}

type sinaItem struct {
	ID         int64            `json:"id"`
	RichText   string           `json:"rich_text"`
	CreateTime string           `json:"create_time"` // "2006-01-02 15:04:05"(北京时间)
	DocURL     string           `json:"docurl"`      // 手机版原文链接(finance.sina.cn)
	LikeNums   int64            `json:"like_nums"`   // 点赞数(弱热度信号)
	Tag        []map[string]any `json:"tag"`
	Ext        string           `json:"ext"` // 字符串化 JSON:{"docurl":"…","docid":"…","stocks":[…]}
}

func (d *sinaDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	url := fmt.Sprintf("%s?page=1&page_size=%d&zhibo_id=152&tag_id=0&dire=f&dpc=1",
		sinaFeedURL, d.pageSize)
	body, err := d.http.getJSON(ctx, url, map[string]string{
		"Referer": "https://finance.sina.com.cn/7x24/",
	})
	if err != nil {
		return nil, fmt.Errorf("sina: %w", err)
	}
	var r sinaResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("sina: bad json: %w", err)
	}
	if r.Result == nil || r.Result.Status == nil {
		return nil, fmt.Errorf("sina: empty result")
	}
	if r.Result.Status.Code != 0 {
		return nil, fmt.Errorf("sina: code=%d msg=%s", r.Result.Status.Code, r.Result.Status.Msg)
	}
	if r.Result.Data == nil || r.Result.Data.Feed == nil {
		observeFetch(sinaFeedURL, 0)
		return nil, nil
	}
	out := normalizeSina(r.Result.Data.Feed.List)
	observeFetch(sinaFeedURL, len(out))
	return out, nil
}

// normalizeSina 真实 DTO → 归一化 RawNews(纯函数,可离线单测)。
func normalizeSina(items []sinaItem) []RawNews {
	out := make([]RawNews, 0, len(items))
	for _, it := range items {
		content := NormalizeContent(stripHTML(it.RichText))
		if content == "" {
			continue
		}
		out = append(out, RawNews{
			ExternalID:  fmt.Sprintf("%d", it.ID),
			URL:         sinaURL(it),
			Title:       truncateRunes(content, 40),
			Content:     content,
			PublishedAt: parseCNTime(it.CreateTime),
			Extra: toRaw(map[string]any{
				"like_nums":  it.LikeNums,
				"tag":        it.Tag,
				"docurl":     it.DocURL,
				"ext_docurl": sinaExtDocURL(it.Ext), // finance.sina.com.cn 版原文
			}),
		})
	}
	return out
}

// sinaURL 优先用 feed 顶层 docurl(实测为 finance.sina.cn 手机版,直接可点)。
func sinaURL(it sinaItem) string {
	if u := strings.TrimSpace(it.DocURL); u != "" {
		return u
	}
	return sinaExtDocURL(it.Ext)
}

// sinaExtDocURL 解析 ext 字符串化 JSON 里的 docurl(finance.sina.com.cn 电脑版)。
// ext 非法 JSON 时如实返回空,不猜测。
func sinaExtDocURL(ext string) string {
	ext = strings.TrimSpace(ext)
	if ext == "" || ext[0] != '{' {
		return ""
	}
	var m struct {
		DocURL string `json:"docurl"`
	}
	if err := json.Unmarshal([]byte(ext), &m); err != nil {
		return ""
	}
	return strings.TrimSpace(m.DocURL)
}

// stripHTML 去掉 rich_text 里的 HTML 标签(实测含 <a> 等片段),保留文本。
// 简易扫描:标签内跳过,其余原样 —— 不引入 HTML 库,避免新依赖。
func stripHTML(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unixPtr unix 秒 → *time.Time(UTC);0 视为缺失返回 nil(不造 1970 年假时间)。
func unixPtr(sec int64) *time.Time {
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

// truncateRunes 按字符(非字节)截断,避免截断多字节汉字。
func truncateRunes(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n])
}
