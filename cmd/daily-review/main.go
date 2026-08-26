// daily-review 每日复盘聚合页命令(迭代 2,设计 §3.3):读 market_snapshots → 02-Market/YYYY-MM-DD.md → git commit。
// 复用 publisher 的 md5 幂等(重跑零提交,§5.6);「我的判断」占位,个人内容走 09-Personal 分域(D17)。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"piks/internal/config"
	"piks/internal/publish"
	"piks/internal/store"
)

var cst = time.FixedZone("CST", 8*3600)

func main() {
	dateFlag := flag.String("date", "", "交易日(默认今天,北京时区;格式 2006-01-02)")
	flag.Parse()

	date := *dateFlag
	if date == "" {
		date = time.Now().In(cst).Format("2006-01-02")
	}
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		fatal("bad date:", err)
	}
	dayUTC := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)

	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "daily-review")
	if err != nil {
		fatal("start task run:", err)
	}

	snap, err := s.GetMarketSnapshotByDate(ctx, dayUTC)
	if err != nil {
		finishFail(ctx, s, runID, date, err)
	}
	if snap == nil {
		fmt.Printf("daily-review %s: 无快照,跳过(先跑 quote-collector + market-state)\n", date)
		_ = s.FinishTaskRun(ctx, runID, "skipped", "", map[string]any{"date": date, "note": "no snapshot"})
		return
	}

	pipeline := "market-state@" + gitShort()
	content := publish.RenderMarket(snap, pipeline)
	path := publish.MarketPath(cfg.VaultPath, dayUTC)
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		finishFail(ctx, s, runID, date, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		finishFail(ctx, s, runID, date, err)
	}

	committed, err := publish.CommitVaultWithMsg(cfg.VaultPath, fmt.Sprintf("publish: 每日复盘 %s", date))
	if err != nil {
		// git 失败不阻断:文件已落盘,记录即可(与事件发布一致)
		fmt.Printf("daily-review %s: 文件已写入 %s,但 git 提交失败: %v\n", date, path, err)
	}

	status := "unchanged"
	if committed > 0 {
		status = "published"
	}
	_ = s.FinishTaskRun(ctx, runID, "success", "", map[string]any{"date": date, "published": status})
	fmt.Printf("daily-review %s: %s (%s)\n", date, status, path)
}

// gitShort 代码仓库当前短哈希(最佳努力;失败返回空)。
func gitShort() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func dirOf(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "."
	}
	return p[:i]
}

func finishFail(ctx context.Context, s *store.Store, runID int64, date string, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"date": date})
	fatal(err)
}

func fatal(v ...any) {
	_, _ = fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}
