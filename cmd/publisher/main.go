// publisher 发布命令:events(Fact 层)→ Markdown 事件卡片 → Obsidian vault(独立 git 仓库)。
// 幂等:已发布事件跳过;文件已存在则视为已发布。推送可选(PIKS_VAULT_REMOTE)。
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"piks/internal/config"
	"piks/internal/publish"
	"piks/internal/store"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "publisher")
	if err != nil {
		fatal("start task run:", err)
	}

	items, err := s.ListEventsForPublishWithSource(ctx)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}

	vault := cfg.VaultPath
	published, skipped := 0, 0
	for _, it := range items {
		path := publish.EventPath(vault, it)
		if _, err := os.Stat(path); err == nil {
			// 文件已存在(上次发布过但 DB 状态未置)则视为已发布,避免重复写+git 噪音
			skipped++
			_ = s.MarkEventPublished(ctx, it.ID)
			continue
		}
		ev, _ := s.GetEvidenceByEventID(ctx, it.ID)
		content := publish.RenderEvent(it, &ev)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			finishFail(ctx, s, runID, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			finishFail(ctx, s, runID, err)
		}
		if err := s.MarkEventPublished(ctx, it.ID); err != nil {
			finishFail(ctx, s, runID, err)
		}
		published++
	}

	committed, err := publish.CommitVault(vault)
	meta := map[string]any{
		"published":  published,
		"skipped":    skipped,
		"vault":      vault,
		"git_commit": committed,
	}
	status, errMsg := "success", ""
	if err != nil {
		status, errMsg = "failed", err.Error()
		meta["git_error"] = err.Error()
	}
	if err := s.FinishTaskRun(ctx, runID, status, errMsg, meta); err != nil {
		fatal("finish task run:", err)
	}
	fmt.Printf("publisher: published=%d skipped=%d vault=%s (git commits=%d)\n", published, skipped, vault, committed)
}

func finishFail(ctx context.Context, s *store.Store, runID int64, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
	fatal("publisher:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
