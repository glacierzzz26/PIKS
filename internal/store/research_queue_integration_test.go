package store_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"piks/internal/store"
)

// 深研队列原语(2026-09-20 容器拆分 P2):认证领与孤儿回收。
// 用测试专用 code(999990 段)与时间戳 run_id,保证可重复运行且不碰真实数据。

// setupQueueTest 打开池 + 注册清理。返回 (s, ctx, code)。
func setupQueueTest(t *testing.T, code string) (*store.Store, context.Context) {
	t.Helper()
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
	// LIFO:先注册关池(最后跑),再注册删行(先跑)—— 同 research_runs_integration_test 的注意事项。
	t.Cleanup(func() { pool.Close() })
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE code=$1`, code) })
	_, _ = pool.Exec(ctx, `DELETE FROM research_runs WHERE code=$1`, code)
	return store.New(pool), ctx
}

// seedPending 建一条 pending run,返回 run_id。
func seedPending(t *testing.T, s *store.Store, ctx context.Context, code, runID string, created time.Time) {
	t.Helper()
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: code, Symbol: "sz" + code,
		Profile: "short-term", AsOf: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("seed %s: %v", runID, err)
	}
	// created_at 显式回填(默认 now(),同批种子的 FIFO 顺序会并列/抖动)。
	if _, err := s.Pool.Exec(ctx,
		`UPDATE research_runs SET created_at=$2, updated_at=$2 WHERE run_id=$1`, runID, created); err != nil {
		t.Fatalf("backdate %s: %v", runID, err)
	}
}

// TestClaimPendingResearchRun 认领:单次取一条、置 gathering、FIFO、并发不相交。
func TestClaimPendingResearchRun(t *testing.T) {
	const code = "999990"
	s, ctx := setupQueueTest(t, code)

	base := time.Now().Add(-time.Hour)
	ids := []string{"test_q_" + code + "_a", "test_q_" + code + "_b", "test_q_" + code + "_c"}
	for i, id := range ids {
		seedPending(t, s, ctx, code, id, base.Add(time.Duration(i)*time.Minute))
	}

	// 1. 空队列不该被误判:先把三条都领走,第三次起应返回 nil。
	got := map[string]string{} // run_id → 认领时的 status
	for i := 0; i < len(ids); i++ {
		run, err := s.ClaimPendingResearchRun(ctx)
		if err != nil {
			t.Fatalf("claim #%d: %v", i, err)
		}
		if run == nil {
			t.Fatalf("claim #%d 应领到行,却得空", i)
		}
		if run.Status != "gathering" {
			t.Fatalf("认领后 status 应为 gathering,实际 %s", run.Status)
		}
		got[run.RunID] = run.Status
	}
	if _, err := s.ClaimPendingResearchRun(ctx); err != nil {
		t.Fatalf("claim empty: %v", err)
	}

	// 2. FIFO:created_at 最早的三条依次被领走(与种子顺序一致)。
	if _, ok := got[ids[0]]; !ok {
		t.Fatalf("最早的行未被领到,实际领到 %v", got)
	}
	// 3. 三条都只被领一次(集合大小即证)。
	if len(got) != len(ids) {
		t.Fatalf("应领到 %d 条互不相同的行,实际 %d", len(ids), len(got))
	}
	t.Logf("ClaimPendingResearchRun ok: FIFO 领走 %d 条,空队列返回 nil", len(got))
}

// TestClaimPendingResearchRunConcurrent 并发认领:两个 goroutine 不拿到同一行。
// 这是 SKIP LOCKED 的核心保证 —— 缺了它两个 worker 会同时跑同一只股票。
func TestClaimPendingResearchRunConcurrent(t *testing.T) {
	const code = "999991"
	s, ctx := setupQueueTest(t, code)

	const n = 8
	base := time.Now().Add(-time.Hour)
	want := map[string]bool{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("test_qc_%s_%02d", code, i)
		seedPending(t, s, ctx, code, id, base.Add(time.Duration(i)*time.Second))
		want[id] = true
	}

	var mu sync.Mutex
	seen := map[string]int{} // run_id → 被认领次数
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				run, err := s.ClaimPendingResearchRun(ctx)
				if err != nil {
					t.Errorf("并发认领出错: %v", err)
					return
				}
				if run == nil {
					return // 队空,本协程收工
				}
				mu.Lock()
				seen[run.RunID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Fatalf("应领到 %d 条,实际 %d 条(漏领)", n, len(seen))
	}
	for id, c := range seen {
		if c != 1 {
			t.Fatalf("run %s 被认领 %d 次 —— SKIP LOCKED 未生效(会重复跑)", id, c)
		}
	}
	t.Logf("并发认领 ok: 4 协程领走 %d 条,每条恰好 1 次", n)
}

// TestReapStuckResearchRuns 孤儿回收:超期进行中 → failed;新鲜/终态 → 不动。
func TestReapStuckResearchRuns(t *testing.T) {
	const code = "999992"
	s, ctx := setupQueueTest(t, code)

	stale := "test_reap_" + code + "_stale" // 超期 gathering → 该被收
	fresh := "test_reap_" + code + "_fresh" // 新鲜 gathering → 不该动
	done := "test_reap_" + code + "_done"   // 终态(即使 updated_at 很旧)→ 不该动

	seedPending(t, s, ctx, code, stale, time.Now().Add(-time.Hour))
	seedPending(t, s, ctx, code, fresh, time.Now().Add(-time.Hour))
	seedPending(t, s, ctx, code, done, time.Now().Add(-time.Hour))

	// 三条都推进到 gathering(模拟 worker 认领后在跑)。
	for _, id := range []string{stale, fresh, done} {
		if err := s.UpdateResearchRunStatus(ctx, id, "gathering", ""); err != nil {
			t.Fatalf("推进 %s: %v", id, err)
		}
	}
	// 收尾其中一条为 done(终态)。
	if err := s.FinishResearchRun(ctx, done, "done", "", nil); err != nil {
		t.Fatalf("finish %s: %v", done, err)
	}
	// 心跳回填:stale/done 的 updated_at 拉到 20 分钟前(超过 grace),fresh 保持现在。
	old := time.Now().Add(-20 * time.Minute)
	if _, err := s.Pool.Exec(ctx,
		`UPDATE research_runs SET updated_at=$2 WHERE run_id=ANY($1)`, []string{stale, done}, old); err != nil {
		t.Fatalf("backdate heartbeat: %v", err)
	}

	grace := 5 * time.Minute
	reaped, err := s.ReapStuckResearchRuns(ctx, grace)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	// 只应收回 stale 一条(done 是终态,不在 ActiveResearchStatuses 内)。
	if len(reaped) != 1 || reaped[0] != stale {
		t.Fatalf("应只收回 stale(%s),实际 %v", stale, reaped)
	}
	// 校验落库结果:fresh 仍在跑,done 仍是 done。
	rFresh, _ := s.GetResearchRun(ctx, fresh)
	rDone, _ := s.GetResearchRun(ctx, done)
	rStale, _ := s.GetResearchRun(ctx, stale)
	if rFresh.Status != "gathering" {
		t.Fatalf("新鲜 run 不该被收,实际 %s", rFresh.Status)
	}
	if rDone.Status != "done" {
		t.Fatalf("终态 run 不该被改,实际 %s", rDone.Status)
	}
	if rStale.Status != "failed" || rStale.Error == nil {
		t.Fatalf("孤儿 run 应收为 failed 且留错因,实际 status=%s error=%v", rStale.Status, rStale.Error)
	}
	t.Logf("ReapStuckResearchRuns ok: 收 %v;fresh=%s done=%s", reaped, rFresh.Status, rDone.Status)
}

// TestResearchQueueRoundTrip quick/days 随行落库并随认领读回(拆镜像后执行方还原运行参数的唯一途径)。
func TestResearchQueueRoundTrip(t *testing.T) {
	const code = "999993"
	s, ctx := setupQueueTest(t, code)

	runID := "test_rt_" + code
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: code, Symbol: "sz" + code, Profile: "prebuy",
		AsOf:  time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
		Quick: true, Days: 60,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	claimed, err := s.ClaimPendingResearchRun(ctx)
	if err != nil || claimed == nil {
		t.Fatalf("claim: run=%v err=%v", claimed, err)
	}
	if claimed.RunID != runID {
		t.Fatalf("认领到 %s,期望 %s", claimed.RunID, runID)
	}
	if !claimed.Quick || claimed.Days != 60 {
		t.Fatalf("quick/days 未随认领还原: quick=%v days=%d", claimed.Quick, claimed.Days)
	}
	t.Logf("队列参数回环 ok: quick=%v days=%d profile=%s", claimed.Quick, claimed.Days, claimed.Profile)
}

// TestCreateResearchRunQueueDefaults 默认值:不显式给 quick/days 时应落 false/0(旧调用方零回归)。
func TestCreateResearchRunQueueDefaults(t *testing.T) {
	const code = "999994"
	s, ctx := setupQueueTest(t, code)

	runID := "test_def_" + code
	if _, err := s.CreateResearchRun(ctx, &store.ResearchRun{
		RunID: runID, Code: code, Symbol: "sz" + code, Profile: "complete-stock",
		AsOf: time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.GetResearchRun(ctx, runID)
	if err != nil || got == nil {
		t.Fatalf("get: %v", err)
	}
	if got.Quick || got.Days != 0 {
		t.Fatalf("默认应为 quick=false days=0,实际 quick=%v days=%d", got.Quick, got.Days)
	}
	t.Logf("默认值 ok: quick=%v days=%d", got.Quick, got.Days)
}
