// quote-collector 行情采集命令(迭代2 G2):东财涨停/炸板/跌停池 + 指数 → observations。
// 设计 §3.1/§2.3。非交易日跳过;受限端点失败如实记 pending,不造假。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"piks/internal/collector"
	"piks/internal/config"
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

	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	runID, err := s.StartTaskRun(ctx, "quote-collector")
	if err != nil {
		fatal("start task run:", err)
	}

	drv := collector.NewMarketDriver()
	raw, err := drv.Fetch(ctx, date)
	if err != nil {
		finishFail(ctx, s, runID, date, err)
	}
	if raw == nil {
		fmt.Printf("quote-collector %s: 非交易日,跳过\n", date)
		_ = s.FinishTaskRun(ctx, runID, "skipped", "", map[string]any{"date": date, "note": "non-trading day"})
		return
	}

	n, err := persistObservations(ctx, s, raw)
	if err != nil {
		finishFail(ctx, s, runID, date, err)
	}

	_ = s.FinishTaskRun(ctx, runID, "success", "", map[string]any{
		"date":         date,
		"zt_count":     len(raw.LimitUp),
		"zd_count":     len(raw.LimitDown),
		"broken_count": len(raw.Broken),
		"indexes":      len(raw.Indexes),
		"pending":      raw.Pending,
		"observations": n,
	})
	fmt.Printf("quote-collector %s: zt=%d zd=%d broken=%d indexes=%d obs=%d pending=%v\n",
		date, len(raw.LimitUp), len(raw.LimitDown), len(raw.Broken), len(raw.Indexes), n, raw.Pending)
}

// persistObservations 归一化快照 → observations 行(设计 §2.3 指标字典)。
func persistObservations(ctx context.Context, s *store.Store, raw *collector.MarketSnapshotRaw) (int, error) {
	obsAt := time.Date(raw.TradeDate.Year(), raw.TradeDate.Month(), raw.TradeDate.Day(), 15, 0, 0, 0, cst)
	var rows []model.Observation
	src := "eastmoney"

	rows = append(rows,
		obs(obsAt, "all", "limit_up_count", itoa(len(raw.LimitUp)), src),
		obs(obsAt, "all", "limit_down_count", itoa(len(raw.LimitDown)), src),
		obs(obsAt, "all", "broken_limit_count", itoa(len(raw.Broken)), src),
		obs(obsAt, "all", "max_board", maxBoard(raw.LimitUp), src),
		obs(obsAt, "all", "zt_pool", mustJSON(raw.LimitUp), src),
		obs(obsAt, "all", "industry_dist", mustJSON(industryDist(raw.LimitUp)), src),
	)
	if raw.TurnoverAmt != nil {
		rows = append(rows, obs(obsAt, "all", "market_turnover", fmt.Sprintf("%.1f", *raw.TurnoverAmt), src))
	}
	if raw.Breadth != nil {
		rows = append(rows,
			obs(obsAt, "all", "breadth_advance", itoa(raw.Breadth.Advance), src),
			obs(obsAt, "all", "breadth_decline", itoa(raw.Breadth.Decline), src),
			obs(obsAt, "all", "breadth_flat", itoa(raw.Breadth.Flat), src),
		)
	}
	for k, q := range raw.Indexes {
		rows = append(rows,
			obs(obsAt, k, "index_close", fmt.Sprintf("%.2f", q.Close), src),
			obs(obsAt, k, "index_pct", fmt.Sprintf("%.2f", q.Pct), src),
		)
	}

	n := 0
	for i := range rows {
		rows[i].ObservedAt = obsAt
		changed, err := s.UpsertObservation(ctx, rows[i])
		if err != nil {
			return n, fmt.Errorf("upsert observation %s/%s: %w", rows[i].Market, rows[i].Indicator, err)
		}
		if changed {
			n++
		}
	}
	return n, nil
}

func obs(at time.Time, market, indicator, value, src string) model.Observation {
	return model.Observation{Market: market, Indicator: indicator, Value: value, ObservedAt: at, Source: &src}
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func maxBoard(items []collector.ZTItem) string {
	m := 0
	for _, it := range items {
		if it.Lbc > m {
			m = it.Lbc
		}
	}
	return itoa(m)
}

// industryDist 涨停行业分布 {"家居用品":5,...},按 count 降序。
func industryDist(items []collector.ZTItem) map[string]int {
	m := make(map[string]int)
	for _, it := range items {
		if it.Hybk != "" {
			m[it.Hybk]++
		}
	}
	return m
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func finishFail(ctx context.Context, s *store.Store, runID int64, date string, err error) {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), map[string]any{"date": date})
	fatal(err)
}

func fatal(v ...any) {
	_, _ = fmt.Fprintln(os.Stderr, v...)
	os.Exit(1)
}
