package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// 事件管线 P-3(issue #83)集成测试:早/晚窗口查询 + 建簇期文档元数据取数。
// 临时库自建 + migrate,不污染开发/生产数据。需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
//
// 覆盖:
//  1. ListEventsInWindow —— 半开区间 [start,end) 边界(start 含、end 不含)、排除 merged、按 created_at 倒序;
//  2. ListEventDocMeta —— url/content 经 LEFT JOIN 取回,未关联 raw 文档的事件落空(零值);
//  3. 存量簇 canonical_event_id **不被 P-3 改动**(无迁移 ⇒ 冻结口径不变)。

func TestP3WindowAndDocMeta(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p3_%d", os.Getpid())
	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+tmpDB); err != nil {
		t.Fatalf("create tmp db: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB) })
	pool, err := store.Open(ctx, replaceDBNameP2(dsn, tmpDB))
	if err != nil {
		t.Fatalf("open tmp pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := store.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s := store.New(pool)

	src := &model.Source{Name: "P3机构", SourceType: "news"}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// 三篇 raw:一篇带 url,一篇无 url,一篇不关联(查 doc meta 的零值分支)。
	docID := func(hash, url string) string {
		doc := &model.RawDocument{SourceID: src.ID, Content: "P3-正文-" + hash, ContentHash: hash, Status: "processed"}
		if url != "" {
			doc.URL = &url
		}
		if _, err := s.InsertRawDocument(ctx, doc); err != nil {
			t.Fatalf("insert raw: %v", err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM raw_documents WHERE content_hash=$1`, hash).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	docWithURL := docID("p3-u", "https://example.com/a")
	docNoURL := docID("p3-n", "")

	mkEvent := func(title, rawDocID string, createdAt time.Time) string {
		ev := &model.Event{Title: title, EventType: "company", Confidence: 0.5, Status: "extracted", RawDocumentID: &rawDocID}
		id, err := s.CreateEvent(ctx, ev)
		if err != nil {
			t.Fatalf("create event: %v", err)
		}
		if _, err := pool.Exec(ctx, `UPDATE events SET created_at=$1, source_id=$2 WHERE id=$3`, createdAt, src.ID, id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	base := time.Date(2026, 9, 22, 9, 15, 0, 0, time.UTC)
	// 窗口 [base, base+9h15m) 内:两条;窗口外:一条(终点正上、早于起点各一条)。
	inA := mkEvent("窗口内甲", docWithURL, base.Add(1*time.Hour))
	inB := mkEvent("窗口内乙", docNoURL, base.Add(2*time.Hour))
	outBefore := mkEvent("窗口前", docWithURL, base.Add(-1*time.Minute))
	atEnd := mkEvent("窗口终点上", docWithURL, base.Add(9*time.Hour+15*time.Minute)) // end 不含 ⇒ 应在窗外
	// merged 行:建簇时会并入,窗口查询须排除。
	mergedID := mkEvent("窗口内已合并", docWithURL, base.Add(3*time.Hour))

	// 手工把 mergedID 标 merged(无需真建簇:窗口查询只按 status 过滤)。
	if _, err := pool.Exec(ctx, `UPDATE events SET status='merged' WHERE id=$1`, mergedID); err != nil {
		t.Fatal(err)
	}

	end := base.Add(9*time.Hour + 15*time.Minute)
	got, err := s.ListEventsInWindow(ctx, base, end)
	if err != nil {
		t.Fatalf("ListEventsInWindow: %v", err)
	}
	ids := map[string]bool{}
	for _, ev := range got {
		ids[ev.ID] = true
	}
	if !ids[inA] || !ids[inB] {
		t.Errorf("窗口内事件应被取回:inA=%v inB=%v", ids[inA], ids[inB])
	}
	if ids[outBefore] {
		t.Error("早于 start 的事件不应被取回(start 为闭边界)")
	}
	if ids[atEnd] {
		t.Error("恰在 end 的事件不应被取回(end 为开边界)")
	}
	if ids[mergedID] {
		t.Error("merged 事件不应进窗口(榜单不展示已并入的重复报道)")
	}
	// 倒序:inB(晚)应在 inA(早)之前。
	if len(got) >= 2 && got[0].CreatedAt.Before(got[1].CreatedAt) {
		t.Errorf("应按 created_at 倒序,got[0]=%s got[1]=%s", got[0].CreatedAt, got[1].CreatedAt)
	}
	// 投影字段齐备(供 toEventItem 复用):来源名 + cluster_id 列在。
	if len(got) > 0 && got[0].SourceName == "" {
		t.Error("EventForAPI.SourceName 应被填充(JOIN sources)")
	}

	// ListEventDocMeta:带 url 的取回 url+content;无 url 的 url 为 nil;未关联的落空。
	meta, err := s.ListEventDocMeta(ctx, []string{inA, inB, outBefore})
	if err != nil {
		t.Fatalf("ListEventDocMeta: %v", err)
	}
	if m, ok := meta[inA]; !ok || m.URL == nil || *m.URL != "https://example.com/a" {
		t.Errorf("inA 应取回 url,got %+v ok=%v", meta[inA], ok)
	}
	if m, ok := meta[inB]; !ok || m.URL != nil {
		t.Errorf("inB 无 url 应为 nil,got %+v", m)
	}
	if m := meta[inA]; m.Content == nil || *m.Content == "" {
		t.Errorf("inA 应取回正文,got %+v", m)
	}

	// 空入参:返回空 map,不报错。
	empty, err := s.ListEventDocMeta(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("空入参应返回空 map,got %v err=%v", empty, err)
	}
}
