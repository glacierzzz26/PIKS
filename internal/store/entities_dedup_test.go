package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"piks/internal/store"
)

// TestEnsureCompanyEntityDedupesSpacedName 回归 issue #6:entity-build 落过带空格名
// (「金 螳 螂」),交易截图导入用无空格正式名再补全时,必须**命中同一条**而非新建,
// 否则同码重复实体。修复前实测会插出第二条(见 PR 说明的复现记录)。
func TestEnsureCompanyEntityDedupesSpacedName(t *testing.T) {
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
	// 用独立 code,避免与真实数据相撞;结束清理彻底不留痕。
	const code = "999001"
	const spaced = "测 试 股"
	const compact = "测试股"
	t.Cleanup(func() { pool.Close() })
	// 按 code + 按名双清:被测定函数若回归(造重复),新建的那条 name 是紧凑名而非空格名,
	// 只按 id/空格名清会漏,故补一条按 code 清(测试自己用 999xxx 段,不碰真实数据)。
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx,
			`DELETE FROM entities WHERE type='company' AND detail->>'code' LIKE '999%'`)
		_, _ = pool.Exec(ctx,
			`DELETE FROM entities WHERE type='company' AND name IN
			   ('测试股','测试 股','金螳螂','脏码股','脏 码 股','新建股','新 建 股','验证股','验 证 股')`)
	})

	// 1. 直接落一条「带空格 + 正确 code」的实体(模拟 issue #6 的脏数据)
	if _, err := pool.Exec(ctx, `
		INSERT INTO entities (type, name, aliases, description, detail, status)
		VALUES ('company', $1, '[]'::jsonb, NULL, jsonb_build_object('code', $2::text), 'active')`,
		spaced, code); err != nil {
		t.Fatalf("seed spaced entity: %v", err)
	}
	var seedID string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM entities WHERE type='company' AND name=$1`, spaced).Scan(&seedID); err != nil {
		t.Fatal(err)
	}

	// 2. 交易导入用无空格名补全 → 必须命中那条脏实体,不新建
	gotID, err := s.EnsureCompanyEntity(ctx, code, compact)
	if err != nil {
		t.Fatalf("EnsureCompanyEntity(compact): %v", err)
	}
	if gotID != seedID {
		t.Fatalf("无空格名未命中既有实体: got=%s want=%s(即新建了重复实体)", gotID, seedID)
	}
	assertCompanyCount(t, pool, code, 1)

	// 3. 反过来:库里是干净名,用带空格名补全 → 归一后同样命中(不新建)。
	//    用**另一个**名字,避免命中步骤 1 那条空格实体。
	const code2 = "999002"
	const clean = "验证股"
	cleanID, err := s.EnsureCompanyEntity(ctx, code2, clean)
	if err != nil {
		t.Fatalf("EnsureCompanyEntity(create clean): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM entities WHERE id=$1`, cleanID)
	})
	got2, err := s.EnsureCompanyEntity(ctx, code2, "验 证 股")
	if err != nil {
		t.Fatalf("EnsureCompanyEntity(spaced): %v", err)
	}
	if got2 != cleanID {
		t.Fatalf("带空格名未归一到既有实体: got=%s want=%s", got2, cleanID)
	}

	// 4. 库里无该名 + 用带空格名补全 → 新建的实体名必须已是归一后的紧凑名
	const code3 = "999003"
	freshID, err := s.EnsureCompanyEntity(ctx, code3, "新 建 股")
	if err != nil {
		t.Fatalf("EnsureCompanyEntity(fresh spaced): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM entities WHERE id=$1`, freshID)
	})
	var newName string
	if err := pool.QueryRow(ctx, `SELECT name FROM entities WHERE id=$1`, freshID).Scan(&newName); err != nil {
		t.Fatal(err)
	}
	if newName != "新建股" {
		t.Fatalf("新建实体名未归一: got=%q want=%q", newName, "新建股")
	}

	// 5. 英文多词名不受影响(空格有意义,不得被压掉)
	const code4 = "999004"
	enID, err := s.EnsureCompanyEntity(ctx, code4, "Test Holdings")
	if err != nil {
		t.Fatalf("EnsureCompanyEntity(english): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM entities WHERE id=$1`, enID)
	})
	var enName string
	if err := pool.QueryRow(ctx, `SELECT name FROM entities WHERE id=$1`, enID).Scan(&enName); err != nil {
		t.Fatal(err)
	}
	if enName != "Test Holdings" {
		t.Fatalf("英文多词名被误归一: got=%q", enName)
	}

	// 6. **生产实况**:空格实体那条的 detail.code 是脏的(名称当 code,issue #2),
	//    此时 code 查询命中不了,只有「去空白名称匹配」能救回 —— 否则又造一条重复。
	//    对应 issue #6 表里的「金螳螂 | "金螳螂"(脏)」两行。
	const dirtyName = "脏 码 股"
	if _, err := pool.Exec(ctx, `
		INSERT INTO entities (type, name, aliases, description, detail, status)
		VALUES ('company', $1, '[]'::jsonb, NULL, jsonb_build_object('code', $2::text), 'active')`,
		dirtyName, "脏码股"); err != nil { // detail.code = 名称(脏)
		t.Fatalf("seed dirty-code entity: %v", err)
	}
	var dirtyID string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM entities WHERE type='company' AND name=$1`, dirtyName).Scan(&dirtyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM entities WHERE id=$1`, dirtyID)
	})
	got3, err := s.EnsureCompanyEntity(ctx, "999005", "脏码股") // 带正确 code + 紧凑名
	if err != nil {
		t.Fatalf("EnsureCompanyEntity(dirty-code row): %v", err)
	}
	if got3 != dirtyID {
		t.Fatalf("脏 code 行未按名称匹配到: got=%s want=%s(又造了重复)", got3, dirtyID)
	}
}

// assertCompanyCount 该 code 的公司实体数(归一生效时应为 1)。
func assertCompanyCount(t *testing.T, pool *pgxpool.Pool, code string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM entities WHERE type='company' AND detail->>'code'=$1`,
		code).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != want {
		t.Fatalf("code %s 实体数 = %d, want %d", code, got, want)
	}
}
