package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"piks/internal/store"
)

// TestDashboardEmptyDBNeverNull 回归 issue #59 的**真实 handler 路径**。
//
// 单元测试(`TestEmptyMarketSnapshotNoNullArrays`)只钉住构造好的结构体;此处打真库,
// 断言空库下 `/api/v1/dashboard` 的**真实 JSON 响应**里没有任何数组字段是 `null`。
//
// 为何必须打真库:缺陷不在某个字段的构造,而在「`len(snaps)==0` 这条分支从未被
// 走到」—— 生产此前总有一两天快照,清库才把它逼出来。只有真库才能走到那条分支。
//
// 在**临时库**里跑(而不是开发库):不污染开发数据,且天然就是「全新安装」形态,
// 正是 issue 的复现条件。需 PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关。
func TestDashboardEmptyDBNeverNull(t *testing.T) {
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

	tmpDB := fmt.Sprintf("piks_web_empty_%d", os.Getpid())
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB); err != nil {
		t.Fatalf("drop tmp db: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+tmpDB); err != nil {
		t.Fatalf("create tmp db: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+tmpDB) })

	tmpDSN := replaceDBName(dsn, tmpDB)
	pool, err := store.Open(ctx, tmpDSN)
	if err != nil {
		t.Fatalf("open tmp pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := store.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("migrate tmp db: %v", err)
	}

	// 确认这是**空**库(无 market_snapshots)—— 否则测的就不是空态分支了。
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM market_snapshots`).Scan(&n); err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if n != 0 {
		t.Fatalf("临时库应为空,实有 %d 条快照", n)
	}

	srv := &Server{store: store.New(pool)}
	rec := httptest.NewRecorder()
	srv.handleAPIDashboard(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	// 顶层四个数组:前端 types.ts 声明为非可选,直接 .map()/.slice()。
	for _, k := range []string{"stats", "snap_history", "top_events", "task_runs"} {
		if string(m[k]) == "null" {
			t.Errorf("dashboard.%s = null(issue #59 白屏根因);应为 []", k)
		}
	}
	// market 块内的三个数组(首页 WatchOverview 直接 m.indices.map)。
	var mk map[string]json.RawMessage
	if err := json.Unmarshal(m["market"], &mk); err != nil {
		t.Fatalf("market 块解析失败: %v", err)
	}
	for _, k := range []string{"indices", "ladder", "industry_dist"} {
		if string(mk[k]) == "null" {
			t.Errorf("market.%s = null(issue #59 白屏根因);应为 []", k)
		}
	}
	// market 标量字段必须齐全(前端无条件读)。
	for _, k := range []string{"trade_date", "emotion_state", "limit_up", "emotion_score"} {
		if _, ok := mk[k]; !ok {
			t.Errorf("market.%s 字段缺失", k)
		}
	}
}

// replaceDBName 把 DSN 的库名换成另一个(测试用临时库)。
func replaceDBName(dsn, db string) string {
	// 形如 postgres://user:pass@host:port/piks?sslmode=disable —— 换掉路径段。
	i := 0
	for j := 0; j < len(dsn); j++ {
		if dsn[j] == '/' && j+1 < len(dsn) {
			// 找 host 之后的第一个 '/'(@ 之后的第一个)
			if k := indexByteFrom(dsn, '@', 0); k >= 0 && j > k {
				i = j
				break
			}
		}
	}
	if i == 0 {
		return dsn
	}
	rest := dsn[i+1:]
	if q := indexByteFrom(rest, '?', 0); q >= 0 {
		return dsn[:i+1] + db + rest[q:]
	}
	return dsn[:i+1] + db
}

func indexByteFrom(s string, b byte, from int) int {
	for i := from; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
