package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"piks/internal/model"
	"piks/internal/store"
)

// TestAccountSnapshotRoundTrip 账户快照存取(issue #19,迁移 0013):
// 关键语义 —— NULL 与 0 严格区分(截图没有 ≠ 确实为零);同日重传 upsert 覆盖。
func TestAccountSnapshotRoundTrip(t *testing.T) {
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set (integration off by default)")
	}
	dsn := os.Getenv("PIKS_DATABASE_URL")
	if dsn == "" {
		t.Skip("PIKS_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(pool)
	t.Cleanup(func() { pool.Close() })
	// 用一个不可能与真实数据相撞的远期日期,结束清理彻底。
	d := time.Date(2099, 1, 15, 0, 0, 0, 0, time.UTC)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM account_snapshots WHERE snapshot_date=$1`, d)
	})

	f := func(v float64) *float64 { return &v }

	// 1. 部分为空:total_asset / daily_pl 有值,其余 NULL
	if err := s.UpsertAccountSnapshot(ctx, model.AccountSnapshot{
		SnapshotDate: d, TotalAsset: f(520000.5), DailyPL: f(-800), Source: "screenshot",
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := s.LatestAccountSnapshot(ctx)
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if got == nil {
		t.Fatal("latest = nil, want snapshot")
	}
	if got.TotalAsset == nil || *got.TotalAsset != 520000.5 {
		t.Fatalf("total_asset = %v, want 520000.5", got.TotalAsset)
	}
	if got.DailyPL == nil || *got.DailyPL != -800 {
		t.Fatalf("daily_pl = %v, want -800", got.DailyPL)
	}
	// 未提供的两项必须仍是 NULL(不能变 0)
	if got.TotalMV != nil {
		t.Fatalf("total_mv 应为 NULL,得到 %v(0 与 NULL 混了)", *got.TotalMV)
	}
	if got.FloatPL != nil {
		t.Fatalf("float_pl 应为 NULL,得到 %v", *got.FloatPL)
	}

	// 2. 同日重传 → 覆盖(UNIQUE snapshot_date,不新增行)
	if err := s.UpsertAccountSnapshot(ctx, model.AccountSnapshot{
		SnapshotDate: d, TotalAsset: f(1), TotalMV: f(2), FloatPL: f(3), DailyPL: f(4), Source: "manual",
	}); err != nil {
		t.Fatalf("upsert overwrite: %v", err)
	}
	assertAccountRowCount(t, pool, d, 1)
	got, _ = s.LatestAccountSnapshot(ctx)
	if got.TotalMV == nil || *got.TotalMV != 2 || got.Source != "manual" {
		t.Fatalf("覆盖未生效: %+v", got)
	}

	// 3. 真 0 与 NULL 区分:显式写 0 必须读回 0(指针非 nil)
	if err := s.UpsertAccountSnapshot(ctx, model.AccountSnapshot{
		SnapshotDate: d, TotalAsset: f(0), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.LatestAccountSnapshot(ctx)
	if got.TotalAsset == nil || *got.TotalAsset != 0 {
		t.Fatalf("显式 0 读回应为 0(非 NULL): %v", got.TotalAsset)
	}
	if got.FloatPL != nil {
		t.Fatalf("本轮未提供的 float_pl 应为 NULL: %v", *got.FloatPL)
	}
}

func assertAccountRowCount(t *testing.T, pool *pgxpool.Pool, d time.Time, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM account_snapshots WHERE snapshot_date=$1`, d).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != want {
		t.Fatalf("account_snapshots 行数 = %d, want %d(upsert 未去重)", got, want)
	}
}
