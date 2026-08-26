// reconcile 命令:raw↔event 数据对账(迭代 1,设计 §3.2)。
// 只读检查,报告写 task_runs.meta + 发布 00-System/recon-{date}.md(Generated 仓库)。数据诚实:异常如实列。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"piks/internal/config"
	"piks/internal/publish"
	"piks/internal/store"
)

func main() {
	vault := flag.String("vault", "", "override vault path (default from config)")
	flag.Parse()

	cfg := config.Load()
	if *vault != "" {
		cfg.VaultPath = *vault
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "reconcile")
	if err != nil {
		fatal("start task run:", err)
	}

	all := []store.ReconIssue{}
	for _, f := range []func(context.Context) ([]store.ReconIssue, error){
		s.ReconStaleRaw, s.ReconFailedRaw, s.ReconProcessedNoEvent,
		s.ReconOrphanEvent, s.ReconMissingEvidence, s.ReconSilentSources,
	} {
		items, err := f(ctx)
		if err != nil {
			finishFail(ctx, s, runID, err)
		}
		all = append(all, items...)
	}

	report := buildReport(all)
	path := filepath.Join(cfg.VaultPath, "00-System", "recon-"+time.Now().Format("2006-01-02")+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		finishFail(ctx, s, runID, err)
	}
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		finishFail(ctx, s, runID, err)
	}
	committed, err := publish.CommitVault(cfg.VaultPath)

	// 按类别计数
	counts := map[string]int{}
	for _, it := range all {
		counts[it.Category]++
	}
	ok := len(all) == 0
	meta := map[string]any{
		"total":      len(all),
		"by_category": counts,
		"conclusion": map[string]bool{"passed": ok},
		"report":     path,
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
	verdict := "通过"
	if !ok {
		verdict = "需关注"
	}
	fmt.Printf("reconcile: 异常=%d 结论=%s 报告=%s (git=%d)\n", len(all), verdict, path, committed)
	for _, it := range all {
		fmt.Printf("  - [%s] %s: %s\n", it.Category, it.EntityID, it.Detail)
	}
}

func buildReport(all []store.ReconIssue) string {
	counts := map[string]int{}
	for _, it := range all {
		counts[it.Category]++
	}
	categories := []string{"stale_raw", "failed_raw", "processed_no_event", "orphan_event", "missing_evidence", "silent_source"}
	labels := map[string]string{
		"stale_raw":         "孤儿 raw(滞留>7天)",
		"failed_raw":        "抽取失败",
		"processed_no_event": "已处理但无事件",
		"orphan_event":      "孤儿 event(无 raw)",
		"missing_evidence":  "缺证据事件",
		"silent_source":     "静默源(近24h无采集)",
	}
	var b strings.Builder
	b.WriteString("---\nid: recon\n")
	fmt.Fprintf(&b, "date: %s\ntype: recon\n---\n\n", time.Now().Format("2006-01-02"))
	b.WriteString("# 对账报告\n\n")
	b.WriteString("> 自动生成,如实反映,不掩盖异常。\n\n## 检查项\n\n")
	b.WriteString("| 检查项 | 异常数 |\n|---|---|\n")
	for _, c := range categories {
		fmt.Fprintf(&b, "| %s | %d |\n", labels[c], counts[c])
	}
	b.WriteString("\n## 异常清单\n")
	if len(all) == 0 {
		b.WriteString("\n_无_\n")
	} else {
		byCat := map[string][]store.ReconIssue{}
		for _, it := range all {
			byCat[it.Category] = append(byCat[it.Category], it)
		}
		for _, c := range categories {
			if len(byCat[c]) == 0 {
				continue
			}
			fmt.Fprintf(&b, "\n### %s (%d)\n\n", labels[c], len(byCat[c]))
			for _, it := range byCat[c] {
				fmt.Fprintf(&b, "- `%s`: %s\n", it.EntityID, it.Detail)
			}
		}
	}
	conclusion := "✅ 通过"
	if len(all) > 0 {
		conclusion = "⚠️ 需关注"
	}
	fmt.Fprintf(&b, "\n## 结论\n%s\n", conclusion)
	return b.String()
}

func finishFail(ctx context.Context, s *store.Store, runID int64, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{})
	fatal("reconcile:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
