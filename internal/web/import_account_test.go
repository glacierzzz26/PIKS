package web

import (
	"encoding/json"
	"testing"
)

// TestBuildImportPreviewAccount 账户汇总抽取(issue #19):
// 关键语义是 null/缺项 → 空串(确认时落 NULL),**绝不折成 0**。
func TestBuildImportPreviewAccount(t *testing.T) {
	// 1. 四项齐 → 原值
	prev, err := buildImportPreview(t.Context(), nil, "position", "att-1", json.RawMessage(`{
		"account":{"total_asset":520000.5,"total_mv":145050,"float_pl":5050,"daily_pl":1200},
		"positions":[{"code":"600519","name":"贵州茅台","qty":100,"cost_price":1400,"price":1450.5,"market_value":145050,"pl":5050}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if prev.Account.TotalAsset != "520000.5" || prev.Account.TotalMV != "145050" ||
		prev.Account.FloatPL != "5050" || prev.Account.DailyPL != "1200" {
		t.Fatalf("账户四项未原值解析: %+v", prev.Account)
	}
	if len(prev.Positions) != 1 {
		t.Fatalf("持仓行数 = %d, want 1", len(prev.Positions))
	}

	// 2. 整块缺失 → 四项全空串(落 NULL),不是 "0"
	prev, err = buildImportPreview(t.Context(), nil, "position", "att-2", json.RawMessage(`{
		"positions":[{"code":"600519","name":"贵州茅台","qty":100,"cost_price":1400,"price":1450.5,"market_value":145050,"pl":5050}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if prev.Account.TotalAsset != "" || prev.Account.TotalMV != "" ||
		prev.Account.FloatPL != "" || prev.Account.DailyPL != "" {
		t.Fatalf("缺 account 块时应为空串(→NULL),得到 %+v", prev.Account)
	}

	// 3. 部分 null / 缺项 → 该项空串,其余正常(**不互相污染**)
	prev, err = buildImportPreview(t.Context(), nil, "position", "att-3", json.RawMessage(`{
		"account":{"total_asset":null,"total_mv":145050,"float_pl":null,"daily_pl":-800},
		"positions":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if prev.Account.TotalAsset != "" {
		t.Fatalf("null total_asset 应 → 空串,得到 %q", prev.Account.TotalAsset)
	}
	if prev.Account.FloatPL != "" {
		t.Fatalf("null float_pl 应 → 空串,得到 %q", prev.Account.FloatPL)
	}
	if prev.Account.TotalMV != "145050" {
		t.Fatalf("total_mv 应保留,得到 %q", prev.Account.TotalMV)
	}
	// 负值(亏)必须原样保留 —— 别让"空"与"负"混淆
	if prev.Account.DailyPL != "-800" {
		t.Fatalf("负值 daily_pl 应原样保留,得到 %q", prev.Account.DailyPL)
	}

	// 4. 真 0 与 null 必须可区分(issue #19 核心:0 ≠ 没有)
	prev, err = buildImportPreview(t.Context(), nil, "position", "att-4", json.RawMessage(`{
		"account":{"total_asset":0,"total_mv":0,"float_pl":0,"daily_pl":0},
		"positions":[]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if prev.Account.TotalAsset != "0" || prev.Account.DailyPL != "0" {
		t.Fatalf("真 0 应解析为 \"0\"(区别于空串): %+v", prev.Account)
	}
}
