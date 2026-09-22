package extract

import (
	"encoding/json"
	"testing"
)

// TestMustJSONArrayNeverNull 是 issue #71 的**直接回归测试**。
//
// 事故:裸 `json.Marshal` 对 nil 切片返回字面量 `null`(err == nil),于是
// events.affected 落成 JSON null;消费方 `jsonb_array_elements_text` 遇标量报
// SQLSTATE 22023 → entity-build 每轮必崩。本用例锁死「nil/空 → `[]`,绝不是 `null`」。
func TestMustJSONArrayNeverNull(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"nil 切片", nil, "[]"},
		{"空切片", []string{}, "[]"},
		{"单元素", []string{"半导体"}, `["半导体"]`},
		{"多元素", []string{"银行", "房地产"}, `["银行","房地产"]`},
		{"空字符串元素", []string{""}, `[""]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(mustJSONArray(c.in))
			if got == "null" {
				t.Fatalf("mustJSONArray(%v) = null —— 正是 issue #71 的根因,不允许", c.in)
			}
			if got != c.want {
				t.Fatalf("mustJSONArray(%v) = %s, want %s", c.in, got, c.want)
			}
			// 产物必须是合法 JSON,且反序列化为数组(非标量)。
			var probe any
			if err := json.Unmarshal([]byte(got), &probe); err != nil {
				t.Fatalf("产物不是合法 JSON: %v (%s)", err, got)
			}
			if _, ok := probe.([]any); !ok {
				t.Fatalf("产物不是 JSON 数组: %T (%s)", probe, got)
			}
		})
	}
}
