// cluster-raw-link 命令:raw 层转载组回填(issue #83 分期 P-4 / 原 P-8)。
//
// 干什么:把「同一篇稿被多家机构各自落了一行」的 raw 文档聚成组,选一个代表行,把同组其余行的
// `raw_documents.canonical_id` 指向代表。读路径据此回答「这条消息**哪些渠道报了 + 各自链接**」——
// 取 raw 层**全集**(某家报了但没被抽出事件时,事件层看不到它,raw 层看得到)。
//
// 判据唯一:`internal/cluster/reprint_raw.go` 的 `RawGroups`(**直接复用** P-1 `GroupReprints`
// 的正文指纹 @0.85,不另立第二把尺子)。本命令只负责 IO。
//
// 🔴 代表**冻结**:只写仍为 NULL 的行(`SetRawCanonicalIDs` 内置 `AND canonical_id IS NULL`);
// 已冻结的行永不重选 —— 否则 url 归属会漂移(迁移 0022 注释)。故本命令**幂等**:重跑零行变更。
//
// 顺序:`scripts/pipeline.sh` 里排在 `worker` **之前**、`cluster` 附近 —— 它是展示层来源的
// 取数前置,与事件聚类(事件层)解耦。零 LLM、零外部调用。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"piks/internal/cluster"
	"piks/internal/config"
	"piks/internal/store"
)

func main() {
	// 收窗天数:<=0 = 不限(全量)。默认 30 天 —— raw 层转载组服务于展示层近期来源列表,
	// 存量老稿无展示需求;收窗也把单轮成本钉成常数(照 cluster 的 window-days 精神)。
	windowDays := flag.Int("window-days", 30, "只对 retrieved_at 在近 N 天内的 raw 文档分组(<=0 = 不限)")
	dryRun := flag.Bool("dry-run", false, "只打印分组结果,不写库")
	flag.Parse()

	ctx := context.Background()
	cfg := config.Load()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "cluster-raw-link")
	if err != nil {
		fatal("start task run:", err)
	}

	var since time.Time
	if *windowDays > 0 {
		since = time.Now().Add(-time.Duration(*windowDays) * 24 * time.Hour)
	}
	docs, err := s.ListRawDocumentsForGrouping(ctx, since)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}

	// 投影成纯函数入参(store → cluster,不互相依赖)。
	entries := make([]cluster.RawDocEntry, len(docs))
	for i, d := range docs {
		rep := ""
		if d.CanonicalID != nil {
			rep = *d.CanonicalID
		}
		entries[i] = cluster.RawDocEntry{
			ID: d.ID, SourceID: d.SourceID, Content: d.Content,
			RetrievedAt: d.RetrievedAt.Format(time.RFC3339Nano), CanonicalID: rep,
		}
	}
	groups := cluster.RawGroups(entries)

	// 待写回 = 各组里仍为 NULL 的成员 → [memberID, repID]。
	pairs := make([][2]string, 0)
	for _, g := range groups {
		for _, m := range g.Members {
			pairs = append(pairs, [2]string{m, g.RepID})
		}
	}

	var changed int64
	if *dryRun {
		for _, g := range groups {
			fmt.Printf("组: rep=%s members=%v\n", g.RepID, g.Members)
		}
	} else {
		changed, err = s.SetRawCanonicalIDs(ctx, pairs)
		if err != nil {
			finishFail(ctx, s, runID, err)
		}
	}

	meta := map[string]any{
		"docs":         len(docs),
		"groups":       len(groups),
		"pairs":        len(pairs), // 待写回(未冻结)行数
		"rows_changed": changed,    // 实际改动行数(重跑 → 0,幂等)
		"window_days":  *windowDays,
		"dry_run":      *dryRun,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fatal("finish task run:", err)
	}
	fmt.Printf("cluster-raw-link: docs=%d groups=%d pairs=%d changed=%d (window=%dd)\n",
		len(docs), len(groups), len(pairs), changed, *windowDays)
}

func finishFail(ctx context.Context, s *store.Store, runID int64, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
	fatal("cluster-raw-link:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
