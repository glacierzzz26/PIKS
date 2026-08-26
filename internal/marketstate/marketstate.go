// Package marketstate 每日市场状态聚合 + 情绪规则模型(迭代 2,设计 §2.2/§3.2)。
// 纯规则加权,零 AI 调用。可离线单测(§5.4 极端输入、§5.5 两日计算)。
package marketstate

import (
	"encoding/json"
	"strconv"
	"time"

	"piks/internal/collector"
	"piks/internal/model"
)

// 指标字典(设计 §2.3),与 cmd/quote-collector 写入 observations 的 indicator 一致。
const (
	IndLimitUpCount   = "limit_up_count"
	IndLimitDownCount = "limit_down_count"
	IndBrokenLimit    = "broken_limit_count"
	IndMaxBoard       = "max_board"
	IndZTPool         = "zt_pool"
	IndIndustryDist   = "industry_dist"
	IndTurnover       = "market_turnover"
	IndBreadthAdv     = "breadth_advance"
	IndBreadthDec     = "breadth_decline"
	IndBreadthFlat    = "breadth_flat"
	IndIndexClose     = "index_close"
	IndIndexPct       = "index_pct"
)

// 情绪组件名(emotion_detail 键,设计 §2.2 表)。
const (
	CompLimitUp    = "limit_up"
	CompLimitDown  = "limit_down"
	CompBreadth    = "breadth_ratio"
	CompBrokenRate = "broken_rate"
	CompMaxBoard   = "max_board"
	CompStrongYest = "strong_yesterday"
	CompIndustry   = "industry_count"
)

// 组件权重(设计 §2.2)。
var weights = map[string]int{
	CompLimitUp:    2,
	CompLimitDown:  2,
	CompBreadth:    3,
	CompBrokenRate: 2,
	CompMaxBoard:   2,
	CompStrongYest: 2,
	CompIndustry:   1,
}

// Breadth 涨跌家数(与 collector.Breadth 同构,避免包耦合)。
type Breadth struct {
	Advance int `json:"advance"`
	Decline int `json:"decline"`
	Flat    int `json:"flat"`
}

// EmotionInput 情绪模型输入。可空项(nil)对应组件记 missing,不影响可用组件得分。
type EmotionInput struct {
	LimitUp       int      // 涨停家数
	LimitDown     int      // 跌停家数
	Breadth       *Breadth // 涨跌家数(nil=未获取)
	BrokenRate    *float64 // 炸板率 broken/(broken+limit_up),0~1(nil=无数据)
	MaxBoard      int      // 连板高度
	StrongAvg     *float64 // 昨日涨停今日涨跌幅均值%,2 位小数(nil=无昨日数据)
	IndustryCount *int     // 出现涨停的行业数(nil=行业分布缺失)
}

// EmotionComponent emotion_detail 单项。
type EmotionComponent struct {
	Weight  int `json:"weight"`
	Score   int `json:"score"`
	Missing bool `json:"missing,omitempty"`
	Value   any `json:"value,omitempty"`
}

// EmotionResult 情绪模型输出。
type EmotionResult struct {
	Score  float64
	State  string
	Detail json.RawMessage
}

