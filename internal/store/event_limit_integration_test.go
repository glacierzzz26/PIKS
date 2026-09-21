package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"piks/internal/store"
)

// UnclusteredEventsTruncated 集成测试(issue #53 验收):
// -limit>0 时必须能报出「未聚类事件多于 limit」,让 cmd/cluster 把截断写进 task_runs.meta;
// -limit<=0(不限)恒为 false。用自建隔离数据,不依赖也不污染真实簇。
func TestUnclusteredEventsTruncated(t *testing.T) {
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
	srcName := "t53-机构-" + suffix
	var srcID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sources(name, source_type, status) VALUES ($1,'news','active') RETURNING id`,
		srcName).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	var docID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO raw_documents(source_id, title, content, content_hash, status)
		 VALUES ($1,'T53TEST','x',$2,'processed') RETURNING id`,
		srcID, "t53-"+suffix).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE raw_document_id=$1`, docID)
		_, _ = pool.Exec(ctx, `DELETE FROM raw_documents WHERE id=$1`, docID)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id=$1`, srcID)
	})

	// 建 3 条未聚类事件
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx,
			`INSERT INTO events(raw_document_id, title, event_type, confidence, status, source_id)
			 VALUES ($1,$2,'company',0.8,'extracted',$3)`,
			docID, "T53TEST-"+suffix, srcID); err != nil {
			t.Fatal(err)
		}
	}

	// 不限:恒 false
	if got, err := s.UnclusteredEventsTruncated(ctx, 0); err != nil || got {
		t.Errorf("limit=0(不限)应为 false,得 %v err=%v", got, err)
	}
	// limit 小于总量:我们这 3 条 + 库里既有 → 必然截断
	if got, err := s.UnclusteredEventsTruncated(ctx, 1); err != nil {
		t.Fatal(err)
	} else if !got {
		t.Error("limit=1 而库中未聚类事件 >1,应报截断")
	}
}
