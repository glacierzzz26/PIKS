package ths

// code → 股票名。**同花顺自选两接口都不给名**(实测),而 PIKS entities 有
// UNIQUE(type,name) 必须有名字。这里用同花顺 realhead 端点补齐(同一上游、
// 无签名、沪深皆通,2026-09-22 实测)。
//
// 端点:GET https://d.10jqka.com.cn/v6/realhead/hs_<code>/last.js
//      → quotebridge_v6_realhead_hs_600519_last({"items":{"name":"贵州茅台", …}})
//
// ⚠️ 取名是**尽力而为**:取不到就返回 ("", false),由调用方决定 deferred(下轮再试),
// 绝不拿 code 当 name 建实体(会污染 (type,name) 唯一键与 ⌘C 跳转)。

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// jsonUnmarshal 局部别名,便于 name.go 与 parse.go 风格一致(仅此处用)。
func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// realheadJSON 抽取 last.js 里 JSONP 包裹的 JSON 对象(外层是 fn({...}) )。
var realheadJSON = regexp.MustCompile(`(?s)\((\{.*\})\)`)

// StockName 取单只股票名。取不到 → ("", false, nil)(非致命,不报错)。
func (c *Client) StockName(ctx context.Context, code string) (string, bool, error) {
	m, err := c.StockNames(ctx, []string{code})
	if err != nil {
		return "", false, err
	}
	n, ok := m[code]
	return n, ok, nil
}

// StockNames 批量取名(逐只 GET,内部被限频 —— 自选 40 只也就 ~40s,一天 3 次可接受)。
// 单只失败不中断整批(尽力而为);整体网络错误才返回 error。
func (c *Client) StockNames(ctx context.Context, codes []string) (map[string]string, error) {
	out := make(map[string]string, len(codes))
	var lastErr error
	okCount := 0
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		u := fmt.Sprintf(realheadURL, code)
		body, err := c.getText(ctx, u, map[string]string{
			"Referer": "https://stockpage.10jqka.com.cn/",
		})
		if err != nil {
			lastErr = err
			continue
		}
		if name, ok := parseRealheadName(body); ok {
			out[code] = name
			okCount++
		}
	}
	if okCount == 0 && lastErr != nil {
		return out, lastErr
	}
	return out, nil
}

// parseRealheadName 从 JSONP 文本抽 items.name(纯函数,fixture 可测)。
func parseRealheadName(body []byte) (string, bool) {
	m := realheadJSON.FindSubmatch(body)
	if m == nil {
		return "", false
	}
	var payload struct {
		Items struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := jsonUnmarshal(m[1], &payload); err != nil {
		return "", false
	}
	name := strings.TrimSpace(payload.Items.Name)
	if name == "" {
		return "", false
	}
	return name, true
}
