package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"piks/internal/model"
	"piks/internal/store"
)

// 事件管线 P-2(issue #83)门控守卫 **反证**测试。临时库自建 + migrate,不污染开发/生产数据。
// 需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
//
// 为什么单列一个文件:迁移 `0021` 的 `origin_kind` 过滤在今日库中是 **no-op**(无 realtime 行),
// 故它**没有天然失败信号** —— 一个被误删的 `AND origin_kind='pipeline'` 不会让任何既有测试变红,
// 只会让 P-5 实时层落地后**静默**把实时行抽进 events / 计入对账。本文件把该契约钉住。
//
// 🔴 **写完必须做一次「先验注入」反证**(纪律记于 memory `guard-falsification-must-verify-injection`):
// 把 `internal/store/raw_documents.go` 与 `internal/store/reconcile.go` 里的
// `AND origin_kind='pipeline'` **真删掉、回读确认落盘**,确认本测试**变红**,然后改回。
// 只看「测试通过」是不够的 —— 测试本身选错断言时也会通过(两层失败叠加)。
//
// 覆盖:
//  1. worker 取数(ListRawPendingStatus)两分支(含/含 failed)—— realtime 行结构上无法被抽进 events;
//  2. reconcile 三查(ReconStaleRaw / ReconFailedRaw / ReconProcessedNoEvent)—— realtime 行不进对账。
//     ⚠️ 实时行**必然**命中这三个检查(永不被 worker 处理 ⇒ 滞留;失败/无事件同理),
//     故这是「实时层不污染正式管线健康口径」的唯一把关点。
func TestP2OriginKindGateGuardsRealtimeRows(t *testing.T) {
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
	tmpDB := fmt.Sprintf("piks_p2guard_%d", os.Getpid())
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

	src := &model.Source{Name: "P2门控机构", SourceType: "news"}
	if err := s.CreateSource(ctx, src); err != nil {
		t.Fatalf("create source: %v", err)
	}

	// insert 一条 raw,返回 id。kind 为 "" 时走 defaultStr → 'pipeline'(即不显式设列)。
	// ⚠️ title 必须非空:`ReconProcessedNoEvent` 把 `r.title` 扫进非指针 `Detail string`,
	// 无标题行会让对账扫描报错(既有代码的 NULL 容忍缺口,与本期无关,此处回避)。
	insert := func(hash, kind, status string) string {
		title := "P2门控-" + hash
		doc := &model.RawDocument{
			SourceID: src.ID, Title: &title, Content: "P2门控-正文-" + hash, ContentHash: hash,
			Status: status, OriginKind: kind,
		}
		if _, err := s.InsertRawDocument(ctx, doc); err != nil {
			t.Fatalf("insert raw %s: %v", hash, err)
		}
		var id string
		if err := pool.QueryRow(ctx, `SELECT id FROM raw_documents WHERE content_hash=$1`, hash).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	// 三组对照:每组一个 pipeline + 一个 realtime,状态覆盖 worker/reconcile 的全部分支。
	// content_hash 必须各不相同 —— 迁移 0016 的去重键**不含** origin_kind(有意设计),
	// 同 (source_id, content_hash) 会撞唯一索引,落不进第二条。
	rawPipe := insert("guard-raw-pipe", "", "raw")
	rawReal := insert("guard-raw-real", "realtime", "raw")

	// ⚠️ InsertRawDocument 不带 status 列以外的写入,MarkRawFailed 才有 error;此处直接 UPDATE。
	failPipe := insert("guard-fail-pipe", "", "raw")
	failReal := insert("guard-fail-real", "realtime", "raw")
	failIDs := []string{failPipe, failReal}
	for _, id := range failIDs {
		if _, err := pool.Exec(ctx,
			`UPDATE raw_documents SET status='failed', error='guard-test' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}

	// 已处理但**不建**事件 ⇒ 命中 ReconProcessedNoEvent。
	procPipe := insert("guard-proc-pipe", "", "raw")
	procReal := insert("guard-proc-real", "realtime", "raw")
	procIDs := []string{procPipe, procReal}
	for _, id := range procIDs {
		if _, err := pool.Exec(ctx, `UPDATE raw_documents SET status='processed' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}

	// 滞留:retrieved_at 推早 8 天(>7 天窗口)⇒ 命中 ReconStaleRaw。
	staleIDs := []string{rawPipe, rawReal}
	for _, id := range staleIDs {
		if _, err := pool.Exec(ctx,
			`UPDATE raw_documents SET retrieved_at = now() - interval '8 days' WHERE id=$1`, id); err != nil {
			t.Fatal(err)
		}
	}

	has := func(list []model.RawDocument, id string) bool {
		for _, d := range list {
			if d.ID == id {
				return true
			}
		}
		return false
	}

	// ── worker 取数:两分支均只见 pipeline ─────────────────────────────────────
	pend, err := s.ListRawPendingStatus(ctx, 500, false)
	if err != nil {
		t.Fatalf("ListRawPendingStatus: %v", err)
	}
	if !has(pend, rawPipe) {
		t.Error("pipeline 的 raw 行应被 worker 取到")
	}
	if has(pend, rawReal) {
		t.Error("🔴 realtime 行被 worker 取到 —— P-5 实时层会被抽进 events(origin_kind 门控失效)")
	}

	pendAll, err := s.ListRawPendingStatus(ctx, 500, true)
	if err != nil {
		t.Fatalf("ListRawPendingStatus(includeFailed): %v", err)
	}
	if !has(pendAll, failPipe) {
		t.Error("pipeline 的 failed 行应在 includeFailed 分支被取到")
	}
	if has(pendAll, failReal) {
		t.Error("🔴 realtime 的 failed 行被 worker 取到(includeFailed 分支漏了 origin_kind 门控)")
	}

	// ── reconcile 三查:realtime 行一律不进对账 ────────────────────────────────
	ct, err := s.ReconStaleRaw(ctx)
	if err != nil {
		t.Fatalf("ReconStaleRaw: %v", err)
	}
	if !reconHas(ct, rawPipe) {
		t.Error("pipeline 滞留行应进对账")
	}
	if reconHas(ct, rawReal) {
		t.Error("🔴 realtime 滞留行进了对账 —— 实时层会污染正式管线健康口径")
	}

	cf, err := s.ReconFailedRaw(ctx)
	if err != nil {
		t.Fatalf("ReconFailedRaw: %v", err)
	}
	if !reconHas(cf, failPipe) {
		t.Error("pipeline 失败行应进对账")
	}
	if reconHas(cf, failReal) {
		t.Error("🔴 realtime 失败行进了对账")
	}

	cp, err := s.ReconProcessedNoEvent(ctx)
	if err != nil {
		t.Fatalf("ReconProcessedNoEvent: %v", err)
	}
	if !reconHas(cp, procPipe) {
		t.Error("pipeline 已处理无事件行应进对账")
	}
	if reconHas(cp, procReal) {
		t.Error("🔴 realtime 已处理无事件行进了对账")
	}

	// 防「测试自己把 realtime 行写坏」的底线:确认 realtime 行真的落库且值为 'realtime',
	// 否则上面的断言可能因「插入失败」而虚假通过。
	var n int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM raw_documents WHERE origin_kind='realtime'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("应落 3 条 realtime 行(反证前提),got %d —— 断言可能因插入失败而虚假通过", n)
	}
}

// reconHas 报告对账结果里是否含某 entity_id。
func reconHas(list []store.ReconIssue, id string) bool {
	for _, r := range list {
		if r.EntityID == id {
			return true
		}
	}
	return false
}
