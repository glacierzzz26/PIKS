package collector

// clsDriver 财联社(CLS)电报驱动(纯 HTTP,非 akshare)。
//
// akshare 的 get_roll_list 封装把 DataFrame 列切成 [标题/内容/日期/时间],**丢掉了
// shareurl 与 reading_num**(issue #43 §重大修正),故自建驱动读原始 JSON。
//
// 端点与字段经本次实测(2026-09-20):
//   GET https://www.cls.cn/v1/roll/get_roll_list?app=CailianpressWeb&category=&last_time=&os=web&refresh_type=1&rn=20&sv=8.4.6&sign=<签名>
//   Header: Referer: https://www.cls.cn/
//   → data.roll_data[].{id,title,content,ctime(unix 秒),shareurl,reading_num,level,confirmed,stock_list,subjects,tags}
//
// ⚠️ 签名算法(实测破解,勿改):sign = md5( sha1( 参数按 key 升序拼 "k=v&k=v" ) )。
//   先 sha1 取 hex 小写,再对其 hex 串做 md5 取 hex 小写;两层缺一不可,且必须排序。
//
// 上游独有且必须留档的信号(issue #43):reading_num(阅读数=真实公众关注度)、
// level(A/B/C 分级)、confirmed(确认标志)、shareurl(原文链接)、stock_list(关联个股)。

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const clsFeedURL = "https://www.cls.cn/v1/roll/get_roll_list"

// clsParams 固定查询参数(实测可用的最小集);rn 为条数。
var clsBaseParams = map[string]string{
	"app":          "CailianpressWeb",
	"category":     "",
	"last_time":    "",
	"os":           "web",
	"refresh_type": "1",
	"sv":           "8.4.6",
}

type clsDriver struct {
	http *httpSource
	rn   int
}

func newCLSDriver() *clsDriver {
	return &clsDriver{
		http: newHTTPSource(15*time.Second, 2*time.Second, 3),
		rn:   20, // 上游单页上限实测 20
	}
}

func (d *clsDriver) Name() string { return "cls" }

// clsResp 真实 DTO(实测 2026-09-20);stock_list/subjects 用 map 承接以原样留档。
type clsResp struct {
	Errno json.RawMessage `json:"errno"` // 成功时为数字 0,失败时为字符串 "10012"
	Msg   string          `json:"msg"`
	Data  *clsData        `json:"data"`
}

type clsData struct {
	RollData []clsItem `json:"roll_data"`
}

type clsItem struct {
	ID         int64            `json:"id"`
	Title      string           `json:"title"`
	Content    string           `json:"content"`
	Brief      string           `json:"brief"`
	CTime      int64            `json:"ctime"`       // unix 秒
	ShareURL   string           `json:"shareurl"`    // 原文链接(akshare 丢弃)
	ReadingNum int64            `json:"reading_num"` // 阅读数(akshare 丢弃)
	Level      string           `json:"level"`       // A/B/C 分级(akshare 丢弃)
	Confirmed  int              `json:"confirmed"`   // 确认标志(akshare 丢弃)
	StockList  []map[string]any `json:"stock_list"`  // 关联个股
	Tags       []map[string]any `json:"tags"`
	Subjects   []map[string]any `json:"subjects"`
}

func (d *clsDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	body, err := d.http.getJSON(ctx, d.signedURL(), map[string]string{
		"Referer": "https://www.cls.cn/",
	})
	if err != nil {
		return nil, fmt.Errorf("cls: %w", err)
	}
	var r clsResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("cls: bad json: %w", err)
	}
	if !clsOK(r.Errno) {
		return nil, fmt.Errorf("cls: errno=%s msg=%s", string(r.Errno), r.Msg)
	}
	if r.Data == nil {
		observeFetch(clsFeedURL, 0)
		return nil, nil
	}
	out := normalizeCLS(r.Data.RollData)
	observeFetch(clsFeedURL, len(out))
	return out, nil
}

// signedURL 拼查询串并附签名。参数每次重建(签名对参数集敏感)。
func (d *clsDriver) signedURL() string {
	p := make(map[string]string, len(clsBaseParams)+2)
	for k, v := range clsBaseParams {
		p[k] = v
	}
	p["rn"] = fmt.Sprintf("%d", d.rn)
	p["sign"] = clsSign(p)

	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+p[k])
	}
	return clsFeedURL + "?" + strings.Join(parts, "&")
}

// clsSign 财联社签名:md5( sha1( 参数按 key 升序拼 "k=v&k=v" ) )。
// 两层哈希均取 hex 小写;不含 sign 本身。本函数经实测校准,改动须重新验证真实接口。
func clsSign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "sign" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	sum1 := sha1.Sum([]byte(strings.Join(parts, "&")))
	hex1 := hex.EncodeToString(sum1[:])
	sum2 := md5.Sum([]byte(hex1))
	return hex.EncodeToString(sum2[:])
}

// clsOK errno 成功判定:上游成功时 "errno":0(数字),失败时为字符串。
func clsOK(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "0" || s == `"0"`
}

// normalizeCLS 真实 DTO → 归一化 RawNews(纯函数,可离线单测)。
func normalizeCLS(items []clsItem) []RawNews {
	out := make([]RawNews, 0, len(items))
	for _, it := range items {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			content = strings.TrimSpace(it.Brief)
		}
		if content == "" {
			continue
		}
		out = append(out, RawNews{
			ExternalID:  fmt.Sprintf("%d", it.ID),
			URL:         it.ShareURL,
			Title:       clsTitle(it, content),
			Content:     content,
			PublishedAt: unixPtr(it.CTime),
			Extra: toRaw(map[string]any{
				"reading_num": it.ReadingNum,
				"level":       it.Level,
				"confirmed":   it.Confirmed,
				"stock_list":  it.StockList,
				"tags":        it.Tags,
				"subjects":    it.Subjects,
			}),
		})
	}
	return out
}

// clsTitle 上游 title 常为空(正文以【标题】开头);为空时从正文【】内取,再退化为前段。
func clsTitle(it clsItem, content string) string {
	if t := strings.TrimSpace(it.Title); t != "" {
		return t
	}
	if t := bracketTitle(content); t != "" {
		return t
	}
	return truncateRunes(content, 40)
}

// bracketTitle 取中文方括号【】内的标题(财联社/金十正文惯用格式)。
func bracketTitle(s string) string {
	start := strings.Index(s, "【")
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start:], "】")
	if end <= len("【")-1 {
		return ""
	}
	return s[start+len("【") : start+end]
}
