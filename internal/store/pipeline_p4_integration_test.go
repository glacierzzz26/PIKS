package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"piks/internal/cluster"
	"piks/internal/model"
	"piks/internal/store"
)

// 事件管线 P-4(issue #83)集成测试:raw 层转载组回填 + 展示取数(raw 层全集)。
// 临时库自建 + migrate,不污染开发/生产数据。需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
//
// 覆盖:
//  1. ListRawDocumentsForGrouping / SetRawCanonicalIDs —— 回填幂等(重跑零变更)、**只填 NULL**(代表冻结)、
//     ListClusterRawSources **取 raw 层全集**:某机构报了但**没抽成事件**,它仍出现在来源里(issue 硬验收);
//  2. 同机构多 URL 全列、只算一个机构(计一票由调用方按机构去重);
//  3. 无 URL 的源如实带出(NULL,不造链接);
//  4. ListClusterTitles —— 簇标题取回。
//
// 🔴 代表冻结:SetRawCanonicalIDs 只写 `canonical_id IS NULL` 的行 —— 已冻结的行永不重选
// (否则 url 归属会漂移)。反证:手工改写一条已冻结行的 canonical_id 后重跑,它**不被覆盖**。

func TestP4RawCanonicalAndClusterRawSources(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p4_%d", os.Getpid())
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

	// 三家机构:甲/乙/丙。
	mkSrc := func(name string) *model.Source {
		src := &model.Source{Name: name, SourceType: "news"}
		if err := s.CreateSource(ctx, src); err != nil {
			t.Fatalf("create source %s: %v", name, err)
		}
		return src
	}
	srcA, srcB, srcC := mkSrc("P4甲"), mkSrc("P4乙"), mkSrc("P4丙")

	// 同一篇稿(近逐字,仅电头不同 ⇒ P-1 判为同组):甲乙丙各一行。
	bodyWithDateline := "财联社9月21日电，央行今日开展5000亿元中期借贷便利MLF操作，中标利率2.30%，与上期持平。"
	bodyNoDateline := "央行今日开展5000亿元中期借贷便利MLF操作，中标利率2.30%，与上期持平。"

	// raw 行:甲(带 url)、乙(带 url,近逐字转载)、丙(带 url,**不抽成事件**)。
	mkDoc := func(srcID, hash, content, url string, ago time.Duration) (string, *string) {
		u := url
		var up *string
		if url != "" {
			up = &u
		}
		doc := &model.RawDocument{SourceID: srcID, Content: content, ContentHash: hash, Status: "processed", URL: up}
		if _, err := s.InsertRawDocument(ctx, doc); err != nil {
			t.Fatalf("insert raw %s: %v", hash, err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM raw_documents WHERE content_hash=$1`, hash).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE raw_documents SET retrieved_at=$1 WHERE id=$2`,
			time.Now().Add(-ago), id); err != nil {
			t.Fatal(err)
		}
		return id, up
	}
	docA, urlA := mkDoc(srcA.ID, "p4-a", bodyWithDateline, "https://a.example/1", 2*time.Hour)
	docB, urlB := mkDoc(srcB.ID, "p4-b", bodyNoDateline, "https://b.example/1", 1*time.Hour)
	docC, urlC := mkDoc(srcC.ID, "p4-c", bodyNoDateline, "https://c.example/1", 30*time.Minute)

	// ── 1. 分组回填:甲乙丙同组(≥2 机构),代表 = 最早入库的甲。
	docs, err := s.ListRawDocumentsForGrouping(ctx, time.Time{})
	if err != nil {
		t.Fatalf("ListRawDocumentsForGrouping: %v", err)
	}
	entries := make([]cluster.RawDocEntry, len(docs))
	for i, d := range docs {
		rep := ""
		if d.CanonicalID != nil {
			rep = *d.CanonicalID
		}
		entries[i] = cluster.RawDocEntry{ID: d.ID, SourceID: d.SourceID, Content: d.Content,
			RetrievedAt: d.RetrievedAt.Format(time.RFC3339Nano), CanonicalID: rep}
	}
	groups := cluster.RawGroups(entries)
	if len(groups) != 1 || len(groups[0].Members) != 2 {
		t.Fatalf("甲乙丙应成一转载组、写回 2 行, got %+v", groups)
	}
	if groups[0].RepID != docA {
		t.Errorf("代表应为最早入库的甲, got %s", groups[0].RepID)
	}
	// 落库。
	pairs := [][2]string{}
	for _, g := range groups {
		for _, m := range g.Members {
			pairs = append(pairs, [2]string{m, g.RepID})
		}
	}
	n, err := s.SetRawCanonicalIDs(ctx, pairs)
	if err != nil {
		t.Fatalf("SetRawCanonicalIDs: %v", err)
	}
	if n != 2 {
		t.Errorf("首次回填应改 2 行(乙丙), got %d", n)
	}

	// ── 2. 幂等:重跑零变更。
	docs2, _ := s.ListRawDocumentsForGrouping(ctx, time.Time{})
	entries2 := make([]cluster.RawDocEntry, len(docs2))
	for i, d := range docs2 {
		rep := ""
		if d.CanonicalID != nil {
			rep = *d.CanonicalID
		}
		entries2[i] = cluster.RawDocEntry{ID: d.ID, SourceID: d.SourceID, Content: d.Content,
			RetrievedAt: d.RetrievedAt.Format(time.RFC3339Nano), CanonicalID: rep}
	}
	groups2 := cluster.RawGroups(entries2)
	if len(groups2) != 1 || len(groups2[0].Members) != 0 {
		t.Fatalf("回填后重跑应无待写成员(幂等), got %+v", groups2)
	}
	pairs2 := [][2]string{}
	for _, g := range groups2 {
		for _, m := range g.Members {
			pairs2 = append(pairs2, [2]string{m, g.RepID})
		}
	}
	if n, _ := s.SetRawCanonicalIDs(ctx, pairs2); n != 0 {
		t.Errorf("重跑应改 0 行, got %d", n)
	}

	// ── 3. 代表冻结反证:手工把乙的 canonical_id 改指乙自己,重跑**不得**覆盖。
	if _, err := pool.Exec(ctx, `UPDATE raw_documents SET canonical_id=id WHERE id=$1`, docB); err != nil {
		t.Fatal(err)
	}
	// 直接构造一对「写乙→甲」的对,断言被 `AND canonical_id IS NULL` 挡下。
	if n, _ := s.SetRawCanonicalIDs(ctx, [][2]string{{docB, docA}}); n != 0 {
		t.Errorf("已冻结行不得被重写(代表冻结),改动了 %d 行", n)
	}
	var repB *string
	if err := pool.QueryRow(ctx, `SELECT canonical_id FROM raw_documents WHERE id=$1`, docB).Scan(&repB); err != nil {
		t.Fatal(err)
	}
	if repB == nil || *repB != docB {
		t.Errorf("乙 的 canonical_id 应保持为自身(不被覆盖), got %v", repB)
	}
	// 恢复:让乙仍指向甲(与代表一致),便于下方取数断言。
	if _, err := pool.Exec(ctx, `UPDATE raw_documents SET canonical_id=$1 WHERE id=$2`, docA, docB); err != nil {
		t.Fatal(err)
	}

	// ── 4. 取数:簇内事件只有甲(乙未抽成事件)、丙**完全没抽事件**。
	clusterID, err := s.CreateEventCluster(ctx, &model.EventCluster{Title: "P4 簇标题"})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	evID, err := s.CreateEvent(ctx, &model.Event{Title: "P4 事件", EventType: "macro",
		Status: "extracted", RawDocumentID: &docA})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}
	if err := s.SetEventClusterNoTouch(ctx, evID, clusterID); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListClusterRawSources(ctx, []string{clusterID})
	if err != nil {
		t.Fatalf("ListClusterRawSources: %v", err)
	}
	rows := got[clusterID]
	if len(rows) != 3 {
		t.Fatalf("raw 层全集应含 甲(代表,事件)、乙(转载)、丙(未抽成事件)共 3 行, got %+v", rows)
	}
	bySrc := map[string]store.ClusterRawSource{}
	for _, r := range rows {
		bySrc[r.Source] = r
	}
	// 🔴 P8 硬验收:丙 报了这篇稿但没抽成事件 —— 它必须出现在来源里。
	if _, ok := bySrc["P4丙"]; !ok {
		t.Fatalf("🔴 未抽成事件的机构(丙)必须出现在 raw 层来源里(P8 硬验收), got %+v", rows)
	}
	if bySrc["P4丙"].URL == nil || *bySrc["P4丙"].URL != *urlC {
		t.Errorf("丙 的链接投影错误: %+v", bySrc["P4丙"])
	}
	// 代表行(甲)IsRep=true,其余(乙)为转载。
	if !bySrc["P4甲"].IsRep {
		t.Error("代表行(甲)应 IsRep=true")
	}
	if bySrc["P4乙"].IsRep {
		t.Error("非代表行(乙)应 IsRep=false(转载)")
	}
	if bySrc["P4甲"].Canonical != docA || bySrc["P4乙"].Canonical != docA {
		t.Error("同组行应共享代表 id")
	}
	// 三家 url 都在,且与各自机构配对。
	if bySrc["P4甲"].URL == nil || *bySrc["P4甲"].URL != *urlA {
		t.Errorf("甲 url 错: %+v", bySrc["P4甲"].URL)
	}
	if bySrc["P4乙"].URL == nil || *bySrc["P4乙"].URL != *urlB {
		t.Errorf("乙 url 错: %+v", bySrc["P4乙"].URL)
	}

	// ── 5. 无 URL 的源:如实 NULL,不造链接。
	docD, _ := mkDoc(srcB.ID, "p4-d", "另一篇完全不同的稿D，内容与前述无关，用于验证无外链源。", "", 10*time.Minute)
	docE, _ := mkDoc(srcC.ID, "p4-e", "另一篇完全不同的稿D，内容与前述无关，用于验证无外链源。", "", 5*time.Minute)
	ev2, _ := s.CreateEvent(ctx, &model.Event{Title: "P4 事件2", EventType: "macro",
		Status: "extracted", RawDocumentID: &docD})
	_ = s.SetEventClusterNoTouch(ctx, ev2, clusterID)
	// 把 DE 组成一组(回填)。
	if _, err := s.SetRawCanonicalIDs(ctx, [][2]string{{docE, docD}}); err != nil {
		t.Fatal(err)
	}
	_ = docE
	got2, err := s.ListClusterRawSources(ctx, []string{clusterID})
	if err != nil {
		t.Fatalf("ListClusterRawSources(2): %v", err)
	}
	foundNilURL := false
	for _, r := range got2[clusterID] {
		if r.Source == "P4丙" && r.Canonical == docD {
			if r.URL != nil {
				t.Errorf("无外链的 raw 行应为 NULL,不得造链接: %+v", r)
			}
			foundNilURL = true
		}
	}
	if !foundNilURL {
		t.Errorf("未找到无外链的源(丙 的稿D), got %+v", got2[clusterID])
	}

	// ── 6. 簇标题。
	titles, err := s.ListClusterTitles(ctx, []string{clusterID})
	if err != nil {
		t.Fatalf("ListClusterTitles: %v", err)
	}
	if titles[clusterID] != "P4 簇标题" {
		t.Errorf("簇标题应为「P4 簇标题」, got %q", titles[clusterID])
	}

	// ── 7. origin_kind 门控:realtime 行不进来源列表(P-2 分道闸)。
	if _, err := pool.Exec(ctx,
		`UPDATE raw_documents SET origin_kind='realtime' WHERE id=$1`, docC); err != nil {
		t.Fatal(err)
	}
	got3, err := s.ListClusterRawSources(ctx, []string{clusterID})
	if err != nil {
		t.Fatalf("ListClusterRawSources(3): %v", err)
	}
	for _, r := range got3[clusterID] {
		if r.Source == "P4丙" && r.Canonical == docC {
			t.Error("🔴 realtime 行进了簇来源列表(origin_kind 门控失效)")
		}
	}
}
