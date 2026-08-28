// G8 语义检索探针(方案 B:LLM 同义扩展,2026-08-28)。
// 背景:Zen 无 embeddings 端点(实测 POST /embeddings → 404),故用 extract 档模型
// 把问题扩展为「原文+同义/近义改写」,并入 n-gram 检索(原文权重 1.0 / 扩展 0.5)。
// 目的:验收「降准」问法能召回「下调存款准备金率」类事件(纯关键词检索原本漏的)。
// 用法:go run ./cmd/probe -probe g8expand [-q "降准"]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"piks/internal/ai"
	"piks/internal/config"
	"piks/internal/store"
)

func probeG8Expand() {
	q := flag.String("q", "降准", "测试问题")
	flag.Parse()

	ctx := context.Background()
	cfg := config.Load()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer pool.Close()
	st := store.New(pool)

	m, err := st.ListAppConfig(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "list app_config:", err)
		os.Exit(1)
	}
	base, key, model := m["ai_service_base_url"], m["ai_api_key"], m["ai_model_extract"]
	if model == "" {
		model = "deepseek-chat"
	}
	if base == "" || key == "" {
		fmt.Fprintln(os.Stderr, "AI 未配置(dev app_config 需指向 Zen)")
		os.Exit(1)
	}
	c := ai.NewOpenAICompat(base, key, model)

	// 1) 同义扩展
	extra, xerr := c.ExpandQuery(ctx, *q)
	fmt.Printf("=== G8 同义扩展探针 问题=%q ===\n", *q)
	if xerr != nil {
		fmt.Println("扩展失败:", xerr, "(预期降级纯关键词)")
		extra = nil
	} else {
		fmt.Printf("扩展词(%d): %s\n", len(extra), strings.Join(extra, " | "))
	}

	// 2) 关键词基线 vs 扩展检索(对照召回差异)
	baseEvents, baseEnts, berr := st.SearchKnowledge(ctx, *q, 8, 8)
	expEvents, expEnts, eerr := st.SearchKnowledgeExpanded(ctx, *q, extra, 8, 8)
	if berr != nil || eerr != nil {
		fmt.Fprintln(os.Stderr, "search:", berr, eerr)
		os.Exit(1)
	}
	fmt.Printf("\n--- 关键词基线: events=%d entities=%d ---\n", len(baseEvents), len(baseEnts))
	for _, e := range baseEvents {
		fmt.Printf("  [E] %s\n", e.Title)
	}
	for _, en := range baseEnts {
		fmt.Printf("  [N] %s (%s)\n", en.Name, en.Type)
	}
	fmt.Printf("\n--- 扩展检索: events=%d entities=%d ---\n", len(expEvents), len(expEnts))
	for _, e := range expEvents {
		fmt.Printf("  [E] %s\n", e.Title)
	}
	for _, en := range expEnts {
		fmt.Printf("  [N] %s (%s)\n", en.Name, en.Type)
	}
}
