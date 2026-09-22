package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"piks/internal/store"
)

// TestListEventsForAPIIncludesMerged 回归 issue #80:被聚类并入代表(`status='merged'`)
// 的事件此前被 `ListEventsForAPI` 排除,使前端「已被合并」筛选恒空。
//
// merged 行**仍是知识库里的真实事件**(只是不再单独发卡片),而本查询是前端事件列表的
// 唯一数据源,故必须放行。发布类查询 `ListEventsForPublish*` 仍排除 merged —— 那是
// 「不重复发卡片」的语义,与本查询职责不同。
//
// 用自建隔离数据(带时间戳后缀),不依赖也不污染真实库。
func TestListEventsForAPIIncludesMerged(t *testing.T) {
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
	srcName := "t80-机构-" + suffix
	var srcID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sources(name, source_type, status) VALUES ($1,'news','active') RETURNING id`,
		srcName).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	title := "T80TEST-" + suffix
	insert := func(status string) string {
		var docID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO raw_documents(source_id, title, content, content_hash, status)
			 VALUES ($1,$2,'x',$3,'processed') RETURNING id`,
			srcID, title, "t80-"+status+"-"+suffix).Scan(&docID); err != nil {
			t.Fatal(err)
		}
		var evID string
		if err := pool.QueryRow(ctx,
			`INSERT INTO events(raw_document_id,title,event_type,summary,facts,affected,
			                     confidence,status,pipeline_version,source_id)
			 VALUES ($1,$2,'policy','s','[]','[]',0.9,$3,'test',$4) RETURNING id`,
			docID, title, status, srcID).Scan(&evID); err != nil {
			t.Fatal(err)
		}
		return evID
	}

	extractedID := insert("extracted")
	mergedID := insert("merged")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE id = ANY($1)`,
			[]string{extractedID, mergedID})
		_, _ = pool.Exec(ctx, `DELETE FROM raw_documents WHERE source_id=$1`, srcID)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id=$1`, srcID)
	})

	evs, err := s.ListEventsForAPI(ctx, "")
	if err != nil {
		t.Fatalf("ListEventsForAPI: %v", err)
	}
	byID := map[string]string{}
	for _, e := range evs {
		byID[e.ID] = e.Status
	}
	if st, ok := byID[mergedID]; !ok {
		t.Errorf("merged 事件 %s 未出现在 ListEventsForAPI —— 「已被合并」筛选会恒空(issue #80)", mergedID)
	} else if st != "merged" {
		t.Errorf("merged 事件下发 status = %q, want merged", st)
	}
	if _, ok := byID[extractedID]; !ok {
		t.Errorf("extracted 事件 %s 未出现在 ListEventsForAPI —— 查询过度收窄", extractedID)
	}

	// 反向:看板「高置信事件」候选**必须排除 merged**(issue #80)—— 那里没有知识态
	// 筛选,merged 会作为重复条目混进 Top N。两条查询共用 eventsListQuery,此处钉住开关。
	tops, err := s.ListTopEventsForDashboard(ctx)
	if err != nil {
		t.Fatalf("ListTopEventsForDashboard: %v", err)
	}
	for _, e := range tops {
		if e.ID == mergedID {
			t.Errorf("merged 事件 %s 混进了 ListTopEventsForDashboard —— 看板 Top N 会出现重复条目", mergedID)
		}
	}
}
