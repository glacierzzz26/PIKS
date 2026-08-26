// collector 采集命令:源适配器 → 归一化 → content_hash 去重 → raw_documents。
// 源健康监控:单次运行 fetch 连续失败 ≥3 次 → 暂停该源(见设计 §4 源健康监控)。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"

	"piks/internal/collector"
	"piks/internal/config"
	"piks/internal/model"
	"piks/internal/store"
)

func main() {
	var (
		driverFlag = flag.String("driver", "file", "collector driver: file|dongcai")
		input      = flag.String("input", "", "file driver input path")
		sourceName = flag.String("source", "news-flash", "source name to attribute documents to")
	)
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "collector")
	if err != nil {
		fatal("start task run:", err)
	}

	drv, err := collector.NewDriver(*driverFlag, *input)
	if err != nil {
		finishFail(ctx, s, runID, *sourceName, *driverFlag, err)
	}

	// 确保数据源存在
	src, err := s.GetSourceByName(ctx, *sourceName)
	if errors.Is(err, pgx.ErrNoRows) {
		src = model.Source{Name: *sourceName, SourceType: "news"}
		if err := s.CreateSource(ctx, &src); err != nil {
			finishFail(ctx, s, runID, *sourceName, *driverFlag, err)
		}
	} else if err != nil {
		finishFail(ctx, s, runID, *sourceName, *driverFlag, err)
	}

	// fetch(最多 3 次,连续失败则暂停源)
	var news []collector.RawNews
	attempts := 0
	for {
		attempts++
		news, err = drv.Fetch(ctx)
		if err == nil {
			break
		}
		if attempts >= 3 {
			_ = s.PauseSource(ctx, src.ID)
			finishFail(ctx, s, runID, *sourceName, *driverFlag,
				fmt.Errorf("fetch failed after 3 attempts, source paused: %w", err))
		}
	}

	// 入库 + 去重
	newCount, dupCount, failCount := 0, 0, 0
	for _, n := range news {
		ok, err := s.InsertRawDocument(ctx, &model.RawDocument{
			SourceID:    src.ID,
			ExternalID:  collector.StrPtr(n.ExternalID),
			URL:         collector.StrPtr(n.URL),
			Title:       collector.StrPtr(n.Title),
			Content:     n.Content,
			ContentHash: collector.ContentHash(n.Content),
			PublishedAt: n.PublishedAt,
		})
		switch {
		case err != nil:
			failCount++
		case ok:
			newCount++
		default:
			dupCount++
		}
	}

	meta := map[string]any{
		"source": *sourceName,
		"driver": *driverFlag,
		"new":    newCount,
		"dup":    dupCount,
		"failed": failCount,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		fatal("finish task run:", err)
	}
	fmt.Printf("collector %s/%s: new=%d dup=%d failed=%d\n",
		*driverFlag, *sourceName, newCount, dupCount, failCount)
}

func finishFail(ctx context.Context, s *store.Store, runID int64, source, driver string, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(),
		map[string]any{"source": source, "driver": driver})
	fatal("collector:", err)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
