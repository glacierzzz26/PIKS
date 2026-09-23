package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// ApplyClusters 落 canonical_event_id 的集成测试(issue #83 P-2/P-3)。
// 临时库自建 + migrate。需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
//
// 断言:建簇后 event_clusters.canonical_event_id = 分量内代表(P6 规则:无元数据时退化为最早创建),
// 且代表在 events 里保留非 merged 状态、其余成员被标 merged。
func TestApplyClustersWritesCanonicalEventID(t *testing.T) {
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set (integration off by default)")
	}
	dsn := os.Getenv("PIKS_DATABASE_URL")
	if dsn == "" {
		t.Skip("PIKS_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	t.Cleanup(admin.Close)
	tmpDB := fmt.Sprintf("piks_p2cl_%d", os.Getpid())
	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+tmpDB); err != nil {
		t.Fatalf("create tmp db: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB) })
	pool, err := store.Open(ctx, replaceDBNameCl(dsn, tmpDB))
	if err != nil {
		t.Fatalf("open tmp pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := store.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := store.New(pool)

	srcID := ""
	if err := pool.QueryRow(ctx, `INSERT INTO sources(name, source_type, status) VALUES('P2CL机构','news','active') RETURNING id`).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	docID := ""
	if err := pool.QueryRow(ctx, `INSERT INTO raw_documents(source_id,title,content,content_hash,status) VALUES($1,'t','c','p2cl','processed') RETURNING id`, srcID).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	events := make([]model.Event, 3)
	for i := range events {
		ev := model.Event{
			Title: fmt.Sprintf("P2CL-%d", i), EventType: "company",
			CreatedAt:  t0.Add(time.Duration(i) * time.Hour), // e0 最早 → 代表
			Confidence: 0.5,
			Status:     "extracted",
		}
		id, err := s.CreateEvent(ctx, &ev)
		if err != nil {
			t.Fatal(err)
		}
		ev.ID = id
		events[i] = ev
		// CreateEvent 不写 created_at;显式对齐,便于断言代表=最早。
		if _, err := pool.Exec(ctx, `UPDATE events SET created_at=$1, raw_document_id=$2, source_id=$3 WHERE id=$4`, ev.CreatedAt, docID, srcID, id); err != nil {
			t.Fatal(err)
		}
	}

	comps := [][]int{{0, 1, 2}}
	if _, err := ApplyClusters(ctx, s, events, comps); err != nil {
		t.Fatalf("ApplyClusters: %v", err)
	}

	// 找到该簇(唯一 active 簇)。
	var cid string
	if err := pool.QueryRow(ctx, `SELECT id FROM event_clusters WHERE status='active'`).Scan(&cid); err != nil {
		t.Fatal(err)
	}
	cl, err := s.GetEventClusterByID(ctx, cid)
	if err != nil {
		t.Fatalf("GetEventClusterByID: %v", err)
	}
	if cl.CanonicalEventID == nil || *cl.CanonicalEventID != events[0].ID {
		t.Errorf("canonical_event_id 应为最早创建的 %s, got %v", events[0].ID, cl.CanonicalEventID)
	}
	// 代表保留非 merged;其余两个 merged。
	got0, _ := s.GetEventByID(ctx, events[0].ID)
	if got0.Status == "merged" {
		t.Errorf("代表不得被标 merged, got %q", got0.Status)
	}
	for _, e := range events[1:] {
		ge, err := s.GetEventByID(ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		if ge.Status != "merged" {
			t.Errorf("%s 应被标 merged, got %q", e.ID, ge.Status)
		}
	}
}

// replaceDBNameCl 把 DSN 的库名段换成 db(与 store/web 包的同类 helper 同形)。
func replaceDBNameCl(dsn, db string) string {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return dsn
	}
	slash := strings.Index(dsn[at:], "/")
	if slash < 0 {
		return dsn
	}
	slash += at
	rest := dsn[slash+1:]
	if q := strings.Index(rest, "?"); q >= 0 {
		return dsn[:slash+1] + db + rest[q:]
	}
	return dsn[:slash+1] + db
}

// ApplyClusters 的 P6 代表选取规则端到端(issue #83 P-3):**有直接链接的成员胜出**,即便它
// 创建更晚 —— 旧口径(最早创建)会选 e0,新规则选带 url 的 e1。
func TestApplyClustersRepresentativePrefersURL(t *testing.T) {
	if os.Getenv("PIKS_TEST_INTEGRATION") == "" {
		t.Skip("PIKS_TEST_INTEGRATION not set (integration off by default)")
	}
	dsn := os.Getenv("PIKS_DATABASE_URL")
	if dsn == "" {
		t.Skip("PIKS_DATABASE_URL not set")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	t.Cleanup(admin.Close)
	tmpDB := fmt.Sprintf("piks_p3cl_%d", os.Getpid())
	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+tmpDB); err != nil {
		t.Fatalf("create tmp db: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB) })
	pool, err := store.Open(ctx, replaceDBNameCl(dsn, tmpDB))
	if err != nil {
		t.Fatalf("open tmp pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := store.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := store.New(pool)

	srcID := ""
	if err := pool.QueryRow(ctx, `INSERT INTO sources(name, source_type, status) VALUES('P3CL机构','news','active') RETURNING id`).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	// 两篇 raw:第一篇无 url,第二篇有 url。
	docNoURL := ""
	if err := pool.QueryRow(ctx, `INSERT INTO raw_documents(source_id,title,content,content_hash,status) VALUES($1,'n','无链接正文','p3cl-n','processed') RETURNING id`, srcID).Scan(&docNoURL); err != nil {
		t.Fatal(err)
	}
	docURL := ""
	if err := pool.QueryRow(ctx, `INSERT INTO raw_documents(source_id,title,content,content_hash,status,url) VALUES($1,'u','有链接正文','p3cl-u','processed','https://example.com/x') RETURNING id`, srcID).Scan(&docURL); err != nil {
		t.Fatal(err)
	}

	t0 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	events := make([]model.Event, 2)
	for i, doc := range []string{docNoURL, docURL} {
		ev := model.Event{Title: fmt.Sprintf("P3CL-%d", i), EventType: "company",
			Confidence: 0.5, Status: "extracted"}
		id, err := s.CreateEvent(ctx, &ev)
		if err != nil {
			t.Fatal(err)
		}
		ev.ID = id
		events[i] = ev
		// e0 最早(旧口径代表),e1 更晚但有链接 ⇒ 新规则应选 e1。
		if _, err := pool.Exec(ctx, `UPDATE events SET created_at=$1, raw_document_id=$2, source_id=$3 WHERE id=$4`,
			t0.Add(time.Duration(i)*time.Hour), doc, srcID, id); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := ApplyClusters(ctx, s, events, [][]int{{0, 1}}); err != nil {
		t.Fatalf("ApplyClusters: %v", err)
	}
	var cid string
	if err := pool.QueryRow(ctx, `SELECT id FROM event_clusters WHERE status='active'`).Scan(&cid); err != nil {
		t.Fatal(err)
	}
	cl, err := s.GetEventClusterByID(ctx, cid)
	if err != nil {
		t.Fatalf("GetEventClusterByID: %v", err)
	}
	if cl.CanonicalEventID == nil || *cl.CanonicalEventID != events[1].ID {
		t.Errorf("P6 规则应选**有链接**的成员 %s(即便更晚), got %v", events[1].ID, cl.CanonicalEventID)
	}
	got0, _ := s.GetEventByID(ctx, events[0].ID)
	if got0.Status != "merged" {
		t.Errorf("无链接的更早成员应被标 merged, got %q", got0.Status)
	}
}
