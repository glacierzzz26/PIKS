package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// TestSmoke 集成冒烟:需要已迁移的 Postgres(设 PIKS_DATABASE_URL)。
// 双开关缺一即跳过,不阻塞普通单测;普通 `go test ./...` 绝不触碰真库:
//   PIKS_TEST_INTEGRATION 未显式设置 → 跳过(防普通测试污染真库,reconcile 曾误报 smoke 静默源)。
func TestSmoke(t *testing.T) {
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set (integration smoke off by default)")
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
	defer pool.Close()
	s := store.New(pool)

	// 唯一名保证可重复运行(上次残留不冲突),结束清理不留痕。
	name := fmt.Sprintf("smoke-%d", time.Now().UnixNano())
	src := &model.Source{Name: name, SourceType: "news", Config: json.RawMessage(`{}`)}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}
	t.Cleanup(func() {
		_, _ = s.Pool.Exec(ctx, `DELETE FROM sources WHERE id=$1`, src.ID)
	})
	got, err := s.GetSourceByName(ctx, name)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if got.Name != name || got.SourceType != "news" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	t.Logf("source roundtrip ok: %s (%s, status=%s)", got.Name, got.SourceType, got.Status)
}
