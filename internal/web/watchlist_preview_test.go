package web

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"piks/internal/store"
)

// TestBuildWatchPreviewDiff 自选镜像的核心语义(设计 frontend-ia §2.4):
// 截图 = 权威快照 → 截图出现的为 add/keep,现有自选缺失的为 remove。
// 集成测试(需 DB):与 store 集成测试同开关。
func TestBuildWatchPreviewDiff(t *testing.T) {
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

	// 种子:三只公司实体 —— 000001/000002 在自选,000003 不在。
	type seed struct{ name, code, status string }
	seeds := []seed{
		{"镜像测试A", "000001", "watch"},
		{"镜像测试B", "000002", "watch"},
		{"镜像测试C", "000003", "active"},
	}
	for _, sd := range seeds {
		id, err := s.EnsureCompanyEntity(ctx, sd.code, sd.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.SetEntityStatus(ctx, id, sd.status); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM entities WHERE type='company' AND name = ANY($1)`,
			[]string{"镜像测试A", "镜像测试B", "镜像测试C"})
	})

	// 截图含 A(keep,已在自选) 与 C(add,原 active);缺失 B → remove。
	// 截图内重复 code 应去重;非个股条目(无 code)被忽略。
	data := json.RawMessage(`{"stocks":[
		{"code":"000001","name":"镜像测试A"},
		{"code":"sh000001","name":"镜像测试A"},
		{"code":"000003","name":"镜像测试C"},
		{"code":"","name":"上证指数"}
	]}`)
	prev, err := buildImportPreview(ctx, s, "watchlist", "att-x", data)
	if err != nil {
		t.Fatal(err)
	}
	// 断言:种子三行的 change 正确,且每个 code 至多一行(去重)。
	// 不假设自选表只有本测试的种子 —— 库里若有其它 watch 实体(如真实自选),
	// 它们会如实出现在 remove 行(镜像语义正确),故不校验总行数。
	got := map[string]string{} // code → change
	for _, w := range prev.Watch {
		if _, dup := got[w.Code]; dup {
			t.Errorf("code %s 出现重复行", w.Code)
		}
		got[w.Code] = w.Change
	}
	want := map[string]string{"000001": "keep", "000002": "remove", "000003": "add"}
	for code, ch := range want {
		if got[code] != ch {
			t.Errorf("code %s: change = %q, want %q", code, got[code], ch)
		}
	}
	// remove 行默认勾选(镜像语义),add 行默认勾选,keep 行不勾选(无变化)
	for _, w := range prev.Watch {
		if w.Change == "keep" && w.Include {
			t.Errorf("keep 行不应默认勾选: %+v", w)
		}
		if w.Change != "keep" && !w.Include {
			t.Errorf("%s 行应默认勾选: %+v", w.Change, w)
		}
	}
}

// TestPreviewWatchDTO 确认 watch 行序列化字段稳定(前端契约)。
func TestPreviewWatchDTO(t *testing.T) {
	p := &ImportPreview{Kind: "watchlist", AttachmentID: "a", Watch: []PreviewWatch{
		{Include: true, Change: "add", Code: "600519", Name: "贵州茅台"},
	}}
	out := toAPIImportPreview(p)
	b, _ := json.Marshal(out)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["watch"]; !ok {
		t.Fatalf("响应缺 watch 字段: %s", b)
	}
	// trades/positions 应序列化为 [] 而非 null(前端 .map 安全)
	if m["trades"] == nil || m["positions"] == nil {
		t.Fatalf("trades/positions 应为 [] 非 null: %s", b)
	}
}
