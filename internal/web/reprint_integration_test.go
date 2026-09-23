package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"piks/internal/model"
	"piks/internal/store"
)

// 剥转载读路径端到端(issue #83 P-1 硬验收)。
//
// 走**真实 handler**(`handleAPIEvents` 打真库),断言:
//
//	簇内 3 家机构 = 1 条原创 + 1 条近逐字转载 + 1 条独立改写
//	⇒ source_count(机构数)= 3,而 independent_count(独立来源数)= 2;
//	   转载那家 cluster_sources[].reprint = true,独立改写那家 false。
//
// 反例(防日后有人为凑数下调阈值):同事件、不同措辞的**改写**不得被标转载。
//
// 在**临时库**里跑(自建 + migrate + t.Cleanup 删库),不污染开发/生产数据。
// 需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
func TestEventsIndependentCountIntegration(t *testing.T) {
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
		t.Fatalf("open admin pool: %v", err)
	}
	t.Cleanup(admin.Close)

	tmpDB := fmt.Sprintf("piks_reprint_%d", os.Getpid())
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB); err != nil {
		t.Fatalf("drop tmp db: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+tmpDB); err != nil {
		t.Fatalf("create tmp db: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB) })

	pool, err := store.Open(ctx, replaceDBName(dsn, tmpDB))
	if err != nil {
		t.Fatalf("open tmp pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := store.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate tmp db: %v", err)
	}
	s := store.New(pool)

	// —— 造数据:1 簇 3 机构 ——
	srcNames := []string{"REPRINTTEST甲", "REPRINTTEST乙", "REPRINTTEST丙"}
	srcs := make([]*model.Source, len(srcNames))
	for i, n := range srcNames {
		srcs[i] = &model.Source{Name: n, SourceType: "news"}
		if err := s.CreateSource(ctx, srcs[i]); err != nil {
			t.Fatalf("create source %s: %v", n, err)
		}
	}
	clusterID, err := s.CreateEventCluster(ctx, &model.EventCluster{Title: "REPRINTTEST 簇"})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	// 甲=原创(带电头);乙=近逐字转载(去电头);丙=独立改写(短标题 vs 全文)。
	orig := "【长鑫科技：第五代工艺技术平台实现量产】财联社9月20日电，长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产。"
	reprint := "【长鑫科技：第五代工艺技术平台实现量产】长鑫科技(688825.SH)公告称，公司于2026年9月20日在世界制造业大会上宣布，第五代工艺技术平台正式实现量产。"
	independent := "宇树科技发布Dex5-S灵巧手"

	mkMember := func(srcID, content, status string) {
		t.Helper()
		title := content
		doc := &model.RawDocument{SourceID: srcID, Title: &title, Content: content, ContentHash: "reprint-" + srcID}
		ok, err := s.InsertRawDocument(ctx, doc)
		if err != nil || !ok {
			t.Fatalf("insert raw doc: ok=%v err=%v", ok, err)
		}
		var docID string
		if err := pool.QueryRow(ctx,
			`SELECT id FROM raw_documents WHERE source_id=$1 AND content_hash=$2`, srcID, doc.ContentHash).Scan(&docID); err != nil {
			t.Fatalf("lookup raw doc: %v", err)
		}
		evID, err := s.CreateEvent(ctx, &model.Event{
			RawDocumentID: &docID, Title: title, EventType: "company", SourceID: &srcID, Status: status,
		})
		if err != nil {
			t.Fatalf("create event: %v", err)
		}
		if status == "merged" {
			if err := s.SetEventCluster(ctx, evID, clusterID, "merged"); err != nil {
				t.Fatalf("merge member: %v", err)
			}
		} else if err := s.SetEventClusterNoTouch(ctx, evID, clusterID); err != nil {
			t.Fatalf("set canonical: %v", err)
		}
	}
	mkMember(srcs[0].ID, orig, "extracted")     // canonical
	mkMember(srcs[1].ID, reprint, "merged")     // 转载
	mkMember(srcs[2].ID, independent, "merged") // 独立改写

	// —— 打真实 handler ——
	srv := &Server{store: s}
	rec := httptest.NewRecorder()
	srv.handleAPIEvents(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var items []apiEventItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal events: %v", err)
	}
	var got *apiEventItem
	for i := range items {
		if items[i].Title == orig {
			got = &items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("未能在响应中找到该事件,共 %d 条", len(items))
	}

	if got.SourceCount != 3 {
		t.Errorf("机构数 source_count 应为 3, got %d", got.SourceCount)
	}
	if got.IndependentCount != 2 {
		t.Errorf("独立来源数 independent_count 应为 2(3 家 - 1 家转载), got %d", got.IndependentCount)
	}
	reprintOf := map[string]bool{}
	for _, cs := range got.ClusterSources {
		reprintOf[cs.Source] = cs.Reprint
	}
	if !reprintOf[srcNames[1]] {
		t.Errorf("近逐字转载的那家(%s)应标 reprint=true: %+v", srcNames[1], got.ClusterSources)
	}
	if reprintOf[srcNames[0]] || reprintOf[srcNames[2]] {
		t.Errorf("原创与独立改写**不得**标转载(误判只可能出在这里): %+v", got.ClusterSources)
	}
}
