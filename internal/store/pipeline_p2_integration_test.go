package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// 事件管线 P-2(issue #83)集成测试。临时库自建 + migrate,不污染开发/生产数据。
// 需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
//
// 覆盖:
//  1. origin_kind 门控 —— worker(ListRawPendingStatus)与 reconcile 的 raw 层查询只认 'pipeline';
//     实时层(P-5)行结构上无法被抽进 events / 不进对账。
//  2. 迁移 0021 的 canonical_event_id 回填 —— 取最早非 merged 成员(与 canonicalIndex 同比较器);
//     全员 merged 的簇保持 NULL。
//  3. human_verdict 在幂等重跑(MergedClusters / SetEventCluster)下**不被冲掉**(P4 #7)。
//  4. 新列在 eventCols/rawDocCols 里 round-trip(漏加列会让所有事件 API 500)。

// replaceDBNameP2 把 DSN 的库名段换成 db(形如 .../piks?sslmode=... → .../piks_p2_123?...)。
func replaceDBNameP2(dsn, db string) string {
	scheme := strings.Index(dsn, "://")
	at := strings.LastIndex(dsn, "@")
	if scheme < 0 || at < 0 {
		return dsn
	}
	slash := strings.Index(dsn[at:], "/")
	if slash < 0 {
		return dsn
	}
	slash += at
	rest := dsn[slash+1:]
	q := strings.Index(rest, "?")
	if q < 0 {
		return dsn[:slash+1] + db
	}
	return dsn[:slash+1] + db + rest[q:]
}

