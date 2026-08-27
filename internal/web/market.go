package web

// market.go 每日复盘/看板共享的市场快照视图模型与解析。
// JSONB 形状对齐 cmd/market-state 与 internal/publish/market.go(诚实解析,缺字段留空态)。

import (
	"encoding/json"
	"fmt"
	"sort"

	"piks/internal/model"
)

type KV struct {
	Name  string
	Count int
}

type IndexVM struct {
	Name, Code, Close, Pct string
}

// TopicVM 热点主题(涨停行业 top5 + 当日事件 top3)。
type TopicVM struct {
	Name     string
	Count    int
	EventIDs []string
}

type EmotionCompVM struct {
	Name    string
	Weight  int
	Score   int
	Missing bool
	Value   string
}

type SnapVM struct {
	Has bool // 快照存在

	Date string
	// 情绪
	Emotion string
	Score   string
	// 市场宽度
	LimitUp, LimitDown, Broken, MaxBoard        int
	BreadthAdvance, BreadthDecline, BreadthFlat int
	Turnover                                    string
	// 12 节
	Indexes         []IndexVM
	StrongYesterday string
	Industry        []KV
	MaxIndustry     int // 行业分布最大计数(条宽归一)
	HotTopics       []TopicVM
	TopEvents       []EventRefVM
	EmotionDetail   []EmotionCompVM
}

var idxNames = map[string]string{"sh": "上证指数", "sz": "深证成指", "cyb": "创业板指"}

// parseSnap 把 market_snapshots 行解析为视图模型。快照为 nil → Has=false(数据缺失如实标注)。
func parseSnap(snap *model.MarketSnapshot) SnapVM {
	vm := SnapVM{}
	if snap == nil {
		return vm
	}
	vm.Has = true
	vm.Date = snap.TradeDate.Format("2006-01-02")
	vm.Emotion = orStr(snap.EmotionState, "—")
	if snap.EmotionScore != nil {
		vm.Score = fmt.Sprintf("%.1f", *snap.EmotionScore)
	}
	vm.LimitUp = intOr(snap.LimitUpCount)
	vm.LimitDown = intOr(snap.LimitDownCount)
	vm.Broken = intOr(snap.BrokenLimitCount)
	vm.MaxBoard = intOr(snap.MaxBoard)
	if snap.TurnoverAmt != nil {
		vm.Turnover = fmt.Sprintf("%.1f 亿", *snap.TurnoverAmt)
	}

	var br struct {
		Advance int `json:"advance"`
		Decline int `json:"decline"`
		Flat    int `json:"flat"`
	}
	if json.Unmarshal(snap.Breadth, &br) == nil {
		vm.BreadthAdvance, vm.BreadthDecline, vm.BreadthFlat = br.Advance, br.Decline, br.Flat
	}

	var idx map[string]map[string]float64
	if json.Unmarshal(snap.IndexJSON, &idx) == nil {
		for _, k := range []string{"sh", "sz", "cyb"} {
			v, ok := idx[k]
			if !ok {
				continue
			}
			vm.Indexes = append(vm.Indexes, IndexVM{
				Name: idxNames[k], Code: k,
				Close: fmt.Sprintf("%.2f", v["close"]),
				Pct:   signPct(v["pct"]),
			})
		}
	}

	var sy struct {
		AvgRet float64 `json:"avg_ret"`
		Count  int     `json:"count"`
	}
	if json.Unmarshal(snap.StrongYesterday, &sy) == nil && sy.Count > 0 {
		vm.StrongYesterday = fmt.Sprintf("昨日 %d 只涨停股今日平均涨跌幅 %s", sy.Count, signPct(sy.AvgRet))
	}

	var dist map[string]int
	if json.Unmarshal(snap.IndustryDist, &dist) == nil {
		type kv struct {
			name  string
			count int
		}
		var ks []kv
		for n, c := range dist {
			ks = append(ks, kv{n, c})
		}
		sort.Slice(ks, func(i, j int) bool {
			if ks[i].count != ks[j].count {
				return ks[i].count > ks[j].count
			}
			return ks[i].name < ks[j].name
		})
		for i, k := range ks {
			if i >= 8 {
				break
			}
			if k.count > vm.MaxIndustry {
				vm.MaxIndustry = k.count
			}
			vm.Industry = append(vm.Industry, KV{Name: k.name, Count: k.count})
		}
	}

	var topics []map[string]any
	if json.Unmarshal(snap.HotTopics, &topics) == nil {
		for _, t := range topics {
			name, _ := t["name"].(string)
			if name == "" {
				continue
			}
			topic := TopicVM{Name: name}
			if c, ok := t["count"].(float64); ok {
				topic.Count = int(c)
			}
			if ids, ok := t["event_ids"].([]any); ok {
				for _, id := range ids {
					if s, ok := id.(string); ok {
						topic.EventIDs = append(topic.EventIDs, s)
					}
				}
			}
			vm.HotTopics = append(vm.HotTopics, topic)
		}
	}

	var evs []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if json.Unmarshal(snap.TopEvents, &evs) == nil {
		for _, ev := range evs {
			vm.TopEvents = append(vm.TopEvents, EventRefVM{ev.ID, ev.Title})
		}
	}

	var detail map[string]struct {
		Weight  int    `json:"weight"`
		Score   int    `json:"score"`
		Missing bool   `json:"missing"`
		Value   any    `json:"value"`
	}
	if json.Unmarshal(snap.EmotionDetail, &detail) == nil {
		for _, name := range []string{"limit_up", "limit_down", "breadth_ratio", "broken_rate", "max_board", "strong_yesterday", "industry_count"} {
			c, ok := detail[name]
			if !ok {
				continue
			}
			val := "—"
			if c.Value != nil {
				val = fmt.Sprintf("%v", c.Value)
			}
			vm.EmotionDetail = append(vm.EmotionDetail, EmotionCompVM{Name: name, Weight: c.Weight, Score: c.Score, Missing: c.Missing, Value: val})
		}
	}
	return vm
}

func intOr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func signPct(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+%.2f%%", v)
	}
	return fmt.Sprintf("%.2f%%", v)
}