// ComputeEmotion 逐组件打分加权求和 → score/state/detail(设计 §2.2 表)。
// missing 组件不参与求和;detail 里保留 missing 标记以可解释。
func ComputeEmotion(in EmotionInput) EmotionResult {
	comp := map[string]*EmotionComponent{}
	add := func(name string, score *int, value any) {
		c := &EmotionComponent{Weight: weights[name], Missing: score == nil, Value: value}
		if score != nil {
			c.Score = *score
		}
		comp[name] = c
	}

	// 涨停数: ≥80:3 / 40~79:2 / 20~39:1 / 5~19:0 / ≤4:-1
	s := 2
	switch {
	case in.LimitUp >= 80:
		s = 3
	case in.LimitUp >= 40:
		s = 2
	case in.LimitUp >= 20:
		s = 1
	case in.LimitUp >= 5:
		s = 0
	default:
		s = -1
	}
	add(CompLimitUp, &s, in.LimitUp)

	// 跌停数: 0:2 / 1~5:0 / ≥6:-2
	s = 2
	switch {
	case in.LimitDown == 0:
		s = 2
	case in.LimitDown <= 5:
		s = 0
	default:
		s = -2
	}
	add(CompLimitDown, &s, in.LimitDown)

	// 涨跌家数比: ≥.75:3 / ≥.6:2 / ≥.45:1 / ≥.4:0 / ≥.3:-1 / ≥.15:-2 / <.15:-3
	if in.Breadth != nil {
		tot := in.Breadth.Advance + in.Breadth.Decline
		if tot > 0 {
			r := float64(in.Breadth.Advance) / float64(tot)
			s := 3
			switch {
			case r >= 0.75:
				s = 3
			case r >= 0.6:
				s = 2
			case r >= 0.45:
				s = 1
			case r >= 0.4:
				s = 0
			case r >= 0.3:
				s = -1
			case r >= 0.15:
				s = -2
			default:
				s = -3
			}
			add(CompBreadth, &s, round2(r))
		} else {
			add(CompBreadth, nil, nil)
		}
	} else {
		add(CompBreadth, nil, nil)
	}

	// 炸板率: <.2:1 / ≤.4:0 / >.4:-2
	if in.BrokenRate != nil {
		r := *in.BrokenRate
		s := 0
		switch {
		case r < 0.2:
			s = 1
		case r <= 0.4:
			s = 0
		default:
			s = -2
		}
		add(CompBrokenRate, &s, round2(r))
	} else {
		add(CompBrokenRate, nil, nil)
	}

	// 连板高度: ≥5:3 / 3~4:2 / 2:1 / 1:0(无涨停 m=0 同 0 分)
	s = 0
	switch {
	case in.MaxBoard >= 5:
		s = 3
	case in.MaxBoard >= 3:
		s = 2
	case in.MaxBoard >= 2:
		s = 1
	default:
		s = 0
	}
	add(CompMaxBoard, &s, in.MaxBoard)

	// 昨日涨停今表现: ≥3:2 / ≥0:1 / ≥-2:0 / <-2:-2
	if in.StrongAvg != nil {
		r := *in.StrongAvg
		s := 0
		switch {
		case r >= 3:
			s = 2
		case r >= 0:
			s = 1
		case r >= -2:
			s = 0
		default:
			s = -2
		}
		add(CompStrongYest, &s, round2(r))
	} else {
		add(CompStrongYest, nil, nil)
	}

	// 涨停行业数: ≥10:1 / 5~9:0 / <5:-1
	if in.IndustryCount != nil {
		n := *in.IndustryCount
		s := 0
		switch {
		case n >= 10:
			s = 1
		case n >= 5:
			s = 0
		default:
			s = -1
		}
		add(CompIndustry, &s, n)
	} else {
		add(CompIndustry, nil, nil)
	}

	var score float64
	for _, c := range comp {
		if !c.Missing {
			score += float64(c.Score * c.Weight)
		}
	}
	detail, _ := json.Marshal(comp)
	return EmotionResult{Score: score, State: stateFromScore(score), Detail: detail}
}

// 得分 → 枚举(设计 §2.2 表)。
func stateFromScore(s float64) string {
	switch {
	case s >= 20:
		return "Extreme_Greed"
	case s >= 12:
		return "Strong"
	case s >= 6:
		return "Warm"
	case s >= -2:
		return "Neutral"
	case s >= -8:
		return "Weak"
	case s >= -14:
		return "Fear"
	default:
		return "Extreme_Fear"
	}
}

