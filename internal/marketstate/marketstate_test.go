package marketstate

import (
	"encoding/json"
	"testing"
	"time"

	"piks/internal/model"
)

func floatp(f float64) *float64 { return &f }
func intp(n int) *int           { return &n }

// §5.4 极端输入:全面贪婪行情 → Extreme_Greed + 高分。
func TestComputeEmotionExtremeGreed(t *testing.T) {
	in := EmotionInput{
		LimitUp:   120,
		LimitDown: 0,
		Breadth:   &Breadth{Advance: 4200, Decline: 300},
		BrokenRate: floatp(0.05),
		MaxBoard:  8,
		StrongAvg: floatp(4.5),
		IndustryCount: intp(15),
	}
	res := ComputeEmotion(in)
	if res.State != "Extreme_Greed" {
		t.Fatalf("state = %s, want Extreme_Greed (score=%.1f)", res.State, res.Score)
	}
	if res.Score < 20 {
		t.Fatalf("score = %.1f, want >=20", res.Score)
	}
	// detail 7 项全有值,无 missing
	var detail map[string]EmotionComponent
	if err := json.Unmarshal(res.Detail, &detail); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{CompLimitUp, CompLimitDown, CompBreadth, CompBrokenRate, CompMaxBoard, CompStrongYest, CompIndustry} {
		c, ok := detail[name]
		if !ok || c.Missing {
			t.Fatalf("component %s missing or marked missing", name)
		}
	}
}

// §5.4 极端输入:恐慌行情 → Extreme_Fear + 负分。
func TestComputeEmotionExtremeFear(t *testing.T) {
	in := EmotionInput{
		LimitUp:   1,
		LimitDown: 120,
		Breadth:   &Breadth{Advance: 200, Decline: 4300},
		BrokenRate: floatp(0.6),
		MaxBoard:  1,
		StrongAvg: floatp(-4.0),
		IndustryCount: intp(1),
	}
	res := ComputeEmotion(in)
	if res.State != "Extreme_Fear" {
		t.Fatalf("state = %s, want Extreme_Fear (score=%.1f)", res.State, res.Score)
	}
	if res.Score >= -15 {
		t.Fatalf("score = %.1f, want <=-15", res.Score)
	}
}

// 阈值边界抽查:score 落在 Warm/Neutral 边界。
func TestStateThresholds(t *testing.T) {
	cases := []struct{ score float64; want string }{
		{20, "Extreme_Greed"}, {19.9, "Strong"}, {12, "Strong"}, {6, "Warm"},
		{5, "Neutral"}, {-2, "Neutral"}, {-3, "Weak"}, {-9, "Fear"}, {-15, "Extreme_Fear"},
	}
	for _, c := range cases {
		if got := stateFromScore(c.score); got != c.want {
			t.Errorf("stateFromScore(%v) = %s, want %s", c.score, got, c.want)
		}
	}
}

// 组件缺失不参与求和,detail 标记 missing。
func TestComputeEmotionMissing(t *testing.T) {
	res := ComputeEmotion(EmotionInput{LimitUp: 50, LimitDown: 0, MaxBoard: 4})
	var detail map[string]EmotionComponent
	if err := json.Unmarshal(res.Detail, &detail); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{CompBreadth, CompBrokenRate, CompStrongYest, CompIndustry} {
		if !detail[name].Missing {
			t.Errorf("component %s should be missing", name)
		}
	}
	// 可用组件:limit_up(2×2)+limit_down(2×2)+max_board(2×2) = 4+4+4 = 12
	if res.Score != 12 {
		t.Fatalf("score = %.1f, want 12 (missing excluded)", res.Score)
	}
}

func obsValue(market, indicator, value string) model.Observation {
	return model.Observation{Market: market, Indicator: indicator, Value: value}
}

// §5.5 两日计算:昨日快照 + 今日观测 + prevReturns → strong_yesterday.avg_ret 正确。
func TestComputeSnapshotTwoDay(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-08-26")

	// 昨日快照:zt_pool 含 2 只
	prev := &model.MarketSnapshot{
		TradeDate: time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		ZTPool:    json.RawMessage(`[{"code":"002084","name":"海鸥住工","zdp":10.1,"lbc":3,"fund":1,"hybk":"家居用品"},{"code":"600000","name":"浦发银行","zdp":10.0,"lbc":1,"fund":1,"hybk":"银行"}]`),
	}
	// 今日:两只代码的今日涨跌幅(一只继续涨停,一只回落)
	prevReturns := map[string]float64{"002084": 10.0, "600000": -3.5}

	obs := []model.Observation{
		obsValue("all", IndLimitUpCount, "52"),
		obsValue("all", IndLimitDownCount, "0"),
		obsValue("all", IndBrokenLimit, "20"),
		obsValue("all", IndMaxBoard, "5"),
		obsValue("all", IndZTPool, `[{"code":"002084","name":"海鸥住工","zdp":10.1,"lbc":3,"fund":1,"hybk":"家居用品"}]`),
		obsValue("all", IndIndustryDist, `{"家居用品":1}`),
		obsValue("sh", IndIndexClose, "3912.52"),
		obsValue("sh", IndIndexPct, "0.59"),
	}

	snap, pending := ComputeSnapshot(day, obs, prev, prevReturns)
	if snap == nil {
		t.Fatal("snap is nil")
	}
	// avg_ret = (10.0 + -3.5)/2 = 3.25,count=2
	var sy struct {
		AvgRet float64 `json:"avg_ret"`
		Count  int     `json:"count"`
	}
	if err := json.Unmarshal(snap.StrongYesterday, &sy); err != nil {
		t.Fatal(err)
	}
	if sy.Count != 2 || sy.AvgRet != 3.25 {
		t.Fatalf("strong_yesterday = %+v, want avg_ret=3.25 count=2", sy)
	}
	for _, p := range pending {
		if p == "strong_yesterday" {
			t.Fatalf("strong_yesterday should not be pending with two-day data")
		}
	}
}

