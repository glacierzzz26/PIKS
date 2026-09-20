package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// ListClusterSources 集成测试(issue #48 T2 验收「簇内可见各源来源」)。
// 需真库(同 smoke_test 的 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关)。
//
// 自建隔离数据(唯一名 + t.Cleanup 删除),不依赖也不污染真实簇:
// 建 2 机构 × (canonical + merged 成员) 的簇,验证
//
//	① merged 成员的来源也被读出(否则跨源簇只剩 1 个来源,多源印证白做);
//	② 同机构多条按机构去重(优先带 url 的那条);
//	③ 上游一级源(金十 extra.source)如实带出。
func TestListClusterSources(t *testing.T) {
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
	srcA := "t2-机构甲-" + suffix
	srcB := "t2-机构乙-" + suffix

	// 清理(逆序:事件 → 原始文档 → 源 → 簇)。events.cluster_id 有 FK 到 event_clusters。
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE title LIKE $1`, "T2TEST%")
		_, _ = pool.Exec(ctx, `DELETE FROM raw_documents WHERE title LIKE $1`, "T2TEST%")
		_, _ = pool.Exec(ctx, `DELETE FROM event_clusters WHERE title LIKE $1`, "T2TEST%")
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE name IN ($1,$2)`, srcA, srcB)
	})

	a := &model.Source{Name: srcA, SourceType: "news"}
	b := &model.Source{Name: srcB, SourceType: "news"}
	for _, x := range []*model.Source{a, b} {
		if err := s.CreateSource(ctx, x); err != nil {
			t.Fatalf("create source: %v", err)
		}
	}

	clusterID, err := s.CreateEventCluster(ctx, &model.EventCluster{Title: "T2TEST 簇-" + suffix})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	// 机构甲:两条记录(一条无 url、一条有 url)→ 去重后应只留**带 url** 的那条。
	// 机构乙:一条,带上游一级源 extra.source = 新华社。
	mkDoc := func(srcID, title, url string, extra string) string {
		t.Helper()
		var u *string
		if url != "" {
			u = &url
		}
		doc := &model.RawDocument{
			SourceID: srcID, Title: &title, Content: title,
			ContentHash: "t2-" + title, URL: u,
		}
		if extra != "" {
			doc.Extra = []byte(extra)
		}
		ok, err := s.InsertRawDocument(ctx, doc)
		if err != nil || !ok {
			t.Fatalf("insert raw doc %q: ok=%v err=%v", title, ok, err)
		}
		// InsertRawDocument 不回填 ID,反查。
		var id string
		if err := pool.QueryRow(ctx,
			`SELECT id FROM raw_documents WHERE source_id=$1 AND content_hash=$2`, srcID, doc.ContentHash).Scan(&id); err != nil {
			t.Fatalf("lookup raw doc: %v", err)
		}
		return id
	}

	newEv := func(docID, srcID, title, status string) string {
		t.Helper()
		id, err := s.CreateEvent(ctx, &model.Event{
			RawDocumentID: &docID, Title: title, EventType: "company",
			SourceID: &srcID, Status: status,
		})
		if err != nil {
			t.Fatalf("create event: %v", err)
		}
		if status == "merged" {
			if err := s.SetEventCluster(ctx, id, clusterID, "merged"); err != nil {
				t.Fatalf("merge member: %v", err)
			}
		} else if err := s.SetEventClusterNoTouch(ctx, id, clusterID); err != nil {
			t.Fatalf("set canonical: %v", err)
		}
		return id
	}

	docA1 := mkDoc(a.ID, "T2TEST 甲-无链接", "", "")
	docA2 := mkDoc(a.ID, "T2TEST 甲-有链接", "https://example.com/a", "")
	docB := mkDoc(b.ID, "T2TEST 乙", "https://example.com/b", `{"source":"新华社"}`)

	evA1 := newEv(docA1, a.ID, "T2TEST 甲-无链接", "merged")
	evA2 := newEv(docA2, a.ID, "T2TEST 甲-有链接", "extracted") // canonical
	evB := newEv(docB, b.ID, "T2TEST 乙", "merged")
	_ = evA1

	got, err := s.ListClusterSources(ctx, []string{clusterID})
	if err != nil {
		t.Fatalf("ListClusterSources: %v", err)
	}
	srcs := got[clusterID]
	if len(srcs) != 2 {
		t.Fatalf("应得 2 个机构(乙为 merged 成员也必须出现),实为 %d: %+v", len(srcs), srcs)
	}
	byName := map[string]store.ClusterSource{}
	for _, cs := range srcs {
		byName[cs.Source] = cs
	}
	if _, ok := byName[srcB]; !ok {
		t.Fatalf("merged 成员机构乙缺失 —— 簇内来源应含全部成员: %+v", srcs)
	}
	if cs := byName[srcA]; cs.URL == nil || *cs.URL != "https://example.com/a" {
		t.Fatalf("机构甲应去重保留带 url 的那条,实为 %+v", cs)
	}
	if cs := byName[srcB]; cs.Origin == nil || *cs.Origin != "新华社" {
		t.Fatalf("上游一级源 extra.source 未带出: %+v", cs)
	}
	if cs := byName[srcA]; cs.Origin != nil {
		t.Fatalf("无 extra.source 时 origin 应为 NULL,实为 %v", *cs.Origin)
	}
	_ = evA2
	_ = evB

	// 空输入不查库。
	if out, err := s.ListClusterSources(ctx, nil); err != nil || out != nil {
		t.Fatalf("空输入应返回 (nil,nil), got %v %v", out, err)
	}
}
