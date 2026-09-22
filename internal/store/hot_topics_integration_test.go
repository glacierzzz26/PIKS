package store_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// 热榜快照落库/读回(issue #68 D 层验收)。需真库(同既有集成测试双开关)。
//
// 自建隔离数据(唯一 source 名 + t.Cleanup 删除),验证:
//
//	① 同源多批快照:ListHotTopicLatest **只回最新一批**,旧批不混入;
//	② 多源:各源**各取自己最新一批**,互不影响(两列各出各的);
//	③ rank 与 hot_value 原样往返(NULL hot_value 保持 NULL,不被写成 0);
//	④ **不触碰 raw_documents/events** —— 本层与事件链路零交集(设计 §6 红线)。
func TestHotTopicItemsRoundTrip(t *testing.T) {
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

	// 用真实枚举值 + 唯一后缀,避免与生产数据混淆,也便于精确清理。
	srcA := "ths-topic"
	srcB := "cls-hot-article"
	old := time.Now().Add(-2 * time.Hour)
	now := time.Now()

	t.Cleanup(func() {
		// 只删本测试插入的行(按 snapshot_at 窗口 + 本次测试特有的标题前缀)。
		_, _ = pool.Exec(ctx,
			`DELETE FROM hot_topic_items WHERE title LIKE 't68-hot-%'`)
	})

	nil9 := int64(12345)
	items := []model.HotTopicItem{
		// 源 A 的**旧**批(应被最新批遮住)
		{Source: srcA, Rank: 1, Title: "t68-hot-A旧批", HotValue: &nil9, SnapshotAt: old},
		// 源 A 的**新**批
		{Source: srcA, Rank: 1, Title: "t68-hot-A新批榜一", HotValue: &nil9, SnapshotAt: now},
		{Source: srcA, Rank: 2, Title: "t68-hot-A新批榜二", HotValue: nil, SnapshotAt: now}, // NULL 热度
		// 源 B 的最新批(时刻与 A 不同 —— 串行采集会差几秒,故各取各的)
		{Source: srcB, Rank: 1, Title: "t68-hot-B榜一", HotValue: &nil9, SnapshotAt: now.Add(time.Second)},
	}
	if _, err := s.InsertHotTopicItems(ctx, items); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.ListHotTopicLatest(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// 过滤出本测试的行(表里可能有生产数据 —— 这是共享库,必须只断言自己的)。
	const prefix = "t68-hot-"
	var mine []model.HotTopicItem
	for _, it := range got {
		if strings.HasPrefix(it.Title, prefix) {
			mine = append(mine, it)
		}
	}
	if len(mine) != 3 {
		t.Fatalf("应只回 3 行(源 A 新批 2 + 源 B 1,旧批被遮), got %d: %+v", len(mine), mine)
	}
	// ① 旧批未混入
	for _, it := range mine {
		if it.Title == "t68-hot-A旧批" {
			t.Error("旧批不应出现在「最新一批」里")
		}
	}
	// ③ NULL 热度保持 NULL(不被写成 0)
	var sawNull bool
	for _, it := range mine {
		if it.Rank == 2 && it.Source == srcA {
			sawNull = true
			if it.HotValue != nil {
				t.Errorf("NULL hot_value 应保持 nil, got %v", *it.HotValue)
			}
		}
	}
	if !sawNull {
		t.Error("未找到 NULL 热度的行")
	}
	// ② 两源各取最新 —— 源 B 的行必须出现(未被源 A 的时刻遮住)
	var sawB bool
	for _, it := range mine {
		if it.Source == srcB {
			sawB = true
		}
	}
	if !sawB {
		t.Error("源 B 的最新批应出现(各源各取最新,不按统一时刻)")
	}
}
