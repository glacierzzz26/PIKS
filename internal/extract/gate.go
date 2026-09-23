package extract

// 抽取成本粗筛(issue #83 分期 P-4 / 原 P7):在 **raw 层、预去重之后、送 LLM 之前**,
// 按「源自带信号」对候选文档排序并取 Top N,把明显不值得花的文档挡在 LLM 之外。
//
// 为什么需要:多源后单日入库 ~180 条(issue #68 C 层盘中采集后更多),而抽取是**每条都调 LLM**。
// 预算护栏(ai_daily_token_budget)是**总量**闸,粗筛是**优先级**闸 —— 预算用完时先跑谁。
//
// ⚠️ **粗筛不得替代预算护栏**:预算是硬上限(钱的正源),粗筛只是「预算不够时先给谁」。
// 二者正交,缺一不可(issue P7)。
//
// ⚠️ **默认保守**(越激进越危险):本版门控取到**只挡明显低价值**的一档 ——
// 缺信号(extra 为空/无任何排序信号)的文档**一律保留**(宁可多花,不可漏重要的)。
// 只有「信号明确且排在末位、又超过当日配额」的才落 deferred。

import (
	"encoding/json"
	"sort"
	"time"
)

// RawGateInput 粗筛所需的最小投影(raw_documents 一行)。
type RawGateInput struct {
	ID          string
	Extra       json.RawMessage // 上游原始字段(important/confirmed/level/reading_num/like_nums)
	RetrievedAt time.Time
}

// GateSignals 从 extra 解析出的排序信号(全部**可选** —— 缺失即 0/空,视为「无信号」)。
//
// ⚠️ 信号来自**各源自带字段**(issue #43 原样落 extra):
//   - 金十:important=1 / confirmed=1(权威重要度)
//   - 财联社:level ∈ {A,B,C}(A 最高)、confirmed=1、reading_num(阅读数)
//   - 富途:level(0=普通)
//   - 新浪:like_nums(点赞,弱热度)
type GateSignals struct {
	Important  bool  // extra.important == 1
	Confirmed  bool  // extra.confirmed == 1
	LevelRank  int   // 源自有分级归一化排名(higher = 更重要);0 = 无分级/未知
	ReadingNum int64 // 财联社 reading_num
	LikeNums   int64 // 新浪 like_nums
}

// parseGateSignals 从 extra 解析信号。容错:非法 JSON / 类型不符一律当「无信号」,不 panic、不报错。
//
// ⚠️ `important`/`confirmed` 在库里可能是数字(金十)也可能是字符串(部分源),
// 用 `json.Number` 宽松承接,再看是否 == 1。
func parseGateSignals(extra json.RawMessage) GateSignals {
	var s GateSignals
	if len(extra) == 0 {
		return s
	}
	var m map[string]any
	if err := json.Unmarshal(extra, &m); err != nil {
		return s
	}
	s.Important = numIsOne(m["important"])
	s.Confirmed = numIsOne(m["confirmed"])
	if lv, ok := m["level"]; ok {
		s.LevelRank = levelRank(lv)
	}
	if v, ok := numOf(m["reading_num"]); ok {
		s.ReadingNum = v
	}
	if v, ok := numOf(m["like_nums"]); ok {
		s.LikeNums = v
	}
	return s
}

// numIsOne 判定 JSON 值是否为数值 1(兼容 "1" 字符串)。
func numIsOne(v any) bool {
	n, ok := numOf(v)
	return ok && n == 1
}

