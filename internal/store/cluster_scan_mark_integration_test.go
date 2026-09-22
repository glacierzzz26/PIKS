package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"piks/internal/store"
)

// issue #75 收敛机制集成测试(标记列 + 窗口):
//
//  1. 标记后不再进正常 pass 池(ListUnclusteredEvents);
//  2. 🔴 但**仍在** ScannedEventsSince 里 —— 这是「标记列 ≠ 永久漏召回」的唯一保证:
//     少了它,被标记事件会从正常 pass 与重审视两处同时消失;
//  3. 窗口口径按 cluster_scanned_at 过滤;
//  4. 窗内的「刚并入新成员的老簇」仍可见(ListActiveClusterRepresentativesSince 用
//     MAX(member.created_at),而非代表的 created_at);
//  5. MarkEventsScanned 不动已归簇事件(cluster_id IS NOT NULL 的并发护栏)。
//
// 用自建隔离数据(前缀 T75TEST),不依赖也不污染真实簇。
func TestClusterScanMarkConvergence(t *testing.T) {
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
	title := "T75TEST-" + suffix

	var srcID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sources(name, source_type, status) VALUES ($1,'news','active') RETURNING id`,
		"t75-机构-"+suffix).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	var docID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO raw_documents(source_id, title, content, content_hash, status)
		 VALUES ($1,$2,'x',$3,'processed') RETURNING id`,
		srcID, title, "t75-"+suffix).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE source_id=$1`, srcID)
		_, _ = pool.Exec(ctx, `DELETE FROM raw_documents WHERE id=$1`, docID)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id=$1`, srcID)
	})

	mkEvent := func(status string) string {
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO events(raw_document_id, title, event_type, confidence, status, source_id)
			 VALUES ($1,$2,'company',0.8,$3,$4) RETURNING id`,
			docID, title, status, srcID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	// 三条未扫描事件。
	idA, idB, idC := mkEvent("extracted"), mkEvent("extracted"), mkEvent("extracted")

	inPool := func(id string) bool {
		evs, err := s.ListUnclusteredEvents(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range evs {
			if e.ID == id {
				return true
			}
		}
		return false
	}
	countScanned := func(since time.Time) int {
		evs, err := s.ScannedEventsSince(ctx, since)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, e := range evs {
			if e.SourceID != nil && *e.SourceID == srcID {
				n++
			}
		}
		return n
	}

	if !inPool(idA) || !inPool(idB) {
		t.Fatal("新事件应在未聚类池中")
	}

	// 标记 A、B(模拟「本轮比对过、无对端」)。
	n, err := s.MarkEventsScanned(ctx, []string{idA, idB})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows marked, got %d", n)
	}

	// 1. 标记后不再进正常 pass 池(收敛)。
	if inPool(idA) || inPool(idB) {
		t.Error("已扫描事件不应再进 ListUnclusteredEvents(池必须收敛)")
	}
	if !inPool(idC) {
		t.Error("未扫描事件应仍在池中")
	}

	// 2. 🔴 但必须出现在扫描事件查询里(否则永久漏召回)。
	scanned := countScanned(time.Time{})
	if scanned != 2 {
		t.Fatalf("ScannedEventsSince(不限) 应含 2 条已标记事件,得 %d", scanned)
	}
	// 3. 窗口:未来时刻 → 0 条。
	if got := countScanned(time.Now().Add(time.Hour)); got != 0 {
		t.Errorf("窗口在未来应不含任何已标记事件,得 %d", got)
	}
	// 窗口:过去 → 2 条。
	if got := countScanned(time.Now().Add(-time.Hour)); got != 2 {
		t.Errorf("窗口在过去应含 2 条已标记事件,得 %d", got)
	}

	// 4. 已归簇事件不得被标记(并发护栏:cluster_id IS NOT NULL)。
	var clusterID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO event_clusters(title, status) VALUES ($1,'active') RETURNING id`, title).Scan(&clusterID); err != nil {
		t.Fatal(err)
	}
	// ⚠️ 清簇必须**先解引用再删簇**:events.cluster_id 有 FK 指向 event_clusters,
	// 而 t.Cleanup 是 LIFO —— 后注册的先跑,若只注册「删簇」,它会早于「删事件」执行,
	// FK 拦住删除 ⇒ 静默残留一行 active 空簇(实测踩过)。
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `UPDATE events SET cluster_id=NULL WHERE id=$1`, idC)
		_, _ = pool.Exec(ctx, `DELETE FROM event_clusters WHERE id=$1`, clusterID)
	})
	if _, err := pool.Exec(ctx, `UPDATE events SET cluster_id=$2 WHERE id=$1`, idC, clusterID); err != nil {
		t.Fatal(err)
	}
	if n, err := s.MarkEventsScanned(ctx, []string{idC}); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Errorf("已归簇事件不应被盖扫描水位,却改了 %d 行", n)
	}
}
