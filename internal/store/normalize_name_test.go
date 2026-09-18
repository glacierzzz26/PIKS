package store

import "testing"

// TestNormalizeStockName 覆盖 issue #6 的两类名字:东财给历史改名票打的空格标记
// (须归一)与英文多词名(空格有意义,须原样保留)。表驱动,含边界。
func TestNormalizeStockName(t *testing.T) {
	cases := []struct{ in, want string }{
		// —— 东财带空格的中文名 → 归一(issue #6 实测全部 11 例)——
		{"金 螳 螂", "金螳螂"},
		{"南 京 港", "南京港"},
		{"新 希 望", "新希望"},
		{"英 力 特", "英力特"},
		{"罗 牛 山", "罗牛山"},
		{"新 大 陆", "新大陆"},
		{"生 意 宝", "生意宝"},
		{"远 望 谷", "远望谷"},
		{"红 宝 丽", "红宝丽"},
		{"粤 传 媒", "粤传媒"},
		{"新 华 都", "新华都"},
		// 全角空格也算空白
		{"金　螳　螂", "金螳螂"},
		// —— 英文多词名:空格有意义,原样 ——
		{"Hugging Face", "Hugging Face"},
		{"SB Energy", "SB Energy"},
		{"Stoke Space", "Stoke Space"},
		{"Starman Optical", "Starman Optical"},
		// —— 干净名 / 边界 ——
		{"贵州茅台", "贵州茅台"},
		{"", ""},
		{"   ", ""},
		{"  贵州茅台  ", "贵州茅台"},
		// 中英混合:含非汉字 → 不动(保守,不让归一吃掉有意义的分隔)
		{"ST 中安", "ST 中安"},
		// 单字间空格但整体含 ASCII:保守保留
		{"A B", "A B"},
	}
	for _, c := range cases {
		if got := NormalizeStockName(c.in); got != c.want {
			t.Errorf("NormalizeStockName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeStockNameIdempotent 归一后必须稳定(重跑不再变),否则 entity-build
// 每日 upsert 会产生 churn。
func TestNormalizeStockNameIdempotent(t *testing.T) {
	for _, s := range []string{"金 螳 螂", "金螳螂", "Hugging Face", "贵州茅台"} {
		once := NormalizeStockName(s)
		if twice := NormalizeStockName(once); twice != once {
			t.Errorf("非幂等: %q → %q → %q", s, once, twice)
		}
	}
}