// §5.5 首日:无昨日快照 → strong_yesterday 标 missing,pending 含 strong_yesterday。
func TestComputeSnapshotFirstDay(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-08-26")
	obs := []model.Observation{
		obsValue("all", IndLimitUpCount, "52"),
		obsValue("all", IndLimitDownCount, "0"),
		obsValue("all", IndBrokenLimit, "20"),
		obsValue("all", IndMaxBoard, "5"),
		obsValue("all", IndZTPool, `[]`),
		obsValue("all", IndIndustryDist, `{"家居用品":1}`),
	}
	snap, pending := ComputeSnapshot(day, obs, nil, nil)
	if snap.StrongYesterday != nil {
		t.Fatalf("first day strong_yesterday should be nil, got %s", snap.StrongYesterday)
	}
	found := false
	for _, p := range pending {
		if p == "strong_yesterday" {
			found = true
		}
	}
	if !found {
		t.Fatal("pending should include strong_yesterday on first day")
	}
	// 情绪:strong_yesterday 组件 missing
	var detail map[string]EmotionComponent
	if err := json.Unmarshal(snap.EmotionDetail, &detail); err != nil {
		t.Fatal(err)
	}
	if !detail[CompStrongYest].Missing {
		t.Fatal("emotion strong_yesterday should be missing on first day")
	}
}

// §5.3 字段齐全:12 项中可算字段都落位。
func TestComputeSnapshotFields(t *testing.T) {
	day, _ := time.Parse("2006-01-02", "2026-08-26")
	obs := []model.Observation{
		obsValue("all", IndLimitUpCount, "52"),
		obsValue("all", IndLimitDownCount, "0"),
		obsValue("all", IndBrokenLimit, "20"),
		obsValue("all", IndMaxBoard, "5"),
		obsValue("all", IndZTPool, `[]`),
		obsValue("all", IndIndustryDist, `{"家居用品":5,"银行":2}`),
		obsValue("all", IndTurnover, "7200.0"),
		obsValue("all", IndBreadthAdv, "2800"),
		obsValue("all", IndBreadthDec, "1900"),
		obsValue("all", IndBreadthFlat, "150"),
		obsValue("sh", IndIndexClose, "3912.52"),
		obsValue("sh", IndIndexPct, "0.59"),
		obsValue("sz", IndIndexClose, "13000.11"),
		obsValue("sz", IndIndexPct, "-0.21"),
		obsValue("cyb", IndIndexClose, "2500.00"),
		obsValue("cyb", IndIndexPct, "1.02"),
	}
	snap, pending := ComputeSnapshot(day, obs, nil, nil)
	if snap.LimitUpCount == nil || *snap.LimitUpCount != 52 {
		t.Fatalf("limit_up_count = %v", snap.LimitUpCount)
	}
	if snap.MaxBoard == nil || *snap.MaxBoard != 5 {
		t.Fatalf("max_board = %v", snap.MaxBoard)
	}
	if snap.TurnoverAmt == nil || *snap.TurnoverAmt != 7200.0 {
		t.Fatalf("turnover = %v", snap.TurnoverAmt)
	}
	if snap.Breadth == nil {
		t.Fatal("breadth is nil")
	}
	if snap.IndexJSON == nil {
		t.Fatal("index_json is nil")
	}
	// 上证/深成/创业板都在
	var idx map[string]map[string]float64
	if err := json.Unmarshal(snap.IndexJSON, &idx); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"sh", "sz", "cyb"} {
		if _, ok := idx[k]; !ok {
			t.Errorf("index %s missing", k)
		}
	}
	// 首日仅 strong_yesterday 缺失
	if len(pending) != 1 || pending[0] != "strong_yesterday" {
		t.Fatalf("pending = %v, want [strong_yesterday]", pending)
	}
	// 情绪 detail 应有值
	if snap.EmotionState == nil || snap.EmotionScore == nil {
		t.Fatal("emotion state/score nil")
	}
}
