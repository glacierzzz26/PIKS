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
