package store_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// ListClusterMembersWithFacts 集成测试(issue #49 T3 冲突检测的数据来源)。
// 需真库(同 smoke_test 的 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关)。
//
// 自建隔离数据(唯一名 + t.Cleanup 删除),验证:
//
//	① **每个成员各自的 facts 都读出**(冲突检测要逐成员比对,不能按机构去重);
//	② merged 成员的 facts 也在(被并入的那家正是另一个「版本」,漏它就等于放弃了比对对象);
//	③ 空输入不查库。
func TestListClusterMembersWithFacts(t *testing.T) {
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
	srcA := "t3-机构甲-" + suffix
	srcB := "t3-机构乙-" + suffix

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE title LIKE $1`, "T3TEST%")
		_, _ = pool.Exec(ctx, `DELETE FROM raw_documents WHERE title LIKE $1`, "T3TEST%")
		_, _ = pool.Exec(ctx, `DELETE FROM event_clusters WHERE title LIKE $1`, "T3TEST%")
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE name IN ($1,$2)`, srcA, srcB)
	})

	a := &model.Source{Name: srcA, SourceType: "news"}
	b := &model.Source{Name: srcB, SourceType: "news"}
	for _, x := range []*model.Source{a, b} {
		if err := s.CreateSource(ctx, x); err != nil {
			t.Fatalf("create source: %v", err)
		}
	}

	clusterID, err := s.CreateEventCluster(ctx, &model.EventCluster{Title: "T3TEST 簇-" + suffix})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	mkDoc := func(srcID, title string) string {
		t.Helper()
		doc := &model.RawDocument{
			SourceID: srcID, Title: &title, Content: title, ContentHash: "t3-" + title,
		}
		ok, err := s.InsertRawDocument(ctx, doc)
		if err != nil || !ok {
			t.Fatalf("insert raw doc %q: ok=%v err=%v", title, ok, err)
		}
		var id string
		if err := pool.QueryRow(ctx,
			`SELECT id FROM raw_documents WHERE source_id=$1 AND content_hash=$2`, srcID, doc.ContentHash).Scan(&id); err != nil {
			t.Fatalf("lookup raw doc: %v", err)
		}
		return id
	}

	// 两家对同一个量给出**不同**的数(3% vs 2%)—— 这正是冲突检测要抓的分叉。
	mkEv := func(docID, srcID, title, status string, facts []string) string {
		t.Helper()
		fj, _ := json.Marshal(facts)
		id, err := s.CreateEvent(ctx, &model.Event{
			RawDocumentID: &docID, Title: title, EventType: "company",
			SourceID: &srcID, Status: status, Facts: fj,
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

	docA := mkDoc(a.ID, "T3TEST 甲")
	docB := mkDoc(b.ID, "T3TEST 乙")
	evA := mkEv(docA, a.ID, "T3TEST 甲", "extracted", []string{"公司股东拟减持不超过3%的股份"})
	evB := mkEv(docB, b.ID, "T3TEST 乙", "merged", []string{"公司股东拟减持不超过2%的股份"})

	got, err := s.ListClusterMembersWithFacts(ctx, []string{clusterID})
	if err != nil {
		t.Fatalf("ListClusterMembersWithFacts: %v", err)
	}
	mem := got[clusterID]
	if len(mem) != 2 {
		t.Fatalf("应得 2 个成员(含 merged),实为 %d: %+v", len(mem), mem)
	}
	byID := map[string]store.ClusterMember{}
	for _, m := range mem {
		byID[m.EventID] = m
	}
	// ① canonical 与 ② merged 成员的 facts 都要在,facts 内容各自独立。
	for _, want := range []struct{ id, src, fact string }{
		{evA, srcA, "公司股东拟减持不超过3%的股份"},
		{evB, srcB, "公司股东拟减持不超过2%的股份"},
	} {
		m, ok := byID[want.id]
		if !ok {
			t.Fatalf("成员 %s(%s)缺失: %+v", want.id, want.src, mem)
		}
		if m.Source != want.src {
			t.Errorf("成员 %s 机构名 = %q, want %q", want.id, m.Source, want.src)
		}
		var facts []string
		if err := json.Unmarshal(m.Facts, &facts); err != nil {
			t.Fatalf("成员 %s facts 解析失败: %v", want.id, err)
		}
		if len(facts) != 1 || facts[0] != want.fact {
			t.Errorf("成员 %s facts = %v, want [%q]", want.id, facts, want.fact)
		}
	}

	// ③ 空输入不查库。
	if out, err := s.ListClusterMembersWithFacts(ctx, nil); err != nil || out != nil {
		t.Fatalf("空输入应返回 (nil,nil), got %v %v", out, err)
	}
}
