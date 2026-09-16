package store_test

import (
	"testing"

	"piks/internal/store"
)

// TestNormalizeCode research full_code → PIKS 6 位代码(设计 §4.6 归一规则,join 正确性的根)。
// 纯函数单测:不依赖数据库,`go test ./...` 默认跑。
func TestNormalizeCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sh600519", "600519"},
		{"sz000560", "000560"},
		{"SH600519", "600519"},
		{"sz300750", "300750"},
		{"bj430047", "430047"},
		{"600519", "600519"}, // 已归一
		{"  sz000560  ", "000560"},
		{"sh60051", "sh60051"}, // 长度不符,不剥前缀(防误伤)
		{"", ""},
	}
	for _, c := range cases {
		if got := store.NormalizeCode(c.in); got != c.want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestIsStockCode 6 位数字校验(issue #2 的根因闸门)。
// NormalizeCode 不校验数字,股票名称会原样穿过;进编排/入库前必须过 IsStockCode。
func TestIsStockCode(t *testing.T) {
	valid := []string{"600519", "000560", "300750", "430047", "002703", "001208"}
	for _, c := range valid {
		if !store.IsStockCode(c) {
			t.Errorf("IsStockCode(%q) = false, want true", c)
		}
	}
	// 非 6 位数字的一律拒:股票名称 / 港股带前缀 / 带字母 / 长度不符 / 空白
	invalid := []string{
		"海南橡胶", "sh600519", "60051", "6005190", "60051a", "abc123", "", "  ",
		"600 519", "１２３４５６", // 全角数字
	}
	for _, c := range invalid {
		if store.IsStockCode(c) {
			t.Errorf("IsStockCode(%q) = true, want false", c)
		}
	}
}

// TestValidStockCode 归一 + 校验合体:合法返回 6 位码,名称等一律返回 ""。
func TestValidStockCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sh600519", "600519"}, // 归一后合法
		{"sz000560", "000560"},
		{"600519", "600519"},
		{"海南橡胶", ""}, // 名称:关键回归,绝不返回名称
		{"沃特股份", ""},
		{"", ""},
		{"sh60051", ""}, // 长度不符
	}
	for _, c := range cases {
		if got := store.ValidStockCode(c.in); got != c.want {
			t.Errorf("ValidStockCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
