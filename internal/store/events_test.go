package store

import (
	"encoding/json"
	"testing"
)

// TestEmptyToArrayIfScalar 覆盖 issue #71 的**写入侧守卫**。
//
// 事故:旧守卫只判 `len(affected) == 0`,而 `json.Marshal(nil)` 产出的字面量 `null`
// 长度是 4 → 守卫看不穿、放行入库。本用例锁死「null / 空 / 标量 → `[]`」,
// 并保证合法数组**原样透传**(不误伤正常数据)。
func TestEmptyToArrayIfScalar(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"零长度", "", "[]"},
		{"字面量 null(事故值)", "null", "[]"},
		{"空数组", "[]", "[]"},
		{"单元素数组", `["半导体"]`, `["半导体"]`},
		{"多元素数组", `["银行","房地产"]`, `["银行","房地产"]`},
		{"字符串标量", `"abc"`, "[]"},
		{"数字标量", "123", "[]"},
		{"对象标量", `{"a":1}`, "[]"},
		{"非法 JSON", `{oops`, "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(emptyToArrayIfScalar(json.RawMessage(c.in)))
			if got != c.want {
				t.Fatalf("emptyToArrayIfScalar(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}