// ComputeSnapshot 由当日 observations + 昨日快照构造 market_snapshots 派生字段。
// 不填 top_events/hot_topics/evidence(由 cmd 从 store 补充)。
// 返回 snapshot 与 pending 缺失项(诚实标记,不造假)。
func ComputeSnapshot(day time.Time, obs []model.Observation, prev *model.MarketSnapshot, prevReturns map[string]float64) (*model.MarketSnapshot, []string) {
	byKey := make(map[string]string)
	for _, o := range obs {
		byKey[o.Market+"/"+o.Indicator] = o.Value
	}
	val := func(market, ind string) (string, bool) {
		v, ok := byKey[market+"/"+ind]
		return v, ok
	}

	var pending []string
	snap := &model.MarketSnapshot{TradeDate: day}

	// 1 指数
	idx := map[string]any{}
	for _, k := range []string{"sh", "sz", "cyb"} {
		c, ok1 := val(k, IndIndexClose)
		p, ok2 := val(k, IndIndexPct)
		if ok1 && ok2 {
			cf, e1 := strconv.ParseFloat(c, 64)
			pf, e2 := strconv.ParseFloat(p, 64)
			if e1 == nil && e2 == nil {
				idx[k] = map[string]float64{"close": cf, "pct": pf}
				continue
			}
		}
		pending = append(pending, "index_"+k)
	}
	if len(idx) > 0 {
		snap.IndexJSON = mustJSON(idx)
	}

	// 2 成交额
	if v, ok := val("all", IndTurnover); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			snap.TurnoverAmt = &f
		}
	} else {
		pending = append(pending, "turnover")
	}

	// 3 涨跌家数
	if a, ok1 := val("all", IndBreadthAdv); ok1 {
		if d, ok2 := val("all", IndBreadthDec); ok2 {
			if f, ok3 := val("all", IndBreadthFlat); ok3 {
				ai, _ := strconv.Atoi(a)
				di, _ := strconv.Atoi(d)
				fi, _ := strconv.Atoi(f)
				b := Breadth{Advance: ai, Decline: di, Flat: fi}
				snap.Breadth = mustJSON(b)
			}
		}
	} else {
		pending = append(pending, "breadth")
	}

	// 4 涨跌停炸板
	if v, ok := val("all", IndLimitUpCount); ok {
		if n, err := strconv.Atoi(v); err == nil {
			snap.LimitUpCount = &n
		}
	} else {
		pending = append(pending, "limit_up")
	}
	if v, ok := val("all", IndLimitDownCount); ok {
		if n, err := strconv.Atoi(v); err == nil {
			snap.LimitDownCount = &n
		}
	} else {
		pending = append(pending, "limit_down")
	}
	if v, ok := val("all", IndBrokenLimit); ok {
		if n, err := strconv.Atoi(v); err == nil {
			snap.BrokenLimitCount = &n
		}
	} else {
		pending = append(pending, "broken")
	}

	// 5 连板高度
	if v, ok := val("all", IndMaxBoard); ok {
		if n, err := strconv.Atoi(v); err == nil {
			snap.MaxBoard = &n
		}
	} else {
		pending = append(pending, "max_board")
	}

	// 4 涨停池(供昨日强势股次日回看)
	if v, ok := val("all", IndZTPool); ok {
		var pool []collector.ZTItem
		if err := json.Unmarshal([]byte(v), &pool); err == nil {
			snap.ZTPool = mustJSON(pool)
		}
	}

	// 7 行业分布
	if v, ok := val("all", IndIndustryDist); ok {
		var dist map[string]int
		if err := json.Unmarshal([]byte(v), &dist); err == nil {
			snap.IndustryDist = mustJSON(dist)
		}
	}

	// 6 昨日强势股表现(昨日涨停今日涨跌幅均值;prevReturns 由 cmd 尽力获取)
	if len(prevReturns) > 0 {
		var sum float64
		for _, r := range prevReturns {
			sum += r
		}
		avg := sum / float64(len(prevReturns))
		snap.StrongYesterday = mustJSON(map[string]any{"avg_ret": round2(avg), "count": len(prevReturns)})
	} else {
		pending = append(pending, "strong_yesterday")
	}

	// 9 市场情绪
	snap.EmotionScore, snap.EmotionState, snap.EmotionDetail = computeEmotionFields(byKey, prevReturns)

	// 10 资金(源待定,如实留空);12 我的判断(预留,不自动写)
	return snap, pending
}

// computeEmotionFields 由观测组装 EmotionInput → score/state/detail。
func computeEmotionFields(byKey map[string]string, prevReturns map[string]float64) (*float64, *string, json.RawMessage) {
	var in EmotionInput
	if v, ok := byKey["all/"+IndLimitUpCount]; ok {
		in.LimitUp, _ = strconv.Atoi(v)
	}
	if v, ok := byKey["all/"+IndLimitDownCount]; ok {
		in.LimitDown, _ = strconv.Atoi(v)
	}
	if a, ok1 := byKey["all/"+IndBreadthAdv]; ok1 {
		if d, ok2 := byKey["all/"+IndBreadthDec]; ok2 {
			ai, _ := strconv.Atoi(a)
			di, _ := strconv.Atoi(d)
			in.Breadth = &Breadth{Advance: ai, Decline: di}
		}
	}
	if lu, ok1 := byKey["all/"+IndLimitUpCount]; ok1 {
		if br, ok2 := byKey["all/"+IndBrokenLimit]; ok2 {
			lui, _ := strconv.Atoi(lu)
			bri, _ := strconv.Atoi(br)
			if lui+bri > 0 {
				r := float64(bri) / float64(lui+bri)
				in.BrokenRate = &r
			}
		}
	}
	if v, ok := byKey["all/"+IndMaxBoard]; ok {
		in.MaxBoard, _ = strconv.Atoi(v)
	}
	if len(prevReturns) > 0 {
		var sum float64
		for _, r := range prevReturns {
			sum += r
		}
		avg := round2(sum / float64(len(prevReturns)))
		in.StrongAvg = &avg
	}
	if v, ok := byKey["all/"+IndIndustryDist]; ok {
		var dist map[string]int
		if err := json.Unmarshal([]byte(v), &dist); err == nil {
			n := len(dist)
			in.IndustryCount = &n
		}
	}
	res := ComputeEmotion(in)
	return &res.Score, &res.State, res.Detail
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}
