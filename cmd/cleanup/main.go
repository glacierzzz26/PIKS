// cleanup 命令:raw_documents 保留期清理(issue #83 分期 P-5)。
//
// 干什么:删除**到期**且**从未被抽取成事件**的 raw 行,给无限增长的采集层划一道保留期
// (issue P3 定「28 天」)。此前 `raw_documents` 只增不删,常驻采集(issue #68 C 层盘中
// 每 3 分钟)会让它无界增长。
//
// 🔴 **只清「无事件引用」的行**(判据见 `store.cleanupWhere`):已抽取的行是**事件溯源**
// (source_url / 详情抽屉来源 / cluster_sources 都消费它),删了会破坏这些消费方,且被
// `events.raw_document_id` 外键挡下。故保留期**不是**「全体 raw 的 28 天」,而是
// 「未被抽取的 raw 行保留 28 天」—— 文档与 UI 文案须逐字写清。
//
// 另外两条安全条件(防悬空):不删**转载组代表**(`raw_documents.canonical_id` 指向的行),
// 否则组内成员 join 不到代表。
//
// 调用时机由 `scripts/cleanup.sh` 把关(周日 02:00 且距上次成功清理 ≥28 天,规避
// 23:59 的 backup.sh)。本命令只管「跑一轮」。`-dry-run` 预览候选(与实删**共用判据**,
// 预览数 == 实删数)。零 LLM、零外部调用。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"piks/internal/config"
	"piks/internal/store"
)

func main() {
	days := flag.Int("days", 28, "保留期天数:删除 retrieved_at 早于 now()-N 天且无事件引用的 raw 行")
	dryRun := flag.Bool("dry-run", false, "只预览候选行数(与实删同判据),不写库")
	flag.Parse()

	if *days <= 0 {
		fatal("cleanup: -days 必须为正数(0/负 会让判据变成「全删」,拒绝执行)")
	}
	olderThan := time.Duration(*days) * 24 * time.Hour

	ctx := context.Background()
	cfg := config.Load()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "cleanup")
	if err != nil {
		fatal("start task run:", err)
	}

	var deleted int64
	if *dryRun {
		rows, err := s.ListRawCleanupCandidates(ctx, olderThan)
		if err != nil {
			finishFail(ctx, s, runID, err)
		}
		for _, r := range rows {
			fmt.Printf("候选: %s  %s  %s  %s\n", r.ID, r.OccurAt.Format(time.RFC3339), r.Source, r.Title)
		}
		deleted = int64(len(rows))
	} else {
		deleted, err = s.PurgeRawDocuments(ctx, olderThan)
		if err != nil {
			finishFail(ctx, s, runID, err)
		}
	}

	meta := map[string]any{
		"days":    *days,
		"deleted": deleted,
		"dry_run": *dryRun,
		"basis":   "raw_documents 到期且无事件引用且非转载组代表",
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fatal("finish task run:", err)
	}
	if *dryRun {
		fmt.Printf("cleanup(dry-run): 候选 %d 行(保留 %d 天)\n", deleted, *days)
	} else {
		fmt.Printf("cleanup: 删除 %d 行(保留 %d 天)\n", deleted, *days)
	}
}

func finishFail(ctx context.Context, s *store.Store, runID int64, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
	fatal("cleanup:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
