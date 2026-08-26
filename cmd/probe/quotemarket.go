// 东财行情接口探针(迭代2 G2,设计 §3.1)。
// 目的:逐字段对照真实响应 DTO + 暴露 WAF 限流缺口(数据诚实)。
// 用法:go run ./cmd/probe -probe quotemarket [-date 2026-08-26]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"piks/internal/collector"
)

func probeQuotemarket() {
	date := flag.String("date", "", "交易日 2006-01-02(默认今天,北京时区)")
	flag.Parse()

	d := *date
	if d == "" {
		d = time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	}

	raw, err := collector.NewMarketDriver().Fetch(context.Background(), d)
	if err != nil {
		fmt.Fprintf(os.Stderr, "quotemarket probe %s: fetch failed: %v\n", d, err)
		os.Exit(1)
	}
	if raw == nil {
		fmt.Printf("quotemarket probe %s: 非交易日(qdate != 请求日),无数据\n", d)
		return
	}

	fmt.Printf("=== quotemarket %s 探针(真实驱动) ===\n", d)
	fmt.Printf("涨停 %d / 跌停 %d / 炸板 %d;pending=%v\n", len(raw.LimitUp), len(raw.LimitDown), len(raw.Broken), raw.Pending)
	fmt.Printf("指数: ")
	for k, q := range raw.Indexes {
		fmt.Printf("%s=%.2f(%+.2f%%) ", k, q.Close, q.Pct)
	}
	fmt.Println()
	if raw.Breadth != nil {
		fmt.Printf("涨跌家数: %+v\n", *raw.Breadth)
	} else {
		fmt.Println("涨跌家数: 未获取(pending)")
	}
	if raw.TurnoverAmt != nil {
		fmt.Printf("成交额: %.1f 亿\n", *raw.TurnoverAmt)
	} else {
		fmt.Println("成交额: 未获取(pending)")
	}
	b, _ := json.MarshalIndent(raw.LimitUp, "", "  ")
	fmt.Printf("--- 涨停池前 5 条 ---\n%s\n", truncateStr(string(b), 600))
}

func truncateStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
