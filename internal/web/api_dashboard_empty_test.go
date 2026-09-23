package web

import (
	"encoding/json"
	"testing"

	"piks/internal/model"
	"piks/internal/store"
)

// TestEmptyMarketSnapshotNoNullArrays 回归 issue #59:空库时 /api/v1/dashboard 的
// market 块曾把 slice 字段编码成 `null`,前端按非可选数组声明并 `.map()` → 首页白屏。
//
// 断言的是**真实 JSON 编码结果**,不是 Go 层 `len()==0` —— 因为缺陷正在
// 「零值 nil slice 经 encoding/json 变 null」这一步,长度检查根本照不到。
func TestEmptyMarketSnapshotNoNullArrays(t *testing.T) {
	raw, err := json.Marshal(emptyMarketSnapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"indices", "ladder", "industry_dist"} {
		v, ok := m[key]
		if !ok {
			t.Errorf("market.%s 字段缺失(前端类型声明为必填)", key)
			continue
		}
		if string(v) == "null" {
			t.Errorf("market.%s = null —— 前端 .map() 会崩(issue #59);应为 []", key)
		}
		if string(v) != "[]" {
			t.Errorf("market.%s = %s, want []", key, v)
		}
	}
	// 标量字段必须齐全(前端无条件读 trade_date / emotion_score 等)。
	for _, key := range []string{
		"trade_date", "limit_up", "limit_down", "broken_limit", "max_board",
		"turnover_yi", "emotion_score", "emotion_state",
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("market.%s 字段缺失", key)
		}
	}
}

// TestZeroValueSlicesAreNull 钉住**缺陷机制本身**,防止有人「简化」回去。
// 这不是测产品行为,是测 encoding/json 的语义 —— 它解释了为何构造时必须显式给空 slice。
func TestZeroValueSlicesAreNull(t *testing.T) {
	raw, err := json.Marshal(apiMarketSnapshot{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(m["indices"]) != "null" {
		t.Errorf("零值 slice 应编码为 null(这是本 issue 的成因);got %s", m["indices"])
	}
	if string(m["ladder"]) != "null" {
		t.Errorf("零值 slice 应编码为 null;got %s", m["ladder"])
	}
}

// TestToEventItemFactsNeverNull 同类 sweep(issue #59):facts 走 json.Unmarshal,
// 源 JSON 为 null / 字段缺失时都留 nil → 序列化成 `null`,而 EventDetail 直接
// `event.facts.map()`。空 facts 必须编码成 `[]`。
func TestToEventItemFactsNeverNull(t *testing.T) {
	for name, raw := range map[string]string{
		"null":    `null`,
		"missing": ``,
		"empty":   `[]`,
	} {
		ev := toEventItem(store.EventForAPI{ID: "e1", Facts: json.RawMessage(raw)}, eventItemInput{})
		out, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("%s: unmarshal: %v", name, err)
		}
		if string(m["facts"]) == "null" {
			t.Errorf("facts 源为 %q 时编码成 null(issue #59);应为 []", raw)
		}
		if string(m["affected"]) == "null" {
			t.Errorf("facts 源为 %q 时 affected 也编码成 null;应为 []", raw)
		}
	}
}

// TestToEntityAliasesNeverNull 同类 sweep(issue #59):前端 CommandPalette 的
// `e.aliases.some(...)` 与 entities.tsx 的 `e.aliases.join(...)` 都是裸调用。
func TestToEntityAliasesNeverNull(t *testing.T) {
	for name, raw := range map[string]string{"null": `null`, "missing": ``, "empty": `[]`} {
		e := toEntity(model.Entity{ID: "n1", Type: "company", Name: "X", Aliases: json.RawMessage(raw)})
		out, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("%s: unmarshal: %v", name, err)
		}
		if string(m["aliases"]) == "null" {
			t.Errorf("aliases 源为 %q 时编码成 null(issue #59);应为 []", raw)
		}
	}
}
