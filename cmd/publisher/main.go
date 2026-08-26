// publisher 发布命令:events(Fact 层)→ Markdown 事件卡片 → Obsidian vault(独立 git 仓库)。
// 增量发布(迭代 1,设计 §3.4):
//   候选 = 未发布事件 ∪ 已发布但被更新(updated_at>published_at)的事件;
//   渲染内容与磁盘 md5 相同 → 跳过写(git 零提交);
//   不同 → 重写卡片(增量); 新事件 → 新建卡片;
//   已发布但被并入簇(merged)的事件 → 删除其旧卡片。
// 推送可选(PIKS_VAULT_REMOTE)。
package main

import (
	"context"
	"crypto/md5"
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

	vault := cfg.VaultPath
	published, updated, unchanged, deleted := 0, 0, 0, 0

	items, err := s.ListEventsForPublishWithSource(ctx)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}
	for _, it := range items {
		path := publish.EventPath(vault, it)
		evs, _ := s.ListEvidenceByEventID(ctx, it.ID)
		content := publish.RenderEvent(it, evs)

		if existing, err := os.ReadFile(path); err == nil {
			if hash(content) == hash(string(existing)) {
				// 内容未变(如仅 updated_at 被触碰):不写盘,仍标记发布态
				unchanged++
				_ = s.MarkEventPublished(ctx, it.ID)
				continue
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				finishFail(ctx, s, runID, err)
			}
			updated++ // 增量重写
		} else {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				finishFail(ctx, s, runID, err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				finishFail(ctx, s, runID, err)
			}
			published++ // 首次发布
		}
		if err := s.MarkEventPublished(ctx, it.ID); err != nil {
			finishFail(ctx, s, runID, err)
		}
	}

	// 删除被并入簇的已发布旧卡片(git 记录删除)
	merged, err := s.ListMergedPublished(ctx)
	if err != nil {
		finishFail(ctx, s, runID, err)
	}
	for _, m := range merged {
		mp := publish.EventPath(vault, store.EventForPublish{ID: m.ID, EventType: m.EventType})
		if err := os.Remove(mp); err == nil {
			deleted++
		}
	}

	committed, err := publish.CommitVault(vault)
	meta := map[string]any{
		"published":  published,
		"updated":    updated,
		"unchanged":  unchanged,
		"deleted":    deleted,
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
	fmt.Printf("publisher: published=%d updated=%d unchanged=%d deleted=%d vault=%s (git commits=%d)\n",
		published, updated, unchanged, deleted, vault, committed)
}

// hash 返回内容 MD5 摘要,用于"写盘是否必要"判断。
func hash(s string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(s)))
}

func finishFail(ctx context.Context, s *store.Store, runID int64, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
	fatal("publisher:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
