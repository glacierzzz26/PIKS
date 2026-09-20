// collector 采集命令:源适配器 → 归一化 → content_hash 去重 → raw_documents。
// 源健康监控:单次运行 fetch 连续失败 ≥3 次 → 暂停该源(见设计 §4 源健康监控)。
//
// issue #43 T1:事件类多源采集。`-driver all` 依次跑全部独立机构源,每源落各自的
// sources 行(**机构名**,不再是任务名 `news-flash`),使前端可区分来源。
// 单源失败**不阻断**其余源(免费源无 SLA);该源自身按既有纪律连续失败 3 次暂停。
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

// sourceSpec 一个机构源的采集规格:驱动名 + sources.name(机构名)+ source_type。
// 机构名是**独立机构的标识**(非任务名),前端「快讯」tab 的来源筛选以此为准。
var sourceSpecs = []struct {
	Driver     string
	Name       string // sources.name —— 机构名
	SourceType string
}{
	{"dongcai", "东方财富", "news"},
	{"jin10", "金十数据", "news"},
	{"cls", "财联社", "news"},
	{"sina", "新浪财经", "news"},
	{"ths", "同花顺", "news"},
	{"futu", "富途资讯", "news"},
}

func main() {
	var (
		driverFlag = flag.String("driver", "file", "collector driver: file|all|dongcai|jin10|cls|sina|ths|futu")
		input      = flag.String("input", "", "file driver input path")
		sourceName = flag.String("source", "", "source name (机构名) override;默认按 driver 映射")
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

	specs, err := resolveSpecs(*driverFlag, *sourceName, *input)
	if err != nil {
		fatal("collector:", err)
	}

	// -driver all:单源失败不阻断其余源(免费源无 SLA,#43)。逐源记账。
	failed := 0
	for _, sp := range specs {
		if err := runOne(ctx, s, sp, *input); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "collector %s/%s FAILED: %v\n", sp.Driver, sp.Name, err)
		}
	}
	if failed == len(specs) {
		fatal("collector: all sources failed")
	}
}

// spec 一个待采集源的解析结果。
type spec struct {
	Driver     string
	Name       string
	SourceType string
}

// resolveSpecs 把 -driver/-source 解析为待采集源清单。
// 显式 -source 覆盖机构名(单一 driver 时才有意义;file 驱动靠它归属)。
func resolveSpecs(driver, source, input string) ([]spec, error) {
	if driver == "all" {
		out := make([]spec, 0, len(sourceSpecs))
		for _, s := range sourceSpecs {
			out = append(out, spec{Driver: s.Driver, Name: s.Name, SourceType: s.SourceType})
		}
		return out, nil
	}
	if driver == "file" {
		name := source
		if name == "" {
			name = "news-flash" // 迭代 0 保底驱动的既有归属,保持向后兼容
		}
		return []spec{{Driver: "file", Name: name, SourceType: "news"}}, nil
	}
	for _, s := range sourceSpecs {
		if s.Driver == driver {
			name := s.Name
			if source != "" {
				name = source
			}
			return []spec{{Driver: s.Driver, Name: name, SourceType: s.SourceType}}, nil
		}
	}
	return nil, fmt.Errorf("unknown driver: %s", driver)
}

// runOne 采集单个源并入库。任一步失败即返回错误(由调用方决定是否阻断)。
func runOne(ctx context.Context, s *store.Store, sp spec, input string) error {
	runID, err := s.StartTaskRun(ctx, "collector")
	if err != nil {
		return fmt.Errorf("start task run: %w", err)
	}

	drv, err := collector.NewDriver(sp.Driver, input)
	if err != nil {
		return finishFail(ctx, s, runID, sp, err)
	}

	src, err := ensureSource(ctx, s, sp)
	if err != nil {
		return finishFail(ctx, s, runID, sp, err)
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
			return finishFail(ctx, s, runID, sp,
				fmt.Errorf("fetch failed after 3 attempts, source paused: %w", err))
		}
	}

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
			Extra:       n.Extra,
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
		"source": sp.Name,
		"driver": sp.Driver,
		"new":    newCount,
		"dup":    dupCount,
		"failed": failCount,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		return fmt.Errorf("finish task run: %w", err)
	}
	fmt.Printf("collector %s/%s: new=%d dup=%d failed=%d\n",
		sp.Driver, sp.Name, newCount, dupCount, failCount)
	return nil
}

// ensureSource 取或建该机构源。源被暂停时**不**自动复活(须人工确认,防抖动源空转)。
func ensureSource(ctx context.Context, s *store.Store, sp spec) (model.Source, error) {
	src, err := s.GetSourceByName(ctx, sp.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		src = model.Source{Name: sp.Name, SourceType: sp.SourceType}
		if err := s.CreateSource(ctx, &src); err != nil {
			return model.Source{}, err
		}
		return src, nil
	}
	return src, err
}

// finishFail 记账失败并返回错误(不 os.Exit —— 多源模式下单源失败须继续跑其余源)。
func finishFail(ctx context.Context, s *store.Store, runID int64, sp spec, err error) error {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(),
		map[string]any{"source": sp.Name, "driver": sp.Driver})
	return err
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