// numOf 宽松取数值:接受 float64 / json.Number / 字符串数字。非法返回 (0,false)。
func numOf(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case json.Number:
		n, err := t.Int64()
		return n, err == nil
	case string:
		if t == "" {
			return 0, false
		}
		var n int64
		if err := json.Unmarshal([]byte(t), &n); err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// levelRank 把源自有分级映射为「越大越重要」的排名(不同源量纲不同,只做**相对**排序):
// 财联社 A/B/C → 3/2/1;富途 level 数值 → 其值(0=普通,A股场景下 1/2 为更高);
// 其余(数字/未知)→ 尽力取数值,取不到则 0。
func levelRank(v any) int {
	if s, ok := v.(string); ok {
		switch s {
		case "A", "a":
			return 3
		case "B", "b":
			return 2
		case "C", "c":
			return 1
		default:
			return 0
		}
	}
	if n, ok := numOf(v); ok {
		return int(n)
	}
	return 0
}

// GateConfig 粗筛参数。
//
// ⚠️ **默认保守**:`DeferLowSignal=false`、`MinKeepRatio=1.0`。即默认**不筛任何东西**
// (只做「必送优先」的排序,不落 deferred),除非运维显式收紧。
// 这样即使新上线也**不会**因为一个激进的默认值把重要消息挡在 LLM 之外。
type GateConfig struct {
	// MustSend 必送:`important=1` 或 `confirmed=1`(源权威标记),任何情况下优先并保留。
	MustSend bool
	// Window 时间窗:cross 层的兜底截止 —— 比 `Now-Window` 更早、且非必送的文档落 deferred,
	// 防「久不跑 → 一大坨一次送」。Window<=0 = 不限(默认)。
	Window time.Duration
	// Now 判定基准时刻(便于测试注入)。
	Now time.Time
	// Max 本轮最多保留(送 LLM)的条数;<=0 = 不限。
	Max int
	// DeferLowSignal:为 true 时,超出 Max 的「无任何信号」文档落 deferred;
	// false(默认)= 超出的也保留(只排序不筛)。
	DeferLowSignal bool
}

// GateResult 粗筛结果。
type GateResult struct {
	Keep     []RawGateInput    // 送 LLM(按优先级排序:必送 → 分级 → 阅读数 → 时间新)
	Deferred []string          // 落 deferred 的 id(附带原因见 GateReasons)
	Reasons  map[string]string // id → 落 deferred 的原因(记账用)
}

// Gate 按 GateConfig 对候选文档排序并分流(纯函数,可离线校准)。
//
// 排序键(降序优先级):
//  1. 必送(important/confirmed);
//  2. LevelRank(源自有分级);
//  3. ReadingNum(财联社阅读数);无则 LikeNums(新浪点赞);
//  4. RetrievedAt(越新越靠前)。
//
// 落 deferred 的条件(仅当 DeferLowSignal 且 Max>0):
//   - 排名在 Max 之外 **且** 无任何信号(既非必送、无分级、无阅读/点赞)—— 即「看不出价值」的;
//   - 或超过 Window 的老文档(非必送)。
//
// ⚠️ 有信号的文档即便超 Max 也**不**落 deferred(返 KEEP 顺序外的仍送入,由预算闸兜底)——
// 粗筛只淘汰「明显不值得」的,不淘汰「只是排后面」的。
func Gate(docs []RawGateInput, cfg GateConfig) GateResult {
	res := GateResult{Reasons: map[string]string{}}
	if len(docs) == 0 {
		return res
	}
	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}
	type cand struct {
		in     RawGateInput
		sig    GateSignals
		must   bool
		hasSig bool
	}
	cands := make([]cand, len(docs))
	for i, d := range docs {
		sig := parseGateSignals(d.Extra)
		must := cfg.MustSend && (sig.Important || sig.Confirmed)
		hasSig := sig.Important || sig.Confirmed || sig.LevelRank > 0 || sig.ReadingNum > 0 || sig.LikeNums > 0
		cands[i] = cand{in: d, sig: sig, must: must, hasSig: hasSig}
	}
	// 稳定排序(降序)。
	sort.SliceStable(cands, func(a, b int) bool {
		ca, cb := cands[a], cands[b]
		if ca.must != cb.must {
			return ca.must
		}
		if ca.sig.LevelRank != cb.sig.LevelRank {
			return ca.sig.LevelRank > cb.sig.LevelRank
		}
		ra := ca.sig.ReadingNum
		if ra == 0 {
			ra = ca.sig.LikeNums
		}
		rb := cb.sig.ReadingNum
		if rb == 0 {
			rb = cb.sig.LikeNums
		}
		if ra != rb {
			return ra > rb
		}
		return ca.in.RetrievedAt.After(cb.in.RetrievedAt)
	})

	for idx, c := range cands {
		overMax := cfg.Max > 0 && idx >= cfg.Max
		stale := false
		if cfg.Window > 0 && !c.must {
			stale = c.in.RetrievedAt.Before(now.Add(-cfg.Window))
		}
		// 落 deferred:仅当显式开启 DeferLowSignal;**且**「超配额无信号」或「过期非必送」。
		if cfg.DeferLowSignal && (stale || (overMax && !c.hasSig)) {
			reason := "超当日配额且无源信号(重要度/分级/阅读数)"
			if stale {
				reason = "超过时间窗且非必送消息"
			}
			res.Deferred = append(res.Deferred, c.in.ID)
			res.Reasons[c.in.ID] = reason
			continue
		}
		res.Keep = append(res.Keep, c.in)
	}
	return res
}
