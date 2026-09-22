package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"piks/internal/store"
)

// ListAffectedTermEvents 集成测试(issue #71 验收):
// 插入一条 affected 为 **JSON null** 的事件(事故成因),ListAffectedTermEvents
// **不得报错** —— 修复前它会抛 SQLSTATE 22023「cannot extract elements from a
// scalar」,使 entity-build 整条命令崩掉。修复后:坏行被跳过,好行正常返回。
//
// 用自建隔离数据,不依赖也不污染真实事件。
func TestListAffectedTermEventsToleratesNullAffected(t *testing.T) {
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

	suffix := time.Now().Format("20060102150405.000000")
	var srcID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sources(name, source_type, status) VALUES ($1,'news','active') RETURNING id`,
		"t71-机构-"+suffix).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	var docID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO raw_documents(source_id, title, content, content_hash, status)
		 VALUES ($1,'T71TEST','x',$2,'processed') RETURNING id`,
		srcID, "t71-"+suffix).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE raw_document_id=$1`, docID)
		_, _ = pool.Exec(ctx, `DELETE FROM raw_documents WHERE id=$1`, docID)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id=$1`, srcID)
	})

	// ① 事故行:affected 为 JSON null(绕过写入守卫,直接落库以复现历史脏数据)
	if _, err := pool.Exec(ctx,
		`INSERT INTO events(raw_document_id, title, event_type, confidence, status, source_id, affected)
		 VALUES ($1,$2,'company',0.8,'extracted',$3,'null'::jsonb)`,
		docID, "T71TEST-null-"+suffix, srcID); err != nil {
		t.Fatal(err)
	}
	// ② 好行:正常数组(须仍被收割到)
	term := "T71术语" + suffix
	if _, err := pool.Exec(ctx,
		`INSERT INTO events(raw_document_id, title, event_type, confidence, status, source_id, affected)
		 VALUES ($1,$2,'company',0.8,'extracted',$3,$4::jsonb)`,
		docID, "T71TEST-ok-"+suffix, srcID, `["`+term+`"]`); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListAffectedTermEvents(ctx)
	if err != nil {
		t.Fatalf("ListAffectedTermEvents 报错(issue #71 未修):%v", err)
	}
	if _, ok := got[term]; !ok {
		t.Errorf("正常数组行未被收割:期望含 %q", term)
	}
}
