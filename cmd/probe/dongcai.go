// probe 东财 7x24 快讯接口探针(G1,设计 §3.5)。
// 目的:逐字段对照真实响应 DTO,不凭想象写字段(数据诚实)。
// 用法:go run ./cmd/probe/dongcai -pages 2 -pageSize 5
// 发现:np-weblist.eastmoney.com/comm/web/getFastNewsZhibo 返回 7x24 直播流,
//       sortEnd 作分页游标,字段经本次探针实测。URL 规律 finance.eastmoney.com/a/{code}.html。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const feedURL = "https://np-weblist.eastmoney.com/comm/web/getFastNewsZhibo"

// 真实 DTO(探针实测,勿臆造字段)。
type zhiboResp struct {
	ReqTrace string     `json:"req_trace"`
	Code     string     `json:"code"`
	Message  string     `json:"message"`
	Data     *zhiboData `json:"data"`
}
type zhiboData struct {
	SortEnd       string      `json:"sortEnd"`
	Index         int         `json:"index"`
	Total         int         `json:"total"`
	Size          int         `json:"size"`
	FastNewsList []zhiboItem `json:"fastNewsList"`
}
type zhiboItem struct {
	Summary    *string  `json:"summary"`
	Code       string   `json:"code"`
	TitleColor int      `json:"titleColor"`
	RealSort   string   `json:"realSort"`
	ShowTime   string   `json:"showTime"`
	Title      string   `json:"title"`
	Share      int      `json:"share"`
	PinglunNum int      `json:"pinglun_Num"`
	StockList  []string `json:"stockList"`
	Image      []string `json:"image"`
}

func main() {
	// 手动扫 -probe 并把 -probe <值> 从 os.Args 中移除(各子命令注册自己 flag 后 Parse)。
	probeName := "dongcai"
	clean := []string{os.Args[0]}
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-probe" {
			if i+1 < len(os.Args) {
				probeName = os.Args[i+1]
				i++
			}
			continue
		}
		clean = append(clean, os.Args[i])
	}
	os.Args = clean
	switch probeName {
	case "dongcai":
		probeDongcai()
	case "quotemarket":
		probeQuotemarket()
	default:
		fmt.Fprintf(os.Stderr, "unknown probe: %s\n", probeName)
		os.Exit(2)
	}
}

func probeDongcai() {
	pages := flag.Int("pages", 2, "max pages to walk via sortEnd cursor")
	pageSize := flag.Int("pageSize", 5, "items per page")
	flag.Parse()

	ctx := context.Background()
	client := &http.Client{Timeout: 15 * time.Second}
	sortEnd := ""
	for p := 1; p <= *pages; p++ {
		url := fmt.Sprintf("%s?client=web&biz=web_724&sortEnd=%s&pageSize=%d&req_trace=probe", feedURL, sortEnd, *pageSize)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "page %d: fetch failed: %v\n", p, err)
			os.Exit(1)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "page %d: HTTP %d\n", p, resp.StatusCode)
			os.Exit(1)
		}
		var r zhiboResp
		if err := json.Unmarshal(body, &r); err != nil {
			fmt.Fprintf(os.Stderr, "page %d: bad json: %v\nraw: %s\n", p, err, body)
			os.Exit(1)
		}
		fmt.Printf("=== page %d (index=%d total=%d size=%d sortEnd=%s) ===\n", p, r.Data.Index, r.Data.Total, r.Data.Size, r.Data.SortEnd)
		if p == 1 {
			var raw any
			_ = json.Unmarshal(body, &raw)
			pretty, _ := json.MarshalIndent(raw, "", "  ")
			fmt.Printf("--- 首页原始响应 ---\n%s\n", pretty)
		}
		for _, it := range r.Data.FastNewsList {
			url := fmt.Sprintf("https://finance.eastmoney.com/a/%s.html", it.Code)
			fmt.Printf("- code=%s\n  title=%s\n  showTime=%s realSort=%s\n  summary=%v\n  stockList=%v image=%v\n  url=%s\n",
				it.Code, it.Title, it.ShowTime, it.RealSort, it.Summary, it.StockList, it.Image, url)
		}
		if r.Data == nil || r.Data.SortEnd == "" || len(r.Data.FastNewsList) == 0 {
			fmt.Println("no more pages")
			break
		}
		sortEnd = r.Data.SortEnd
		time.Sleep(200 * time.Millisecond)
	}
}