func TestP2OriginKindGate(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p2_%d", os.Getpid())
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

	src := &model.Source{Name: "P2GATE机构", SourceType: "news"}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// 一条正式管线(raw) + 一条实时层(raw/realtime) + 一条正式 failed。
	mk := func(hash, kind, status string) string {
		doc := &model.RawDocument{SourceID: src.ID, Content: "P2-" + hash, ContentHash: hash,
			Status: status, OriginKind: kind}
		ok, err := s.InsertRawDocument(ctx, doc)
		if err != nil || !ok {
			t.Fatalf("insert raw %s: ok=%v err=%v", hash, ok, err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM raw_documents WHERE source_id=$1 AND content_hash=$2`, src.ID, hash).Scan(&id); err != nil {
			t.Fatalf("lookup raw: %v", err)
		}
		return id
	}
	pipelineID := mk("p2-pipe", "", "raw") // 空 origin_kind → default 'pipeline'
	realtimeID := mk("p2-real", "realtime", "raw")
	failedID := mk("p2-fail", "pipeline", "failed")

	var kind string
	if err := pool.QueryRow(ctx, `SELECT origin_kind FROM raw_documents WHERE id=$1`, pipelineID).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "pipeline" {
		t.Errorf("空 OriginKind 应落 'pipeline',got %q", kind)
	}

	// rawDocCols round-trip:GetRawDocumentByID 用 RowToStructByName 扫**全列**,
	// 漏掉 origin_kind/canonical_id 会直接报「cannot find field」—— 这条守住列清单。
	gotDoc, err := s.GetRawDocumentByID(ctx, realtimeID)
	if err != nil {
		t.Fatalf("GetRawDocumentByID(rawDocCols round-trip): %v", err)
	}
	if gotDoc.OriginKind != "realtime" {
		t.Errorf("round-trip origin_kind 应为 realtime,got %q", gotDoc.OriginKind)
	}
	if gotDoc.CanonicalID != nil {
		t.Errorf("canonical_id 本版应恒为 NULL(P-8 才填),got %v", *gotDoc.CanonicalID)
	}

	// worker 门控:只取 pipeline 的 'raw' 行,realtime 行不可见。
	pending, err := s.ListRawPendingStatus(ctx, 1000, false)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, d := range pending {
		ids[d.ID] = true
	}
	if !ids[pipelineID] {
		t.Error("正式管线 raw 行必须进 worker 候选")
	}
	if ids[realtimeID] {
		t.Error("🔴 实时层(raw)行**不得**进 worker 候选(会污染 events 表)")
	}

	// includeFailed 也不得带出 realtime(failed)行。
	pendingF, err := s.ListRawPendingStatus(ctx, 1000, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range pendingF {
		if d.ID == realtimeID {
			t.Error("🔴 includeFailed 也不得带出实时层行")
		}
	}

	// reconcile 门控:把正式 raw 行后拨 8 天 → 只报它,不报 realtime 行(即便它也很旧)。
	if _, err := pool.Exec(ctx, `UPDATE raw_documents SET retrieved_at = now() - interval '8 days' WHERE id = ANY($1)`, []string{pipelineID, realtimeID}); err != nil {
		t.Fatal(err)
	}
	stale, err := s.ReconStaleRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range stale {
		if is.EntityID == realtimeID {
			t.Error("🔴 实时层行不得进对账「滞留」")
		}
	}
	foundPipe := false
	for _, is := range stale {
		if is.EntityID == pipelineID {
			foundPipe = true
		}
	}
	if !foundPipe {
		t.Error("正式管线滞留行应被对账报出(过滤误伤 pipeline)")
	}
	failedIssues, err := s.ReconFailedRaw(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range failedIssues {
		if is.EntityID == realtimeID {
			t.Error("🔴 实时层 failed 行不得计入正式异常")
		}
	}
	_ = failedID
}

// TestP2CanonicalEventIDBackfill 直接执行迁移 0021 的**回填段**,验证语义。
// 先建到 0020 的库(不含新列)→ 造簇与事件 → 再执行 0021 → 断回填结果。
// 这是唯一能观察「回填对既有行生效」的方式,同时校验 0021 是独立可执行的 SQL。
func TestP2CanonicalEventIDBackfill(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p2bf_%d", os.Getpid())
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

	// 只应用 0001~0020(用临时目录放副本),让 event_clusters 尚无 canonical_event_id。
	// ⚠️ 0021 及之后一律排除:0021 由下方手工执行;0022 亦依赖 0021 建的列(raw_documents.canonical_id),
	// 在此阶段会因「列不存在」而失败(P-4 引入)。本测试只关心 0021 的回填,不需要 0022。
	srcDir := "../../migrations"
	tmpDir := t.TempDir()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		if strings.HasPrefix(n, "0021") || strings.HasPrefix(n, "0022") {
			continue // 0021 留到最后单独执行;0022 依赖 0021 的列,本测试不涉及
		}
		b, err := os.ReadFile(filepath.Join(srcDir, n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmpDir, n), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ApplyMigrations(ctx, pool, tmpDir); err != nil {
		t.Fatalf("migrate to 0020: %v", err)
	}

	// 造数据(此时无 canonical_event_id 列)。
	srcID := ""
	if err := pool.QueryRow(ctx, `INSERT INTO sources(name, source_type, status) VALUES('P2BF机构','news','active') RETURNING id`).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	docID := ""
	if err := pool.QueryRow(ctx, `INSERT INTO raw_documents(source_id,title,content,content_hash,status) VALUES($1,'t','c','p2bf','processed') RETURNING id`, srcID).Scan(&docID); err != nil {
		t.Fatal(err)
	}
	mkEv := func(title string, at time.Time, conf float64, status string) string {
		var id string
		if err := pool.QueryRow(ctx,
			`INSERT INTO events(raw_document_id,title,event_type,confidence,status,source_id,created_at) VALUES($1,$2,'company',$3,$4,$5,$6) RETURNING id`,
			docID, title, conf, status, srcID, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mkCluster := func(title string) string {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO event_clusters(title,status) VALUES($1,'active') RETURNING id`, title).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	t0 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)

	// (a) 最早者胜:早的 conf 低,晚的 conf 高 → 应选早者。
	cA := mkCluster("P2BF-A")
	eA1 := mkEv("A1", t0, 0.5, "extracted")
	eA2 := mkEv("A2", t0.Add(time.Hour), 0.9, "extracted")
	// (b) 同时间取高置信。
	cB := mkCluster("P2BF-B")
	eB1 := mkEv("B1", t0, 0.5, "extracted")
	eB2 := mkEv("B2", t0, 0.9, "extracted")
	// (c) 全员 merged → NULL。
	cC := mkCluster("P2BF-C")
	eC1 := mkEv("C1", t0, 0.5, "merged")
	// (d) 空簇 → NULL。
	cD := mkCluster("P2BF-D")
	for _, p := range []struct{ ev, cl string }{{eA1, cA}, {eA2, cA}, {eB1, cB}, {eB2, cB}, {eC1, cC}} {
		if _, err := pool.Exec(ctx, `UPDATE events SET cluster_id=$1 WHERE id=$2`, p.cl, p.ev); err != nil {
			t.Fatal(err)
		}
	}

	// 执行迁移 0021(读真实文件内容,确保测的是实际 SQL)。
	body, err := os.ReadFile(filepath.Join(srcDir, "0021_event_pipeline_p2.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// 逐语句执行(psql 风格的多语句一次 Exec 在 pgx simple protocol 下可用)。
	if _, err := pool.Exec(ctx, string(body)); err != nil {
		t.Fatalf("exec 0021: %v", err)
	}

	get := func(cid string) *string {
		var v *string
		if err := pool.QueryRow(ctx, `SELECT canonical_event_id FROM event_clusters WHERE id=$1`, cid).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	assertEq := func(cid, want, label string) {
		got := get(cid)
		if want == "" {
			if got != nil {
				t.Errorf("%s: 期望 NULL,got %s", label, *got)
			}
			return
		}
		if got == nil || *got != want {
			t.Errorf("%s: 期望 %s,got %v", label, want, got)
		}
	}
	assertEq(cA, eA1, "A:最早创建者胜(时间为先,压过高置信)")
	assertEq(cB, eB2, "B:同时间取高置信")
	assertEq(cC, "", "C:全员 merged → NULL")
	assertEq(cD, "", "D:空簇 → NULL")

	// 回填幂等:再跑一次,值不变。
	if _, err := pool.Exec(ctx, string(body)); err != nil {
		t.Fatalf("exec 0021 二次: %v", err)
	}
	assertEq(cA, eA1, "A(二次):幂等")
}

// TestP2HumanVerdictSurvivesRecluster 人工标记在幂等重跑下不被冲掉(issue P4 #7)。
func TestP2HumanVerdictSurvivesRecluster(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p2hv_%d", os.Getpid())
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

	src := &model.Source{Name: "P2HV机构", SourceType: "news"}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	title := "P2HV"
	doc := &model.RawDocument{SourceID: src.ID, Content: "P2HV", ContentHash: "p2hv"}
	if ok, err := s.InsertRawDocument(ctx, doc); err != nil || !ok {
		t.Fatalf("insert raw: %v", err)
	}
	var docID string
	_ = pool.QueryRow(ctx, `SELECT id FROM raw_documents WHERE source_id=$1 AND content_hash='p2hv'`, src.ID).Scan(&docID)
	evID, err := s.CreateEvent(ctx, &model.Event{RawDocumentID: &docID, Title: title, EventType: "company", SourceID: &src.ID, Status: "extracted"})
	if err != nil {
		t.Fatal(err)
	}
	// 人工标记。
	verdict := "confirmed"
	if _, err := pool.Exec(ctx, `UPDATE events SET human_verdict=$1 WHERE id=$2`, verdict, evID); err != nil {
		t.Fatal(err)
	}
	// 幂等重跑:建簇 → 并入(SetEventCluster 改 status='merged')→ MergeClusters 整簇搬运。
	cid, err := s.CreateEventCluster(ctx, &model.EventCluster{Title: title})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEventCluster(ctx, evID, cid, "merged"); err != nil {
		t.Fatal(err)
	}
	cid2, err := s.CreateEventCluster(ctx, &model.EventCluster{Title: title + "-survivor"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MergeClusters(ctx, cid, cid2); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetEventByID(ctx, evID)
	if err != nil {
		t.Fatal(err)
	}
	if got.HumanVerdict == nil || *got.HumanVerdict != verdict {
		t.Errorf("🔴 引擎重跑冲掉了人工标记:期望 %q, got %v", verdict, got.HumanVerdict)
	}
	if got.Status != "merged" {
		t.Errorf("status 应被重跑改为 merged(确认重跑确实发生), got %q", got.Status)
	}
}
