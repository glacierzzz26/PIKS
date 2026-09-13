package store_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"piks/internal/model"
	"piks/internal/store"
)

// TestUpsertEntityKeepsWatch 回归:entity-build 每日 upsert 不带 status,
// 若把空 status 当 'active' 会清空自选(watch)。设计要求空 status = 保持既有。
// 与其它集成测试同开关;用唯一 name 保证可重复,结束清理不留痕。
func TestUpsertEntityKeepsWatch(t *testing.T) {
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
	name := "测试自选保持_" + t.Name()
	t.Cleanup(func() { pool.Close() })
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM entities WHERE type='concept' AND name=$1`, name)
	})

	// 1. 新建显式 watch
	if _, _, err := s.UpsertEntity(ctx, &model.Entity{
		Type: "concept", Name: name, Status: "watch",
	}); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	assertStatus(t, pool, name, "watch")

	// 2. 模拟 entity-build:不带 status 的 upsert(别名/detail 变更触发写路径)
	alias := json.RawMessage(`["别名甲"]`)
	if _, _, err := s.UpsertEntity(ctx, &model.Entity{
		Type: "concept", Name: name, Aliases: alias,
		Detail: json.RawMessage(`{"src":"test"}`),
	}); err != nil {
		t.Fatalf("upsert no-status: %v", err)
	}
	assertStatus(t, pool, name, "watch") // 地雷复现点:修复前会被改成 active

	// 3. 再跑一次同样内容:零变更(幂等),仍 watch
	created, err := func() (bool, error) {
		_, c, e := s.UpsertEntity(ctx, &model.Entity{
			Type: "concept", Name: name, Aliases: alias,
			Detail: json.RawMessage(`{"src":"test"}`),
		})
		return c, e
	}()
	if err != nil || created {
		t.Fatalf("idempotent: created=%v err=%v", created, err)
	}
	assertStatus(t, pool, name, "watch")

	// 4. 显式改 archived(截图镜像移出)仍生效
	if _, _, err := s.UpsertEntity(ctx, &model.Entity{
		Type: "concept", Name: name, Status: "archived",
	}); err != nil {
		t.Fatalf("explicit archived: %v", err)
	}
	assertStatus(t, pool, name, "archived")
}

func assertStatus(t *testing.T, pool *pgxpool.Pool, name, want string) {
	t.Helper()
	var got string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM entities WHERE type='concept' AND name=$1`, name).Scan(&got); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}
