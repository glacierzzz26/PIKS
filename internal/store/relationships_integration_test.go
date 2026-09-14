package store_test

import (
	"context"
	"os"
	"testing"

	"piks/internal/model"
	"piks/internal/store"
)

// TestCreateRelationshipDefaultsProperties 回归:调用方不设 properties 时,
// 落库须为 '{}' 而非 NULL(relationships.properties 是 NOT NULL)。
// P6-4 决策边首跑曾因 NULL 触发 23502 —— 这条锁住 store 边界的默认行为。
// 与 smoke_test 同开关(集成需真库);端点用固定 UUID(无 FK,不污染真实数据)。
func TestCreateRelationshipDefaultsProperties(t *testing.T) {
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
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM relationships WHERE source=$1`, "p6-4-test")
	})

	const src = "00000000-0000-4000-8000-0000000000a1"
	const dst = "00000000-0000-4000-8000-0000000000a2"
	srcp := "p6-4-test"
	rel := &model.Relationship{
		FromType: "trade", FromID: src,
		ToType: "event", ToID: dst,
		RelType: "based_on", Source: &srcp,
		// Properties 故意留空 —— 正是回归点。
	}
	if err := s.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("CreateRelationship(未设 properties) 失败: %v", err)
	}

	var props string
	if err := pool.QueryRow(ctx,
		`SELECT properties::text FROM relationships WHERE from_id=$1 AND to_id=$2 AND rel_type='based_on'`,
		src, dst).Scan(&props); err != nil {
		t.Fatal(err)
	}
	if props != "{}" {
		t.Fatalf("properties 应默认 '{}',实为 %q", props)
	}
	// 幂等:重复写入不报错、不新增。
	if err := s.CreateRelationship(ctx, rel); err != nil {
		t.Fatalf("幂等重写失败: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM relationships WHERE from_id=$1`, src).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("幂等应只留 1 行,实为 %d", n)
	}
}
