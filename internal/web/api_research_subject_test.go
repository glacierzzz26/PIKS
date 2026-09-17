package web

import (
	"encoding/json"
	"testing"

	"piks/internal/store"
)

// TestSubjectPresentation 主体展示名(P9-2 版面 / issue #13 宏观)。
//
// 三类主体的展示名**各有来源**:
//   - 公司  :entities 富化名(入参 name)
//   - 行业  :metrics.industry_index.ref.name
//   - 宏观  :metrics.macro.ref.name(#13)
//
// 关键回归:宏观若落到公司分支,会去查 `type='company'` 的实体表 → 空名。
// 且 Go 侧**不得**内置维度表(key→名 属 research 侧知识,D-M2/D-11)。
func TestSubjectPresentation(t *testing.T) {
	cases := []struct {
		name     string
		code     string
		entity   string // entities 富化名(公司用)
		metrics  string
		wantType string
		wantDisp string
	}{
		{
			name:     "公司:沿用实体富化名",
			code:     "600519",
			entity:   "贵州茅台",
			metrics:  `{}`,
			wantType: store.SubjectCompany,
			wantDisp: "贵州茅台",
		},
		{
			name:     "公司:无富化名则空(不臆测,前端退回代码)",
			code:     "600519",
			entity:   "",
			metrics:  `{}`,
			wantType: store.SubjectCompany,
			wantDisp: "",
		},
		{
			name:     "行业:取 industry_index.ref.name",
			code:     "sw801010",
			metrics:  `{"industry_index":{"ref":{"name":"农林牧渔"}}}`,
			wantType: store.SubjectIndustry,
			wantDisp: "农林牧渔",
		},
		{
			name:     "宏观:取 macro.ref.name",
			code:     "macro:cn_cpi",
			metrics:  `{"macro":{"ref":{"name":"居民消费价格指数（CPI）"}}}`,
			wantType: store.SubjectMacro,
			wantDisp: "居民消费价格指数（CPI）",
		},
		{
			// 回归:宏观码配行业形态的 metrics(或反之)不该串名 —— 键不同即取不到。
			name:     "宏观:指标卡里没有 macro 键则空",
			code:     "macro:cn_gdp",
			metrics:  `{"industry_index":{"ref":{"name":"农林牧渔"}}}`,
			wantType: store.SubjectMacro,
			wantDisp: "",
		},
		{
			name:     "宏观:空 metrics 退回空串",
			code:     "macro:cn_m2",
			metrics:  ``,
			wantType: store.SubjectMacro,
			wantDisp: "",
		},
		{
			name:     "行业:坏 JSON 退回空串,不阻断渲染",
			code:     "sw851251",
			metrics:  `{not json`,
			wantType: store.SubjectIndustry,
			wantDisp: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &store.ResearchRun{
				Code:    c.code,
				Metrics: json.RawMessage(c.metrics),
			}
			gotType, gotDisp := subjectPresentation(r, c.entity)
			if gotType != c.wantType {
				t.Errorf("subjectType = %q, want %q", gotType, c.wantType)
			}
			if gotDisp != c.wantDisp {
				t.Errorf("displayName = %q, want %q", gotDisp, c.wantDisp)
			}
		})
	}
}
