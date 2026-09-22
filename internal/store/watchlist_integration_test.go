package store_test

// watchlist_entries 真库集成测(迁移 0020)。
// 双开关:PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL。用自建隔离数据(6 位码 + t.Cleanup),不污染真实自选。
//
// 🔴 最关键的一条:加入价/日**不被 entity-build 覆盖**(这是本表存在的唯一理由)。

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

func openWatchTestStore(t *testing.T) (context.Context, *store.Store, func(string) string) {
	t.Helper()
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
	t.Cleanup(func() { pool.Close() })
	s := store.New(pool)
	// 返回「登记清理」闭包:测试里每建一个 code 就登记一次,test 结束删掉本行。
	clean := func(code string) string {
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM watchlist_entries WHERE code=$1`, code)
		})
		return code
	}
	return ctx, s, clean
}

// uniqueCode 生成一个 6 位数字码(9 开头:B 股段,自选里几乎不会出现 → 隔离)。
func uniqueCode(t *testing.T) string {
	t.Helper()
	return "9" + padFive(time.Now().UnixNano()%100000)
}

func padFive(n int64) string {
	b := make([]byte, 5)
	for i := 4; i >= 0; i-- {
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b)
}

func f64(v float64) *float64 { return &v }

// rowExists 该 code 的行是否仍在表里(不论 removed_at —— 用来证明「移出不删行」)。
func rowExists(ctx context.Context, t *testing.T, s *store.Store, code string) bool {
	t.Helper()
	var n int
	if err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM watchlist_entries WHERE code=$1`, code).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// ① 往返 + NULL 语义:价/日为 nil → 读回仍为 nil(绝不被写成 0)。
func TestWatchlistEntryNullRoundTrip(t *testing.T) {
	ctx, s, clean := openWatchTestStore(t)
	code := clean(uniqueCode(t))

	if err := s.UpsertWatchlistEntry(ctx, code, nil, "SH", "17", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListWatchlistEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range got {
		if e.Code != code {
			continue
		}
		found = true
		if e.AddedPrice != nil {
			t.Fatalf("added_price 应为 NULL, got %v", *e.AddedPrice)
		}
		if e.AddedOn != nil {
			t.Fatalf("added_on 应为 NULL, got %v", *e.AddedOn)
		}
	}
	if !found {
		t.Fatal("未找到刚写入的行")
	}
}

// ② 幂等:同一 UPSERT 连跑两次 → 仍是一行,removed_at 保持 NULL。
func TestWatchlistEntryIdempotent(t *testing.T) {
	ctx, s, clean := openWatchTestStore(t)
	code := clean(uniqueCode(t))

	for i := 0; i < 2; i++ {
		if err := s.UpsertWatchlistEntry(ctx, code, nil, "SZ", "33", f64(12.34), nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListWatchlistEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range got {
		if e.Code != code {
			continue
		}
		n++
		if e.RemovedAt != nil {
			t.Fatal("幂等 keep 后 removed_at 应为 NULL")
		}
		if e.AddedPrice == nil || *e.AddedPrice != 12.34 {
			t.Fatalf("added_price=%v", e.AddedPrice)
		}
	}
	if n != 1 {
		t.Fatalf("应恰有一行, got %d", n)
	}
}

// ③ remove → 不删行(removed_at 非空、不在 live 查询里);re-add → 回 NULL 且价/日取新值。
func TestWatchlistEntryRemoveKeepsRowReAddRefreshes(t *testing.T) {
	ctx, s, clean := openWatchTestStore(t)
	code := clean(uniqueCode(t))

	day1 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := s.UpsertWatchlistEntry(ctx, code, nil, "SH", "17", f64(10), &day1, nil); err != nil {
		t.Fatal(err)
	}
	changed, err := s.MarkWatchlistRemoved(ctx, code)
	if err != nil || !changed {
		t.Fatalf("首次移出应发生变更, changed=%v err=%v", changed, err)
	}
	// 不在 live 列表
	live, err := s.ListWatchlistEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range live {
		if e.Code == code {
			t.Fatal("移出后不应出现在 ListWatchlistEntries(removed_at IS NULL)")
		}
	}
	// 行仍在(不删行)
	if !rowExists(ctx, t, s, code) {
		t.Fatal("移出后行不应被删除")
	}
	// 再次移出 → 幂等,不重复变更
	if again, _ := s.MarkWatchlistRemoved(ctx, code); again {
		t.Fatal("重复移出不应再报告变更")
	}

	// re-add:上游给新价/日 → 应覆盖(同花顺 re-add 会重新给价/日)
	day2 := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	if err := s.UpsertWatchlistEntry(ctx, code, nil, "SH", "17", f64(20), &day2, nil); err != nil {
		t.Fatal(err)
	}
	live, err = s.ListWatchlistEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got *float64
	var on *time.Time
	for _, e := range live {
		if e.Code == code {
			got, on = e.AddedPrice, e.AddedOn
		}
	}
	if got == nil || *got != 20 {
		t.Fatalf("re-add 应覆盖为新价 20, got %v", got)
	}
	if on == nil || !on.Equal(day2) {
		t.Fatalf("re-add 应覆盖为新日, got %v", on)
	}
}

// ④ 🔴 红线:加入价/日**不被 entity-build 的 UpsertEntity 覆盖**。
// 这条钉死「价/日不能进 entities.detail」的理由 —— 本次最关键的回归测试。
func TestWatchlistEntrySurvivesEntityBuild(t *testing.T) {
	ctx, s, clean := openWatchTestStore(t)
	code := clean(uniqueCode(t))

	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := s.UpsertWatchlistEntry(ctx, code, nil, "SH", "17", f64(12.34), &day, nil); err != nil {
		t.Fatal(err)
	}

	// 模拟 entity-build 每日 upsert:同 code 的公司实体,**不设 status**(空 = 保持既有 watch)。
	// detail 里带 code —— 与 entity-build 落库形态一致。
	name := "t20-测试票-" + code
	detail, _ := json.Marshal(map[string]string{"code": code, "source": "entity-build"})
	entID, _, err := s.UpsertEntity(ctx, &model.Entity{
		Type: "company", Name: name, Aliases: json.RawMessage(`[]`), Detail: detail,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = s.Pool.Exec(ctx, `DELETE FROM entities WHERE id=$1`, entID) })

	got, err := s.ListWatchlistEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var price *float64
	var on *time.Time
	for _, e := range got {
		if e.Code == code {
			price, on = e.AddedPrice, e.AddedOn
		}
	}
	if price == nil || *price != 12.34 {
		t.Fatalf("entity-build 后加入价被改: %v(应为 12.34)", price)
	}
	if on == nil || !on.Equal(day) {
		t.Fatalf("entity-build 后加入日被改: %v", on)
	}
	// 对照:实体自身 detail 已被 UpsertEntity 覆盖为本次写入值(证明「每轮覆盖」的地雷真实存在,
	// 也因此价/日绝不可能放 detail —— 这正是本表存在的理由)。
	var entDetail []byte
	if err := s.Pool.QueryRow(ctx, `SELECT detail FROM entities WHERE id=$1`, entID).Scan(&entDetail); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]string
	if err := json.Unmarshal(entDetail, &parsed); err != nil || parsed["source"] != "entity-build" {
		t.Fatalf("entities.detail 应被覆盖为本次写入, got %s (err=%v)", entDetail, err)
	}
}

// TaskRunSlotDone:只有 success 才去重;failed 必须允许重试。
func TestTaskRunSlotDone(t *testing.T) {
	ctx, s, _ := openWatchTestStore(t)
	slot := "TESTSLOT#" + uniqueCode(t)
	since := time.Now().Add(-time.Minute)

	id, err := s.StartTaskRun(ctx, "watch-sync-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = s.Pool.Exec(ctx, `DELETE FROM task_runs WHERE id=$1`, id) })

	// failed 不算「已跑」→ 允许重试
	if err := s.FinishTaskRun(ctx, id, "failed", "boom", map[string]any{"slot": slot}); err != nil {
		t.Fatal(err)
	}
	if done, err := s.TaskRunSlotDone(ctx, "watch-sync-test", slot, since); err != nil || done {
		t.Fatalf("failed 不应算已跑, done=%v err=%v", done, err)
	}
	// 改成 success → 算已跑
	if err := s.FinishTaskRun(ctx, id, "success", "", map[string]any{"slot": slot}); err != nil {
		t.Fatal(err)
	}
	if done, err := s.TaskRunSlotDone(ctx, "watch-sync-test", slot, since); err != nil || !done {
		t.Fatalf("success 应算已跑, done=%v err=%v", done, err)
	}
	// 不同 slot → 不算
	if done, _ := s.TaskRunSlotDone(ctx, "watch-sync-test", slot+"-x", since); done {
		t.Fatal("不同 slot 不应算已跑")
	}
}
