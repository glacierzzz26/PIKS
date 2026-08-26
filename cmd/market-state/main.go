// market-state 快照聚合命令(迭代 2,设计 §3.2):读当日 observations + 昨日快照 → market_snapshots。
// 纯规则,零 AI。情绪组件逐项打分入 emotion_detail;昨日强势股尽力而为,失败标 missing(§5.8 宁缺毋假)。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"piks/internal/collector"
	"piks/internal/config"
	"piks/internal/marketstate"
	"piks/internal/model"
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

	runID, err := s.StartTaskRun(ctx, "market-state")
	if err != nil {
		fatal("start task run:", err)
	}

	obs, err := s.ListObservationsByDate(ctx, dayUTC)
	if err != nil {
		finishFail(ctx, s, runID, date, err)
	}
	if len(obs) == 0 {
		fmt.Printf("market-state %s: 无观测,跳过(非交易日或 quote-collector 未运行)\n", date)
		_ = s.FinishTaskRun(ctx, runID, "skipped", "", map[string]any{"date": date, "note": "no observations"})
		return
	}

	// 昨日快照(最近一个 < 今日的快照)
	prev := prevSnapshot(ctx, s, dayUTC)

	// 昨日强势股:昨日涨停池代码 → 今日涨跌幅(尽力而为,失败标 missing)
	var prevReturns map[string]float64
	if prev != nil && len(prev.ZTPool) > 0 {
		var codes []string
		var pool []collector.ZTItem
		if err := json.Unmarshal(prev.ZTPool, &pool); err == nil {
			for _, it := range pool {
				codes = append(codes, it.Code)
			}
		}
		if len(codes) > 0 {
			drv := collector.NewMarketDriver()
			if returns, err := drv.FetchDailyReturns(ctx, codes); err == nil && len(returns) > 0 {
				prevReturns = returns
			}
		}
	}

	snap, pending := marketstate.ComputeSnapshot(day, obs, prev, prevReturns)

	// 11 重要事件 + 8 热点(events + 涨停行业派生,§2.1/§边界)
	events, err := s.ListEventsByDate(ctx, dayUTC)
	if err != nil {
		finishFail(ctx, s, runID, date, err)
	}
	topEvents := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		topEvents = append(topEvents, map[string]any{"id": ev.ID, "title": ev.Title})
	}
	if len(topEvents) > 0 {
		snap.TopEvents = mustJSON(topEvents)
	}
	snap.HotTopics = mustJSON(buildHotTopics(snap.IndustryDist, events))

	// 血缘:本快照派生自 observations(quote-collector 写入)
	snap.Evidence = mustJSON(map[string]any{
		"source":   "observations (quote-collector)",
		"fetched":  time.Now().UTC(),
		"pending":  pending,
		"prev_day": prev != nil,
	})

	if err := s.UpsertMarketSnapshot(ctx, snap); err != nil {
		finishFail(ctx, s, runID, date, err)
	}

	meta := map[string]any{"date": date}
	if snap.EmotionState != nil {
		meta["emotion_state"] = *snap.EmotionState
	}
	if snap.EmotionScore != nil {
		meta["emotion_score"] = *snap.EmotionScore
	}
	_ = s.FinishTaskRun(ctx, runID, "success", "", meta)
	fmt.Printf("market-state %s: emotion=%s score=%.1f events=%d pending=%v\n",
		date, strPtr(snap.EmotionState), scoreOrZero(snap.EmotionScore), len(events), pending)
}

// prevSnapshot 最近一个早于 day 的快照。
func prevSnapshot(ctx context.Context, s *store.Store, day time.Time) *model.MarketSnapshot {
	list, err := s.ListMarketSnapshots(ctx, 3)
	if err != nil {
		return nil
	}
	for i := range list {
		if list[i].TradeDate.Before(day) {
			return &list[i]
		}
	}
	return nil
}

// buildHotTopics 热点 = 涨停行业(top5)+ 当日事件(top3),设计 JSON 形 [{name,event_ids}]。
func buildHotTopics(industryRaw []byte, events []model.Event) []map[string]any {
	out := make([]map[string]any, 0, 8)
	var dist map[string]int
	if json.Unmarshal(industryRaw, &dist) == nil {
		type kv struct {
			name  string
			count int
		}
		var ks []kv
		for n, c := range dist {
			ks = append(ks, kv{n, c})
		}
		sort.Slice(ks, func(i, j int) bool { return ks[i].count > ks[j].count })
		for i, k := range ks {
			if i >= 5 {
				break
			}
			out = append(out, map[string]any{"name": k.name, "count": k.count})
		}
	}
	for i, ev := range events {
		if i >= 3 {
			break
		}
		out = append(out, map[string]any{"name": ev.Title, "event_ids": []string{ev.ID}})
	}
	return out
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func strPtr(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func scoreOrZero(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func finishFail(ctx context.Context, s *store.Store, runID int64, date string, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"date": date})
	fatal(err)
}

func fatal(v ...any) {
	_, _ = fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}
