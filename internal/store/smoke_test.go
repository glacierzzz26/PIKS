package store_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"piks/internal/model"
	"piks/internal/store"
)

// TestSmoke 集成冒烟:需要已迁移的 Postgres(设 PIKS_DATABASE_URL)。
// 无环境变量时跳过,不阻塞普通单测。
func TestSmoke(t *testing.T) {
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

	src := &model.Source{Name: "smoke-test", SourceType: "news", Config: json.RawMessage(`{}`)}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}
	got, err := s.GetSourceByName(ctx, "smoke-test")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if got.Name != "smoke-test" || got.SourceType != "news" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	t.Logf("source roundtrip ok: %s (%s, status=%s)", got.Name, got.SourceType, got.Status)
}
