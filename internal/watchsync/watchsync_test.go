package watchsync

// 纯策略离线单测(零 DB / 零网络)。fixture 驱动的表用例 —— 协议/规则一改,单测先红。

import (
	"testing"
	"time"

	"piks/internal/ths"
)

func TestFilter(t *testing.T) {
	in := []ths.SelfStock{
		{Code: "601091", MarketID: "17"}, // SH A 股 → 保留
		{Code: "N225", MarketID: "48"},   // 指数 → not_stock_code(非 6 位数字)
		{Code: "", MarketID: "17"},       // 无 code → no_code
		{Code: "KS11", MarketID: "48"},   // 指数 → not_stock_code
		{Code: "00700", MarketID: "55"},  // 港股(5 位码)→ not_stock_code(先于市场白名单命中)
		{Code: "000001", MarketID: "33"}, // SZ → 保留
		{Code: "688041", MarketID: "18"}, // 科创板 → 保留
		{Code: "830799", MarketID: "71"}, // 北交所 → 保留
		{Code: "600519", MarketID: "20"}, // 沪 ETF → market_not_ashare
		{Code: "601091", MarketID: "17"}, // 重复 → 去重
	}
	kept, dropped := Filter(in)
	// 10 条输入:4 保留、5 丢弃、1 条重复被静默去重(重复不算「丢弃」,是同一只)。
	if len(kept) != 4 {
		t.Fatalf("应保留 4 只 A 股, got %d: %+v", len(kept), kept)
	}
	wantKept := map[string]string{"601091": "17", "000001": "33", "688041": "18", "830799": "71"}
	for _, k := range kept {
		if wantKept[k.Code] != k.MarketID {
			t.Errorf("保留项 %s marketid 应为 %q, got %q", k.Code, wantKept[k.Code], k.MarketID)
		}
	}
	if len(dropped) != 5 {
		t.Fatalf("应丢弃 5 条, got %d: %+v", len(dropped), dropped)
	}
	byCode := map[string]string{}
	for _, d := range dropped {
		byCode[d.Code] = d.Reason
	}
	// ⚠️ code 已过 NormalizeCode(小写化),故用归一后的键断言。
	for code, want := range map[string]string{
		"n225": ReasonNotStock, "": ReasonNoCode, "ks11": ReasonNotStock,
		"00700":  ReasonNotStock, // 5 位非 A 股码:格式门先于市场门命中
		"600519": ReasonMarketExcl,
	} {
		if byCode[code] != want {
			t.Errorf("丢弃 %q 原因应为 %q, got %q", code, want, byCode[code])
		}
	}
}

func TestMerge(t *testing.T) {
	price := 12.34
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	kept := []ths.SelfStock{
		{Code: "601091", MarketID: "17"},
		{Code: "000001", MarketID: "33"},
	}
	details := []ths.Detail{
		{Code: "601091", MarketID: "17", Price: &price, AddedOn: &day},
		// 000001 无元数据
	}
	entries, missing := Merge(kept, details)
	if len(entries) != 2 {
		t.Fatalf("应 2 条, got %d", len(entries))
	}
	if missing != 1 {
		t.Fatalf("缺价计数应为 1, got %d", missing)
	}
	if entries[0].Market != "SH" || entries[1].Market != "SZ" {
		t.Errorf("market 缩写错: %q %q", entries[0].Market, entries[1].Market)
	}
	if entries[0].Price == nil || *entries[0].Price != 12.34 {
		t.Errorf("601091 价应为 12.34, got %v", entries[0].Price)
	}
	if entries[1].Price != nil || entries[1].AddedOn != nil {
		t.Errorf("000001 无元数据,价/日应为 nil, got %v %v", entries[1].Price, entries[1].AddedOn)
	}
}

func TestDiffAddKeepRemoveOrder(t *testing.T) {
	existing := []Existing{
		{Code: "600000", EntityID: "e-600000"}, // 本地有,上游无 → remove
		{Code: "601091", EntityID: "e-601091"}, // 两边都有 → keep
	}
	incoming := []Entry{
		{Code: "601091", Market: "SH"},
		{Code: "000001", Market: "SZ"}, // 上游有,本地无 → add
	}
	p := Diff(existing, incoming)
	add, keep, remove := p.Counts()
	if add != 1 || keep != 1 || remove != 1 {
		t.Fatalf("add/keep/remove 应为 1/1/1, got %d/%d/%d", add, keep, remove)
	}
	// 顺序:add/keep 在前(按 code 升序),remove 在后。
	wantKinds := []string{ChangeAdd, ChangeKeep, ChangeRemove}
	wantCodes := []string{"000001", "601091", "600000"}
	for i, c := range p.Changes {
		if c.Kind != wantKinds[i] || c.Code != wantCodes[i] {
			t.Fatalf("第 %d 条应为 %s/%s, got %s/%s", i, wantKinds[i], wantCodes[i], c.Kind, c.Code)
		}
	}
	// remove 必须带 EntityID(供置 archived)。
	if p.Changes[2].EntityID != "e-600000" {
		t.Errorf("remove 项应带 EntityID, got %q", p.Changes[2].EntityID)
	}
}

