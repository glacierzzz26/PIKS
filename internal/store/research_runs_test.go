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
// ⚠️ P9 起这是**公司专用**校验:行业主体(sw801010)不在此列,应走 NormalizeSubject。
func TestValidStockCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sh600519", "600519"}, // 归一后合法
		{"sz000560", "000560"},
		{"600519", "600519"},
		{"海南橡胶", ""}, // 名称:关键回归,绝不返回名称
		{"沃特股份", ""},
		{"", ""},
		{"sh60051", ""},  // 长度不符
		{"sw801010", ""}, // 行业码不是股票码(P9):行业走 NormalizeSubject
	}
	for _, c := range cases {
		if got := store.ValidStockCode(c.in); got != c.want {
			t.Errorf("ValidStockCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeSubject 主体轴归一(P9 / issue #10):公司 / 行业 / 宏观三分。
// 关键回归:行业码 sw801010 绝不能被当公司(否则 SubjectFullCode 会产出 bj801010)。
func TestNormalizeSubject(t *testing.T) {
	cases := []struct {
		in       string
		wantType string
		wantCode string
	}{
		// 公司:既有语义不变
		{"600519", store.SubjectCompany, "600519"},
		{"sh600519", store.SubjectCompany, "600519"},
		{"sz000560", store.SubjectCompany, "000560"},
		{"bj430047", store.SubjectCompany, "430047"},
		{"  600519  ", store.SubjectCompany, "600519"},
		// 行业:sw + 6 位申万码
		{"sw801010", store.SubjectIndustry, "sw801010"}, // 农林牧渔(104 只);规范码带前缀
		{"sw851251", store.SubjectIndustry, "sw851251"}, // 白酒Ⅲ
		{"SW801010", store.SubjectIndustry, "sw801010"}, // 大小写不敏感(归一为小写)
		{"  sw801010  ", store.SubjectIndustry, "sw801010"},
		// 宏观:#13 预留
		{"macro:cpi", store.SubjectMacro, "macro:cpi"},
		{"MACRO:gdp-yoy", store.SubjectMacro, "macro:gdp-yoy"},
		// 不可识别:名称 / 残缺 / 恰是地雷(裸申万码会被当北交所股票)
		{"海南橡胶", "", ""},
		{"sw80101", "", ""},                        // 位数不足
		{"sw8010101", "", ""},                      // 位数过多
		{"swabcdef", "", ""},                       // 非数字
		{"macro:", "", ""},                         // 空 key
		{"801010", store.SubjectCompany, "801010"}, // 裸码仍按公司(下游会 bj 前缀;调用方应传 sw801010)
	}
	for _, c := range cases {
		gotType, gotCode := store.NormalizeSubject(c.in)
		if gotType != c.wantType || gotCode != c.wantCode {
			t.Errorf("NormalizeSubject(%q) = (%q, %q), want (%q, %q)",
				c.in, gotType, gotCode, c.wantType, c.wantCode)
		}
	}
}

// TestNormalizeCodeIndustryPassthrough 记录 P9 依赖的**底层事实**:
// NormalizeCode 对行业码原样穿过(它只剥 sh/sz/bj 且要求 len==前缀+6)。
// 这条一旦变了,主体轴的「行业码零冲突」前提就不成立 —— 故显式锁住。
func TestNormalizeCodeIndustryPassthrough(t *testing.T) {
	for _, c := range []string{"sw801010", "sw851251"} {
		if got := store.NormalizeCode(c); got != c {
			t.Errorf("NormalizeCode(%q) = %q, want 原样穿过(主体轴依赖此性质)", c, got)
		}
	}
}
