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

// 事件管线 P-5(issue #83)集成测试:raw 层保留期清理的**安全边界**。
// 临时库自建 + migrate,不污染开发/生产数据。需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
//
// 覆盖(红线:保留期**只清无事件引用、非转载组代表**的行):
//  1. 到期 + 无引用 + 非代表 ⇒ 删;
//  2. 到期但**有事件引用** ⇒ **不删**(事件溯源 + FK 安全);
//  3. 到期但**被别的行指向**(转载组代表)⇒ **不删**(防组内成员 canonical_id 悬空);
//  4. 未到期 ⇒ 不删;
//  5. 幂等:连跑两次,第二次 0 行;dry-run 候选数 == 实删数。
//
// ⚠️ 判据是**单趟**的:只在「删除发生时」保证不删掉被引用的行。若整组(代表 + 成员)
// 同时到期,代表在本趟仍被**存活**的成员护住 → 下趟成员没了、代表才可清。这是**渐进清理**,
// 不是漏删:任何时刻都不留悬空 canonical_id。故用例 3 用一个**未到期**的成员来验「护住」。
func TestP5RawCleanupSafety(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p5_%d", os.Getpid())
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

	src := &model.Source{Name: "P5机构", SourceType: "news"}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	old := time.Now().Add(-40 * 24 * time.Hour) // 40 天前 ⇒ 已到期
	fresh := time.Now().Add(-3 * 24 * time.Hour)

	// mk 落一条 raw 并显式设 retrieved_at,返回 id。
	mk := func(hash string, retrievedAt time.Time) string {
		doc := &model.RawDocument{SourceID: src.ID, Content: "P5-" + hash, ContentHash: hash, Status: "processed"}
		if _, err := s.InsertRawDocument(ctx, doc); err != nil {
			t.Fatalf("insert raw: %v", err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM raw_documents WHERE content_hash=$1`, hash).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE raw_documents SET retrieved_at=$1 WHERE id=$2`, retrievedAt, id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	purgeable := mk("p5-del", old)    // 到期、无引用、非代表 ⇒ 应删
	withEvent := mk("p5-evt", old)    // 到期但有事件引用 ⇒ 不删
	rep := mk("p5-rep", old)          // 到期且被**未到期**成员指向 ⇒ 不删(护住)
	memberOld := mk("p5-mem", old)    // 到期、指向 rep;自身非代表 ⇒ 可删(删的是指针,不是目标)
	memberNew := mk("p5-rep2", fresh) // 未到期、指向 rep ⇒ 使 rep 有存活入边
	notOld := mk("p5-new", fresh)     // 未到期 ⇒ 不删

	// withEvent:建一个事件引用它(状态任意,只验证「有引用」)。
	ev := &model.Event{Title: "P5事件", EventType: "company", Confidence: 0.5, Status: "extracted", RawDocumentID: &withEvent}
	if _, err := s.CreateEvent(ctx, ev); err != nil {
		t.Fatalf("create event: %v", err)
	}
	// 两条行指向 rep ⇒ rep 是「转载组代表」(被指向者)。
	if _, err := pool.Exec(ctx, `UPDATE raw_documents SET canonical_id=$1 WHERE id IN ($2,$3)`, rep, memberOld, memberNew); err != nil {
		t.Fatal(err)
	}

	// dry-run 候选数 = 实删数(共用判据):候选应为 purgeable + memberOld(2 行)。
	cands, err := s.ListRawCleanupCandidates(ctx, 28*24*time.Hour)
	if err != nil {
		t.Fatalf("candidates: %v", err)
	}
	got := map[string]bool{}
	for _, c := range cands {
		got[c.ID] = true
	}
	if !got[purgeable] || !got[memberOld] {
		t.Errorf("候选应含到期无引用非代表行:purgeable=%v memberOld=%v", got[purgeable], got[memberOld])
	}
	if got[withEvent] {
		t.Error("有事件引用的行不应进候选")
	}
	if got[rep] {
		t.Error("被未到期成员指向的代表不应进候选(否则 canonical_id 悬空)")
	}
	if got[notOld] || got[memberNew] {
		t.Error("未到期的行不应进候选")
	}

	// 实删。
	n, err := s.PurgeRawDocuments(ctx, 28*24*time.Hour)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != int64(len(cands)) {
		t.Errorf("dry-run 候选数(%d)应等于实删数(%d)", len(cands), n)
	}

	// 断言存活:purgeable/memberOld 已删;其余仍在。
	survive := func(id string) bool {
		var c int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM raw_documents WHERE id=$1`, id).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c == 1
	}
	if survive(purgeable) || survive(memberOld) {
		t.Error("到期无引用非代表行应被删除")
	}
	if !survive(withEvent) {
		t.Error("有事件引用的行**不应**被删除(事件溯源 + FK)")
	}
	if !survive(rep) {
		t.Error("被存活成员指向的代表**不应**被删除")
	}
	if !survive(notOld) || !survive(memberNew) {
		t.Error("未到期的行不应被删除")
	}

	// 幂等:重跑 0 行。
	again, err := s.PurgeRawDocuments(ctx, 28*24*time.Hour)
	if err != nil {
		t.Fatalf("purge again: %v", err)
	}
	if again != 0 {
		t.Errorf("重跑应 0 行(幂等),got %d", again)
	}
}

// TestP5FlashWindow 覆盖实时层读路径 `ListRawDocumentsWithSource(since)`(issue #83 P-5):
//   - **零值 since = 不限**(缺省全量,与旧行为逐字一致)—— 这条是回归护栏:`since` 加入后
//     曾因 SQL 里参数占位错写(`$2` 而实参只有 1 个)导致缺省调用直接 SQLSTATE 42P18 500;
//   - 非零 since 只回**原始到达时刻**在窗内的行,且锚 `COALESCE(published_at,retrieved_at,created_at)`。
func TestP5FlashWindow(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p5w_%d", os.Getpid())
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
	src := &model.Source{Name: "P5窗机构", SourceType: "news"}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}
	mk := func(hash string, arrived time.Time) string {
		doc := &model.RawDocument{SourceID: src.ID, Content: "P5W-" + hash, ContentHash: hash, Status: "raw"}
		if _, err := s.InsertRawDocument(ctx, doc); err != nil {
			t.Fatalf("insert raw: %v", err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM raw_documents WHERE content_hash=$1`, hash).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE raw_documents SET retrieved_at=$1 WHERE id=$2`, arrived, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	recent := mk("p5w-recent", time.Now().Add(-1*time.Hour)) // 近 3h 内
	stale := mk("p5w-stale", time.Now().Add(-50*time.Hour))  // 窗外的老行

	// 零值 since:不限 ⇒ 两行都在(回归护栏:此处曾 500)。
	all, err := s.ListRawDocumentsWithSource(ctx, "", time.Time{})
	if err != nil {
		t.Fatalf("默认(零值 since)查询失败(回归:参数占位/缺省路径): %v", err)
	}
	ids := map[string]bool{}
	for _, f := range all {
		ids[f.ID] = true
	}
	if !ids[recent] || !ids[stale] {
		t.Errorf("零值 since 应回全量(不限):recent=%v stale=%v", ids[recent], ids[stale])
	}

	// 滚动近 3h:只回 recent。
	near, err := s.ListRawDocumentsWithSource(ctx, "", time.Now().Add(-3*time.Hour))
	if err != nil {
		t.Fatalf("近 3h 查询: %v", err)
	}
	nearIDs := map[string]bool{}
	for _, f := range near {
		nearIDs[f.ID] = true
	}
	if !nearIDs[recent] {
		t.Error("近 3h 窗应含 1 小时前的行")
	}
	if nearIDs[stale] {
		t.Error("近 3h 窗不应含 50 小时前的行")
	}
}