func TestDiffEmptyBoth(t *testing.T) {
	p := Diff(nil, nil)
	if len(p.Changes) != 0 {
		t.Fatalf("两侧皆空应无变更, got %+v", p.Changes)
	}
}

func TestDueSlot(t *testing.T) {
	slots := []Slot{
		{At: "09:00", Grace: 90 * time.Minute},
		{At: "12:55", Grace: 90 * time.Minute},
		{At: "18:00", Grace: 4 * time.Hour},
	}
	mk := func(hh, mm int) time.Time {
		return time.Date(2026, 9, 22, hh, mm, 0, 0, cst)
	}
	noDone := func(string) bool { return false }

	// ① 08:00:未到任何时点 → 无 due
	if due, _ := DueSlot(mk(8, 0), "2026-09-22", slots, noDone); due != "" {
		t.Fatalf("08:00 不应有 due, got %q", due)
	}
	// ② 09:30:09:00 刚过且在窗内 → due = 09:00
	if due, _ := DueSlot(mk(9, 30), "2026-09-22", slots, noDone); due != "2026-09-22#09:00" {
		t.Fatalf("09:30 应 due 09:00, got %q", due)
	}
	// ③ 09:00 已跑过、12:55 已过 → due = 12:55
	done9 := func(k string) bool { return k == "2026-09-22#09:00" }
	if due, _ := DueSlot(mk(13, 0), "2026-09-22", slots, done9); due != "2026-09-22#12:55" {
		t.Fatalf("应 due 12:55, got %q", due)
	}
	// ④ 14:10:09:00 超 90m 窗(09:00+90m=10:30)→ missed;12:55 在窗内(到 14:25)→ due
	due, missed := DueSlot(mk(14, 10), "2026-09-22", slots, noDone)
	if due != "2026-09-22#12:55" {
		t.Fatalf("应 due 12:55, got %q", due)
	}
	if len(missed) != 1 || missed[0] != "2026-09-22#09:00" {
		t.Fatalf("应 missed 09:00, got %v", missed)
	}
	// ⑤ 18:00 后的宽容窗是 4h(到 22:00),20:00 仍可补
	if due, _ := DueSlot(mk(20, 0), "2026-09-22", slots, noDone); due != "2026-09-22#18:00" {
		t.Fatalf("20:00 应 due 18:00(4h 窗), got %q", due)
	}
	// ⑥ 23:00:18:00 也超窗 → 全部 missed、无 due(不补陈旧快照)
	due, missed = DueSlot(mk(23, 0), "2026-09-22", slots, noDone)
	if due != "" {
		t.Fatalf("23:00 不应有 due, got %q", due)
	}
	if len(missed) != 3 {
		t.Fatalf("23:00 应 3 个 missed, got %v", missed)
	}
}

func TestSlotKey(t *testing.T) {
	if got := SlotKey("2026-09-22", "09:00"); got != "2026-09-22#09:00" {
		t.Fatalf("SlotKey 格式错: %q", got)
	}
}

func TestParseSlots(t *testing.T) {
	slots, err := ParseSlots("18:00, 09:00 , 12:55", 90*time.Minute, 4*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 3 {
		t.Fatalf("应 3 个时点, got %d", len(slots))
	}
	// 应升序
	if slots[0].At != "09:00" || slots[1].At != "12:55" || slots[2].At != "18:00" {
		t.Fatalf("应升序排列, got %+v", slots)
	}
	// 末个用 tail grace,其余 default
	if slots[0].Grace != 90*time.Minute || slots[2].Grace != 4*time.Hour {
		t.Fatalf("grace 分配错: %+v", slots)
	}
	if _, err := ParseSlots("9點", time.Minute, time.Minute); err == nil {
		t.Fatal("非法时点应报错")
	}
	if _, err := ParseSlots("", time.Minute, time.Minute); err == nil {
		t.Fatal("空列表应报错")
	}
}

func TestBeijingDay(t *testing.T) {
	// UTC 2026-09-21 17:00 = 北京 2026-09-22 01:00 → 北京日应为 09-22。
	utc := time.Date(2026, 9, 21, 17, 0, 0, 0, time.UTC)
	if got := BeijingDay(utc); got != "2026-09-22" {
		t.Fatalf("北京时间日应为 2026-09-22, got %q", got)
	}
}
