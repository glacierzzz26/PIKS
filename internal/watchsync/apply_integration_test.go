package watchsync_test

// watchsync.Apply 真库集成测(双开关同仓库惯例)。用**假上游**(零网络),
// 验证编排的红线语义 —— 尤其「名称未解析绝不建实体」与「移出保留历史」。

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"piks/internal/store"
	"piks/internal/ths"
	"piks/internal/watchsync"
)

type fakeUpstream struct {
	stocks  []ths.SelfStock
	details []ths.Detail
	names   map[string]string
}

func (f *fakeUpstream) SelfStocks(context.Context) ([]ths.SelfStock, error) { return f.stocks, nil }
func (f *fakeUpstream) SelfStockDetails(context.Context) ([]ths.Detail, error) {
	return f.details, nil
}
func (f *fakeUpstream) StockNames(context.Context, []string) (map[string]string, error) {
	return f.names, nil
}
func (f *fakeUpstream) UserID() string       { return "test" }
func (f *fakeUpstream) CookieSource() string { return "inject" }

func openApplyTest(t *testing.T) (context.Context, *store.Store) {
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
	return ctx, store.New(pool)
}

// 用 9 开头(未分配的 B 股段)+ 时间尾数,避免与真实自选冲突;清理按前缀。
func mkCode(t *testing.T) string {
	t.Helper()
	ns := time.Now().UnixNano() % 100000
	b := make([]byte, 5)
	for i := 4; i >= 0; i-- {
		b[i] = byte('0' + ns%10)
		ns /= 10
	}
	return "9" + string(b)
}

// ① 🔴 名称未解析 → 不建实体(绝不拿 code 当 name 污染 (type,name) 唯一键)。
func TestApplyDeferredNameNoEntity(t *testing.T) {
	ctx, s := openApplyTest(t)
	code := mkCode(t)
	t.Cleanup(func() { cleanupCode(ctx, s, code) })

	up := &fakeUpstream{
		stocks: []ths.SelfStock{{Code: code, MarketID: "33"}},
		names:  map[string]string{}, // 本地无、realhead 也无 → deferred
	}
	st, err := watchsync.Apply(ctx, s, up, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Deferred) != 1 || st.Deferred[0] != code {
		t.Fatalf("应 deferred 该 code, got %+v", st.Deferred)
	}
	// 必须没有 code 同名实体。
	if id, _ := s.EntityIDByCode(ctx, code); id != nil {
		t.Fatalf("deferred 不应建实体, 却建了 %s", *id)
	}
	// 也不应有 watchlist_entries 行(未建实体即未同步)。
	ents, _ := s.ListWatchlistEntries(ctx)
	for _, e := range ents {
		if e.Code == code {
			t.Fatal("deferred 不应落 watchlist_entries")
		}
	}
}

