package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"piks/internal/announce"
	"piks/internal/model"
	"piks/internal/store"
)

// 公告分级落库/读回(issue #68 A 层验收)。
// 需真库(同既有集成测试:PIKS_TEST_INTEGRATION + PIKS_DATABASE_URL 双开关)。
//
// 自建隔离数据(唯一名 + t.Cleanup 删除),验证:
//
//	① grade 随 InsertRawDocument 落库、经 ListAnnouncementsWithSource 原样读回;
//	② 未分级的行(NULL,如历史数据)grade 为 nil —— 前端据此归入常规,不得消失。
func TestAnnounceGradeRoundTrip(t *testing.T) {
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
	srcName := "t68-公告分级-" + suffix

	var srcID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO sources(name,source_type) VALUES($1,'announcement') RETURNING id`,
		srcName).Scan(&srcID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM raw_documents WHERE source_id=$1`, srcID)
		_, _ = pool.Exec(ctx, `DELETE FROM sources WHERE id=$1`, srcID)
	})

	mustTitle := "关于收到中国证券监督管理委员会立案告知书的公告"
	plainTitle := "关于未分级历史行的公告"
	mustGrade := announce.Grade(mustTitle)
	if mustGrade != announce.LevelMust {
		t.Fatalf("前置:示例标题应判为 must, got %s", mustGrade)
	}
	h := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

	// ① 已分级
	if ok, err := s.InsertRawDocument(ctx, &model.RawDocument{
		SourceID: srcID, Title: &mustTitle, Content: mustTitle,
		ContentHash: h(mustTitle), Status: "collected", Grade: &mustGrade,
	}); err != nil || !ok {
		t.Fatalf("插入已分级行失败: ok=%v err=%v", ok, err)
	}
	// ② 未分级(NULL)——模拟本迁移前的历史行
	if ok, err := s.InsertRawDocument(ctx, &model.RawDocument{
		SourceID: srcID, Title: &plainTitle, Content: plainTitle,
		ContentHash: h(plainTitle), Status: "collected",
	}); err != nil || !ok {
		t.Fatalf("插入未分级行失败: ok=%v err=%v", ok, err)
	}

	rows, err := s.ListAnnouncementsWithSource(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var gotMust, gotPlain *string
	for _, r := range rows {
		if r.Source != srcName {
			continue
		}
		switch r.Title {
		case mustTitle:
			gotMust = r.Grade
		case plainTitle:
			gotPlain = r.Grade
		}
	}
	if gotMust == nil || *gotMust != announce.LevelMust {
		t.Errorf("已分级行读回应为 must, got %v", gotMust)
	}
	if gotPlain != nil {
		t.Errorf("未分级行读回应为 NULL, got %q", *gotPlain)
	}
}
