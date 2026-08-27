// web 命令:迭代 5 Web 平台(替换 Obsidian 界面层,设计 web-app.md)。
// PostgreSQL 直接渲染 HTML;图谱/点选面板走 /api JSON。读只读(5-1)+ 配置编辑(5-2 前置),AI 对话在 5-3。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"piks/internal/config"
	"piks/internal/store"
	"piks/internal/web"
)

func main() {
	listenFlag := flag.String("listen", "", "listen address (default: PIKS_LISTEN_ADDR 或 :8090)")
	flag.Parse()

	cfg := config.Load()
	listen := os.Getenv("PIKS_LISTEN_ADDR")
	if *listenFlag != "" {
		listen = *listenFlag
	}
	if listen == "" {
		listen = ":8090"
	}

	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()

	s := store.New(pool)
	// AI 配置权威源 = 数据库 app_config(不再读 PIKS_AI_* 环境变量)。
	if err := s.ApplyAppConfig(ctx, &cfg); err != nil {
		fatal("apply app config:", err)
	}
	sv, err := web.NewServer(s, cfg)
	if err != nil {
		fatal("new server:", err)
	}

	log.Printf("piks-web listening on %s (db ok)", listen)
	srv := &http.Server{
		Addr:              listen,
		Handler:           sv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		fatal(err)
	}
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