// ② 名称命中的 add:建实体 + status=watch + 落价/日;且 watchlist_entries.entity_id 指回。
func TestApplyAddCreatesWatchEntity(t *testing.T) {
	ctx, s := openApplyTest(t)
	code := mkCode(t)
	t.Cleanup(func() { cleanupCode(ctx, s, code) })

	price := 12.34
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	up := &fakeUpstream{
		stocks:  []ths.SelfStock{{Code: code, MarketID: "33"}},
		details: []ths.Detail{{Code: code, MarketID: "33", Price: &price, AddedOn: &day}},
		names:   map[string]string{code: "t20-测试" + code},
	}
	if _, err := watchsync.Apply(ctx, s, up, false); err != nil {
		t.Fatal(err)
	}
	id, err := s.EntityIDByCode(ctx, code)
	if err != nil || id == nil {
		t.Fatalf("应建实体, id=%v err=%v", id, err)
	}
	// 实体状态 = watch
	var status string
	if err := s.Pool.QueryRow(ctx, `SELECT status FROM entities WHERE id=$1`, *id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "watch" {
		t.Fatalf("entities.status 应为 watch, got %q", status)
	}
	// entry 的 entity_id 指回
	ents, _ := s.ListWatchlistEntries(ctx)
	var gotEntity *string
	var gotPrice *float64
	for _, e := range ents {
		if e.Code == code {
			gotEntity, gotPrice = e.EntityID, e.AddedPrice
		}
	}
	if gotEntity == nil || *gotEntity != *id {
		t.Fatalf("entry.entity_id 应指回实体, got %v", gotEntity)
	}
	if gotPrice == nil || *gotPrice != 12.34 {
		t.Fatalf("entry.added_price 应为 12.34, got %v", gotPrice)
	}
}

// ③ 反向:上游移出 → entities.status=archived + removed_at 非空 + **行未删**。
func TestApplyRemoveArchivesKeepsRow(t *testing.T) {
	ctx, s := openApplyTest(t)
	code := mkCode(t)
	t.Cleanup(func() { cleanupCode(ctx, s, code) })

	name := "t20-移出" + code
	add := &fakeUpstream{
		stocks: []ths.SelfStock{{Code: code, MarketID: "33"}},
		names:  map[string]string{code: name},
	}
	if _, err := watchsync.Apply(ctx, s, add, false); err != nil {
		t.Fatal(err)
	}
	// 第二轮:上游把该 code 移除,另加一只保底(否则「上游为空」守卫会拦)。
	other := mkCode(t)
	t.Cleanup(func() { cleanupCode(ctx, s, other) })
	rm := &fakeUpstream{
		stocks: []ths.SelfStock{{Code: other, MarketID: "33"}},
		names:  map[string]string{code: other, code: name}, // other 有名字
	}
	if _, err := watchsync.Apply(ctx, s, rm, false); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := s.Pool.QueryRow(ctx,
		`SELECT status FROM entities WHERE type='company' AND detail->>'code'=$1`, code).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "archived" {
		t.Fatalf("移出后 entities.status 应为 archived, got %q", status)
	}
	// 行仍在
	var removedAt *time.Time
	if err := s.Pool.QueryRow(ctx,
		`SELECT removed_at FROM watchlist_entries WHERE code=$1`, code).Scan(&removedAt); err != nil {
		t.Fatalf("移出后行不应被删除: %v", err)
	}
	if removedAt == nil {
		t.Fatal("移出后 removed_at 应非空")
	}
}

// ④ 🔴 上游名单为空 → 必须失败(不误清空)。这是防「cookie 失效把自选全归档」的闸门。
func TestApplyEmptyUpstreamFailsClosed(t *testing.T) {
	ctx, s := openApplyTest(t)
	up := &fakeUpstream{stocks: nil}
	if _, err := watchsync.Apply(ctx, s, up, false); err == nil {
		t.Fatal("上游为空应报错, 绝不静默清空自选")
	}
}

// ⑤ detail 失败 → 降级:名单照常同步,stats.DetailFailed=true(不 fail 整轮)。
func TestApplyDetailFailureDegrades(t *testing.T) {
	ctx, s := openApplyTest(t)
	code := mkCode(t)
	t.Cleanup(func() { cleanupCode(ctx, s, code) })

	up := &failDetailUpstream{code: code}
	st, err := watchsync.Apply(ctx, s, up, false)
	if err != nil {
		t.Fatalf("detail 失败不应 fail 整轮: %v", err)
	}
	if !st.DetailFailed {
		t.Fatal("stats.DetailFailed 应为 true(降级必须可见)")
	}
	// 名单仍同步:实体建成、status=watch、entry 价/日为 NULL(不是 0)。
	id, _ := s.EntityIDByCode(ctx, code)
	if id == nil {
		t.Fatal("detail 失败时名单仍应同步(实体已建)")
	}
	ents, _ := s.ListWatchlistEntries(ctx)
	for _, e := range ents {
		if e.Code == code {
			if e.AddedPrice != nil {
				t.Fatalf("detail 失败时 added_price 应为 NULL, got %v", *e.AddedPrice)
			}
			return
		}
	}
	t.Fatal("应有 watchlist_entries 行")
}

type failDetailUpstream struct{ code string }

func (f *failDetailUpstream) SelfStocks(context.Context) ([]ths.SelfStock, error) {
	return []ths.SelfStock{{Code: f.code, MarketID: "33"}}, nil
}
func (f *failDetailUpstream) SelfStockDetails(context.Context) ([]ths.Detail, error) {
	return nil, errFake
}
func (f *failDetailUpstream) StockNames(_ context.Context, _ []string) (map[string]string, error) {
	return map[string]string{f.code: "t20-detail降级" + f.code}, nil
}
func (f *failDetailUpstream) UserID() string       { return "test" }
func (f *failDetailUpstream) CookieSource() string { return "inject" }

var errFake = errors.New("detail 接口故障")

// cleanupCode 删除本测试建的实体与 watchlist_entries 行。
func cleanupCode(ctx context.Context, s *store.Store, code string) {
	_, _ = s.Pool.Exec(ctx, `DELETE FROM watchlist_entries WHERE code=$1`, code)
	_, _ = s.Pool.Exec(ctx, `DELETE FROM entities WHERE type='company' AND detail->>'code'=$1`, code)
}
