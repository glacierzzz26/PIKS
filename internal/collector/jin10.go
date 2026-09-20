package collector

// jin10Driver 金十数据 7x24 快讯驱动(纯 HTTP,非 akshare —— akshare 内无快讯接口)。
// 端点与字段经本次实测(2026-09-20,issue #43):
//   GET https://flash-api.jin10.com/get_flash_list?channel=-8200&vip=1
//   Header: x-app-id / x-version(网页前端同款,非用户凭据)
//   → data[].{id,time,type,important,tags,channel,data:{content,title,source,source_link,pic}}
//
// 相对 akshare 的独有价值(issue #43 §金十三个额外价值):
//   1. important 自带重要度 → 「消息(重要)」判据(此前只靠 LLM 抽取判定);
//   2. data.source 标注一级源(新华社/央视/上证报…)= 免费溯源链;
//   3. 时效性强,盘中密度高。
//
// 归一化:ExternalID=id,Content=title+content,PublishedAt=解析 time;
// 上游原始字段(important/source/source_link/tags/channel/type)全量落 extra。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const jin10FeedURL = "https://flash-api.jin10.com/get_flash_list?channel=-8200&vip=1"

// 网页前端同款固定标识(公开常量,非用户凭据)。
const (
	jin10AppID   = "bVBF4FyRTn5NJF5n"
	jin10Version = "1.0.0"
)

type jin10Driver struct {
	http *httpSource
}

func newJin10Driver() *jin10Driver {
	return &jin10Driver{
		// 免费源无 SLA:请求间隔 2s,最多 3 次退避。
		http: newHTTPSource(15*time.Second, 2*time.Second, 3),
	}
}

func (d *jin10Driver) Name() string { return "jin10" }

// jin10Resp 真实 DTO(实测 2026-09-20)。
// data[].data 用 map 承接以原样留档(字段随 type 变化:type=2 为数据类,另有 attr 等)。
type jin10Resp struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    []jin10Item `json:"data"`
}

type jin10Item struct {
	ID        string         `json:"id"`
	Time      string         `json:"time"`
	Type      int            `json:"type"`
	Important int            `json:"important"`
	Tags      []string       `json:"tags"`
	Channel   []int          `json:"channel"`
	Data      map[string]any `json:"data"`
}

func (d *jin10Driver) Fetch(ctx context.Context) ([]RawNews, error) {
	body, err := d.http.getJSON(ctx, jin10FeedURL, map[string]string{
		"x-app-id":  jin10AppID,
		"x-version": jin10Version,
		"Referer":   "https://www.jin10.com/",
	})
	if err != nil {
		return nil, fmt.Errorf("jin10: %w", err)
	}
	var r jin10Resp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("jin10: bad json: %w", err)
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("jin10: status=%d %s", r.Status, r.Message)
	}
	return normalizeJin10(r.Data), nil
}

// normalizeJin10 真实 DTO → 归一化 RawNews(纯函数,可离线单测)。
func normalizeJin10(items []jin10Item) []RawNews {
	out := make([]RawNews, 0, len(items))
	for _, it := range items {
		content := jin10Content(it.Data)
		if content == "" {
			continue // 无正文(纯图片/空条目)不入库,不造空文档
		}
		out = append(out, RawNews{
			ExternalID:  it.ID,
			URL:         jin10URL(it),
			Title:       jin10Title(it.Data, content),
			Content:     content,
			PublishedAt: parseCNTime(it.Time),
			Extra: toRaw(map[string]any{
				"important":   it.Important,
				"type":        it.Type,
				"tags":        it.Tags,
				"channel":     it.Channel,
				"source":      jsonGet(it.Data, "source"),      // 一级源(新华社/央视…)
				"source_link": jsonGet(it.Data, "source_link"), // 一级源原文链接
				"pic":         jsonGet(it.Data, "pic"),
			}),
		})
	}
	return out
}

// jin10Content 正文:title 为空的条目内容即 content;非空时标题已含在 content 的【】里,
// 直接用 content(避免重复拼接 —— 实测 title 恒为空,留此分支防御 type=2 数据类)。
func jin10Content(data map[string]any) string {
	c := jsonGet(data, "content")
	t := jsonGet(data, "title")
	if t == "" || c == "" {
		return c
	}
	return t + " " + c
}

// jin10Title 标题取上游 title(实测恒空);为空时从正文【】内取(金十惯用格式),
// 再退化为正文前段,保证列表可读。
func jin10Title(data map[string]any, content string) string {
	if t := jsonGet(data, "title"); t != "" {
		return t
	}
	if t := bracketTitle(content); t != "" {
		return t
	}
	return truncateRunes(content, 40)
}

// jin10URL 金十快讯无逐条原文 URL(实测 detail 页为 hash 路由,GET 返回 404)。
// **不造假链接**:该源如实返回空,由调用方决定是否留空(issue #43 验收:确无则留空)。
// 一级源链接 source_link 若存在,优先作为可点原文(真实外链)。
func jin10URL(it jin10Item) string {
	return jsonGet(it.Data, "source_link")
}
