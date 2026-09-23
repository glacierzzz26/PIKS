package cluster

import (
	"reflect"
	"testing"
)

// 同一篇稿的近逐字两版(仅电头不同 ⇒ 归一化后一致)。复用 P-1 判据。
const (
	rawBodyA = "财联社9月21日电，央行今日开展5000亿元中期借贷便利MLF操作，中标利率2.30%，与上期持平。"
	rawBodyB = "金十数据9月21日讯，央行今日开展5000亿元中期借贷便利MLF操作，中标利率2.30%，与上期持平。"
	// 指纹远低于 0.85 的另一篇稿。
	rawBodyC = "国家统计局今日发布数据显示，8月社会消费品零售总额同比增长3.5%，环比回升。"
)

// 与 P-1 GroupReprints 判据一致的前提:两版确实被判为同一组(否则下面的用例前提不成立)。
func TestRawGroupsBodiesAreReprints(t *testing.T) {
	groups := GroupReprints([]string{rawBodyA, rawBodyB})
	if len(groups) != 1 || len(groups[0]) != 2 {
		t.Fatalf("前提不成立:两版近逐字正文应被 P-1 判为同组,实际 %v", groups)
	}
	if n := GroupReprints([]string{rawBodyA, rawBodyC}); len(n) != 2 {
		t.Fatalf("前提不成立:两篇不同稿应各自成组,实际 %v", n)
	}
}

// ≥2 个不同机构 + 近逐字 ⇒ 成组;代表取最早入库者。
func TestRawGroupsFormsGroupAcrossOrgs(t *testing.T) {
	docs := []RawDocEntry{
		{ID: "b", SourceID: "org2", Content: rawBodyB, RetrievedAt: "2026-09-21T10:00:00Z"},
		{ID: "a", SourceID: "org1", Content: rawBodyA, RetrievedAt: "2026-09-21T09:00:00Z"},
	}
	got := RawGroups(docs)
	want := []RawGroup{{RepID: "a", Members: []string{"b"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("成组错误\n got=%+v\nwant=%+v", got, want)
	}
}

// 同机构两条近逐字 ⇒ **不**成组(这不是「多家转载同一篇稿」)。
func TestRawGroupsSameOrgNotGrouped(t *testing.T) {
	docs := []RawDocEntry{
		{ID: "a", SourceID: "org1", Content: rawBodyA, RetrievedAt: "2026-09-21T09:00:00Z"},
		{ID: "b", SourceID: "org1", Content: rawBodyB, RetrievedAt: "2026-09-21T10:00:00Z"},
	}
	if got := RawGroups(docs); len(got) != 0 {
		t.Fatalf("同机构近逐字不应成组,实际 %+v", got)
	}
}

// 单成员 / 空输入 ⇒ 不成组。
func TestRawGroupsSingletonNoGroup(t *testing.T) {
	if got := RawGroups([]RawDocEntry{{ID: "a", SourceID: "org1", Content: rawBodyA}}); got != nil {
		t.Fatalf("单成员不应成组,实际 %+v", got)
	}
	if got := RawGroups(nil); got != nil {
		t.Fatalf("空输入应返回 nil,实际 %+v", got)
	}
}

// 代表冻结:组内已有 canonical_id 的行 ⇒ 以它为组代表,且它**不出现在 Members**(不重写)。
func TestRawGroupsFrozenRepWinsAndIsNotRewritten(t *testing.T) {
	docs := []RawDocEntry{
		{ID: "a", SourceID: "org1", Content: rawBodyA, RetrievedAt: "2026-09-21T09:00:00Z", CanonicalID: "zzz"},
		{ID: "b", SourceID: "org2", Content: rawBodyB, RetrievedAt: "2026-09-21T10:00:00Z"},
	}
	got := RawGroups(docs)
	if len(got) != 1 {
		t.Fatalf("应成 1 组,实际 %+v", got)
	}
	// 冻结代表优先于「最早入库」规则:a 虽是最早,但已冻结指向 zzz ⇒ 组代表 = zzz。
	if got[0].RepID != "zzz" {
		t.Fatalf("已冻结代表应胜出,got RepID=%q", got[0].RepID)
	}
	// a 已冻结(CanonicalID 非空),不得进 Members;b 需写回。
	if !reflect.DeepEqual(got[0].Members, []string{"b"}) {
		t.Fatalf("已冻结行不应进 Members,got %v", got[0].Members)
	}
}

// 已收敛的组:组内成员全已冻结 ⇒ Members 为空(无需写库,重跑幂等的关键)。
func TestRawGroupsAllFrozenNoMembers(t *testing.T) {
	docs := []RawDocEntry{
		{ID: "a", SourceID: "org1", Content: rawBodyA, RetrievedAt: "2026-09-21T09:00:00Z", CanonicalID: "a"},
		{ID: "b", SourceID: "org2", Content: rawBodyB, RetrievedAt: "2026-09-21T10:00:00Z", CanonicalID: "a"},
	}
	got := RawGroups(docs)
	if len(got) != 1 || len(got[0].Members) != 0 {
		t.Fatalf("全冻结组应无待写成员,实际 %+v", got)
	}
	if got[0].RepID != "a" {
		t.Fatalf("冻结代表应为 a,got %q", got[0].RepID)
	}
}

// 三成员组:代表取最早,同则 id 字典序最小;其余全部进 Members。
func TestRawGroupsRepTieBreakByID(t *testing.T) {
	docs := []RawDocEntry{
		{ID: "c", SourceID: "org3", Content: rawBodyB, RetrievedAt: "2026-09-21T09:00:00Z"},
		{ID: "a", SourceID: "org1", Content: rawBodyA, RetrievedAt: "2026-09-21T09:00:00Z"},
		{ID: "b", SourceID: "org2", Content: rawBodyB, RetrievedAt: "2026-09-21T11:00:00Z"},
	}
	got := RawGroups(docs)
	if len(got) != 1 {
		t.Fatalf("应成 1 组,实际 %+v", got)
	}
	if got[0].RepID != "a" {
		t.Fatalf("同 retrieved_at 应取 id 最小者 a,got %q", got[0].RepID)
	}
	if !reflect.DeepEqual(got[0].Members, []string{"b", "c"}) {
		t.Fatalf("其余成员应全进 Members,got %v", got[0].Members)
	}
}
