// probe 同花顺自选协议探针(Phase A 硬门槛验证 + 日常体检)。
//
// 用法:
//
//	# ① 账密登录(真全自动路径;验证 verify2 四步是否仍可用)
//	go run ./cmd/probe -probe ths-selfstock -account <账号> -password <密码>
//
//	# ② 只验 cookie 注入(已实测可用;不碰账密)
//	go run ./cmd/probe -probe ths-selfstock -cookie 'userid=…; sess_tk=…; utk=…'
//
//	# ③ 只跑登录并打印拿到的 cookie(自助排障)
//	go run ./cmd/probe -probe ths-selfstock -account <账号> -password <密码> -print-cookie
//
// ⚠️ 本命令**只读**(绝不写自选),且**不进任何镜像**(cmd/probe 不在 Dockerfile 的
// tools COPY 清单里 —— 见 scripts/check-image-topology.sh)。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"piks/internal/ths"
)

func probeTHSSelfstock() {
	account := flag.String("account", "", "同花顺账号(与 -cookie 二选一)")
	password := flag.String("password", "", "同花顺密码(与 -cookie 二选一)")
	cookie := flag.String("cookie", "", "整串 cookie(优先于账密;已实测可用)")
	printCookie := flag.Bool("print-cookie", false, "登录后打印拿到的 cookie(排障用)")
	noNames := flag.Bool("no-names", false, "跳过 realhead 取名(省 ~40s 限频等待)")
	flag.Parse()

	creds := ths.Credentials{Cookie: *cookie, Account: *account, Password: *password}
	if creds.Cookie == "" && (creds.Account == "" || creds.Password == "") {
		fmt.Fprintln(os.Stderr, "需提供 -cookie 或 -account/-password")
		os.Exit(2)
	}

	ctx := context.Background()
	c := ths.New(creds)

	fmt.Println("======================================================================")
	fmt.Println("同花顺自选只读探针")
	fmt.Printf("凭据来源: %s\n", sourceLabel(creds))

	// ① 会话
	fmt.Print("\n[1] EnsureSession … ")
	if err := c.EnsureSession(ctx); err != nil {
		fmt.Printf("✗ %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ userid=%s\n", c.UserID())
	if exp := c.SessionExpiry(); !exp.IsZero() {
		fmt.Printf("    sess_tk 过期: %s(距今 %s)\n", exp.Format("2006-01-02 15:04"), time.Until(exp).Round(time.Hour))
	} else {
		fmt.Println("    sess_tk 过期: 未知(无 sess_tk 或非 JWT)")
	}
	if *printCookie {
		fmt.Println("    （登录所得 cookie 键见下）")
	}

	// ② 名单
	fmt.Print("\n[2] SelfStocks(我的自选名单)… ")
	items, err := c.SelfStocks(ctx)
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ %d 项\n", len(items))

	kept, dropped := 0, 0
	byMarket := map[string]int{}
	for _, it := range items {
		byMarket[it.MarketID]++
		if ths.IsAShare(it.MarketID) && len(it.Code) == 6 {
			kept++
		} else {
			dropped++
		}
	}
	fmt.Printf("    A 股(可入 PIKS): %d;非 A 股(将丢弃): %d\n", kept, dropped)
	printMarketDist(byMarket, items)

	// ③ 元数据(加入价/日)
	fmt.Print("\n[3] SelfStockDetails(加入价/加入日)… ")
	details, err := c.SelfStockDetails(ctx)
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		os.Exit(1)
	}
	withPrice, withDate := 0, 0
	for _, d := range details {
		if d.Price != nil {
			withPrice++
		}
		if d.AddedOn != nil {
			withDate++
		}
	}
	fmt.Printf("✓ %d 条;带价 %d;带日 %d\n", len(details), withPrice, withDate)
	for i, d := range details {
		if i >= 3 {
			break
		}
		fmt.Printf("    %s.%s 价=%s 日=%s\n", d.Code, ths.MarketAbbr(d.MarketID),
			fmtPtr(d.Price), fmtDate(d.AddedOn))
	}

	// ④ 取名(realhead)—— 验证 code→name 回退链
	if !*noNames {
		fmt.Print("\n[4] StockNames(realhead code→name,逐只限频 ~1s)… ")
		codes := make([]string, 0, len(items))
		for _, it := range items {
			if ths.IsAShare(it.MarketID) && len(it.Code) == 6 {
				codes = append(codes, it.Code)
			}
		}
		names, err := c.StockNames(ctx, codes)
		if err != nil {
			fmt.Printf("✗ %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ %d/%d 只取到名字\n", len(names), len(codes))
		miss := []string{}
		for _, code := range codes {
			if _, ok := names[code]; !ok {
				miss = append(miss, code)
			}
		}
		if len(miss) > 0 {
			fmt.Printf("    未取到名字(将 deferred 下轮重试): %v\n", miss)
		}
		sample := codes
		if len(sample) > 5 {
			sample = sample[:5]
		}
		for _, code := range sample {
			fmt.Printf("    %s → %s\n", code, names[code])
		}
	}

	fmt.Println("\n探针结束(未做任何写入)")
}

func sourceLabel(c ths.Credentials) string {
	if c.Cookie != "" {
		return "inject(cookie)"
	}
	return "login(账密)"
}

func printMarketDist(byMarket map[string]int, items []ths.SelfStock) {
	type kv struct {
		k string
		v int
	}
	rows := make([]kv, 0, len(byMarket))
	for k, v := range byMarket {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].v > rows[j].v })
	fmt.Print("    marketid 分布: ")
	for _, r := range rows {
		fmt.Printf("%s(%s)×%d ", r.k, ths.MarketAbbr(r.k), r.v)
	}
	fmt.Println()
	// 打印被丢弃的条目(如实不静默)
	var nonA []string
	for _, it := range items {
		if !ths.IsAShare(it.MarketID) || len(it.Code) != 6 {
			nonA = append(nonA, fmt.Sprintf("%s.%s", it.Code, ths.MarketAbbr(it.MarketID)))
		}
	}
	if len(nonA) > 0 {
		fmt.Printf("    丢弃条目: %v\n", nonA)
	}
}

func fmtPtr(p *float64) string {
	if p == nil {
		return "—"
	}
	return fmt.Sprintf("%g", *p)
}

func fmtDate(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.Format("2006-01-02")
}
