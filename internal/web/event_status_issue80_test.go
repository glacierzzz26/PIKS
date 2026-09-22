package web

import (
	"encoding/json"
	"testing"
)

// TestEventStatusMappingPairing 回归 issue #80:抽取态筛选曾恒空。
//
// 旧映射:confirmed → verified|published、pending → extracted。但 `verified`/`published`
// 的**唯一写入方是已随 P6 下线的 vault 发布器**(发布生命周期改由 published_at 承载),
// 生产实测无任何 verified/published 行 → 「已确认」永远 0 条、「待复核」= 全量,
// 两个 chip 都没有鉴别力。本测试钉住新约定:前端取值与后端在产态**一一对应**。
func TestEventStatusMappingPairing(t *testing.T) {
	cases := []struct {
		backend string
		front   string
	}{
		{"extracted", "extracted"}, // 唯一由 internal/extract 写入的在产态
		{"merged", "merged"},       // 聚类并入代表后的合并态(仍下发给前端)
		{"verified", "extracted"},  // 历史遗留行,知识态上仍「已抽取未合并」
		{"published", "extracted"},
	}
	for _, c := range cases {
		if got := eventStatusFront(c.backend); got != c.front {
			t.Errorf("eventStatusFront(%q) = %q, want %q", c.backend, got, c.front)
		}
		// 筛选必须能命中该口径自身的映射结果 —— 否则就是「点了没反应」的死选项。
		if !eventStatusOK(c.backend, c.front) {
			t.Errorf("eventStatusOK(%q, %q) = false —— 前端筛选项点不出本条", c.backend, c.front)
		}
	}
}

// TestEventStatusFilterNoDeadOption 回归 issue #80:不得再有结构上恒空的筛选项。
//
// 「已确认」的根因是映射到永不产生的 verified|published。此测试锁死:**每个前端
// 筛选项都必须至少命中一种被 eventStatusFront 认可的后端态**,否则即死选项复活。
func TestEventStatusFilterNoDeadOption(t *testing.T) {
	backendStates := []string{"extracted", "merged", "verified", "published"}
	frontOptions := []string{"extracted", "merged"}
	for _, opt := range frontOptions {
		hit := false
		for _, bs := range backendStates {
			if eventStatusOK(bs, opt) {
				hit = true
				break
			}
		}
		if !hit {
			t.Errorf("筛选项 %q 命中不到任何在产后端态 —— 又一个「已确认」式死选项", opt)
		}
	}
	// 空筛选 = 全放行(「全部」)。
	for _, bs := range backendStates {
		if !eventStatusOK(bs, "") {
			t.Errorf("空筛选应放行全部,却排除了 %q", bs)
		}
	}
}

// TestAffectedTermsSearchable 回归 issue #80:事件搜索框承诺搜「影响实体」,
// 但 handleAPIEvents 只 strSub(Title, Summary) → 按公司名搜不到。
//
// affectedTerms 把 affected JSON 词表摊平供 strSub 使用;此处钉住摊平口径与
// 解析失败(非法 JSON / null)时不 panic、不误匹配。
func TestAffectedTermsSearchable(t *testing.T) {
	// 真实形状:affected 是字符串数组的 JSON(`["贵州茅台","白酒"]`)。
	terms := affectedTerms(json.RawMessage(`["贵州茅台","白酒"]`))
	if !strSub("贵州茅台", terms) {
		t.Errorf("affectedTerms = %q,应能被实体名搜到", terms)
	}
	if !strSub("白酒", terms) {
		t.Errorf("affectedTerms = %q,应能被第二个实体搜到", terms)
	}
	// 解析失败的输入不得 panic、不得产出会误匹配的字符串。
	for _, bad := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`{"a":1}`)} {
		if got := affectedTerms(bad); got != "" {
			t.Errorf("affectedTerms(%s) = %q, want 空串", bad, got)
		}
		if strSub("任意词", affectedTerms(bad)) {
			t.Errorf("affectedTerms(%s) 解析失败却匹配上了任意词", bad)
		}
	}
}
