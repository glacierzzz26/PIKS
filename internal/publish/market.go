// market 每日复盘页渲染(迭代 2,设计 §3.3)。02-Market/ 属 Generated 分域,重复运行覆盖。
// 12 节按架构 §22.1 每日复盘框架;「我的判断」只留提示占位(分域 D17,个人内容在 09-Personal)。
package publish

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"piks/internal/model"
)

// MarketPath 复盘页路径:02-Market/YYYY-MM-DD.md
func MarketPath(vault string, day time.Time) string {
	return filepath.Join(vault, "02-Market", day.Format("2006-01-02")+".md")
}

// RenderMarket 渲染每日复盘页。pipeline 传 "market-state@<git-short>"。
func RenderMarket(snap *model.MarketSnapshot, pipeline string) string {
	var b strings.Builder
	day := snap.TradeDate.Format("2006-01-02")
	emotion := strPtr(snap.EmotionState)

	fmt.Fprintf(&b, "---\nid: market-%s\ntype: market-daily\ndate: %s\n", day, day)
	fmt.Fprintf(&b, "emotion: %s\n", emotion)
	if pipeline != "" {
		fmt.Fprintf(&b, "pipeline: %s\n", pipeline)
	}
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# 每日复盘 %s\n\n", day)

	// 1 指数
	b.WriteString("## 指数\n")
	renderIndexes(&b, snap.IndexJSON)

	// 2 成交额
	b.WriteString("\n## 成交额\n")
	if snap.TurnoverAmt != nil {
		fmt.Fprintf(&b, "两市 %.1f 亿\n", *snap.TurnoverAmt)
	} else {
		b.WriteString("_数据缺失(pending,源未核验)_\n")
	}

	// 3 涨跌家数
	b.WriteString("\n## 涨跌家数\n")
	if len(snap.Breadth) > 0 {
		var br struct {
			Advance int `json:"advance"`
			Decline int `json:"decline"`
			Flat    int `json:"flat"`
		}
		if json.Unmarshal(snap.Breadth, &br) == nil {
			fmt.Fprintf(&b, "上涨 %d / 下跌 %d / 平盘 %d\n", br.Advance, br.Decline, br.Flat)
		}
	} else {
		b.WriteString("_数据缺失(pending,源未核验)_\n")
	}

	// 4 涨停/跌停/炸板
	b.WriteString("\n## 涨停/跌停/炸板\n")
	limitUp := intOr(snap.LimitUpCount)
	limitDown := intOr(snap.LimitDownCount)
	broken := intOr(snap.BrokenLimitCount)
	fmt.Fprintf(&b, "涨停 %d / 跌停 %d / 炸板 %d\n", limitUp, limitDown, broken)

	// 5 连板高度
	b.WriteString("\n## 连板高度\n")
	fmt.Fprintf(&b, "%d 连板\n", intOr(snap.MaxBoard))

	// 6 昨日强势股表现
	b.WriteString("\n## 昨日强势股表现\n")
	renderStrongYesterday(&b, snap.StrongYesterday)

	// 7 行业表现(涨停分布)
	b.WriteString("\n## 行业表现(涨停分布)\n")
	renderIndustry(&b, snap.IndustryDist)

	// 8 热点
	b.WriteString("\n## 热点\n")
	renderHotTopics(&b, snap.HotTopics)

	// 9 市场情绪
	b.WriteString("\n## 市场情绪\n")
	renderEmotion(&b, snap)

	// 10 资金
	b.WriteString("\n## 资金\n")
	b.WriteString("_源待定,本期留空_\n")

	// 11 重要事件
	b.WriteString("\n## 重要事件\n")
	renderTopEvents(&b, snap.TopEvents)

	// 12 我的判断
	b.WriteString("\n## 我的判断\n")
	fmt.Fprintf(&b, "> 个人判断请写入 `09-Personal/复盘/%s.md`(分域双源:Generated 不承载个人内容,可 wikilink 互链)\n", day)

	return b.String()
}

func renderIndexes(b *strings.Builder, raw []byte) {
	var idx map[string]map[string]float64
	if json.Unmarshal(raw, &idx) != nil || len(idx) == 0 {
		b.WriteString("_数据缺失(pending,源未核验)_\n")
		return
	}
	names := map[string]string{"sh": "上证指数", "sz": "深证成指", "cyb": "创业板指"}
	b.WriteString("| 指数 | 收盘 | 涨跌幅 |\n| --- | --- | --- |\n")
	for _, k := range []string{"sh", "sz", "cyb"} {
		v, ok := idx[k]
		if !ok {
			continue
		}
		fmt.Fprintf(b, "| %s | %.2f | %s |\n", names[k], v["close"], signPct(v["pct"]))
	}
}

func renderStrongYesterday(b *strings.Builder, raw []byte) {
	if len(raw) == 0 {
		b.WriteString("_缺失(无昨日数据或行情未获取)_\n")
		return
	}
	var sy struct {
		AvgRet float64 `json:"avg_ret"`
		Count  int     `json:"count"`
	}
	if json.Unmarshal(raw, &sy) != nil || sy.Count == 0 {
		b.WriteString("_缺失(无昨日数据或行情未获取)_\n")
		return
	}
	fmt.Fprintf(b, "昨日 %d 只涨停股今日平均涨跌幅 %s\n", sy.Count, signPct(sy.AvgRet))
}

func renderIndustry(b *strings.Builder, raw []byte) {
	var dist map[string]int
	if json.Unmarshal(raw, &dist) != nil || len(dist) == 0 {
		b.WriteString("_无涨停或无行业标签_\n")
		return
	}
	type kv struct {
		name  string
		count int
	}
	var ks []kv
	for n, c := range dist {
		ks = append(ks, kv{n, c})
	}
	sort.Slice(ks, func(i, j int) bool {
		// count 降序,name 升序作次级键,保证确定性(同 count 时 Go map 随机序 → 幂等破坏)
		if ks[i].count != ks[j].count {
			return ks[i].count > ks[j].count
		}
		return ks[i].name < ks[j].name
	})
	for i, k := range ks {
		if i >= 8 {
			break
		}
		fmt.Fprintf(b, "- %s ×%d\n", k.name, k.count)
	}
}

func renderHotTopics(b *strings.Builder, raw []byte) {
	var topics []map[string]any
	if json.Unmarshal(raw, &topics) != nil || len(topics) == 0 {
		b.WriteString("_无_\n")
		return
	}
	for _, t := range topics {
		name, _ := t["name"].(string)
		if name == "" {
			continue
		}
		fmt.Fprintf(b, "- %s", name)
		if count, ok := t["count"].(float64); ok {
			fmt.Fprintf(b, "(涨停 %d 家)", int(count))
		}
		if ids, ok := t["event_ids"].([]any); ok {
			for _, id := range ids {
				fmt.Fprintf(b, " [[event-%s]]", shortID(fmt.Sprint(id)))
			}
		}
		b.WriteString("\n")
	}
}

func renderEmotion(b *strings.Builder, snap *model.MarketSnapshot) {
	emotion := "未知"
	if snap.EmotionState != nil {
		emotion = *snap.EmotionState
	}
	score := 0.0
	if snap.EmotionScore != nil {
		score = *snap.EmotionScore
	}
	fmt.Fprintf(b, "**%s (%.1f 分)**。规则加权,非交易信号(架构 §9.8)。\n\n", emotion, score)
	b.WriteString("| 组件 | 权重 | 分值 | 值 |\n| --- | --- | --- | --- |\n")
	var detail map[string]struct {
		Weight  int `json:"weight"`
		Score   int `json:"score"`
		Missing bool `json:"missing"`
		Value   any `json:"value"`
	}
	if json.Unmarshal(snap.EmotionDetail, &detail) == nil {
		order := []string{"limit_up", "limit_down", "breadth_ratio", "broken_rate", "max_board", "strong_yesterday", "industry_count"}
		for _, name := range order {
			c, ok := detail[name]
			if !ok {
				continue
			}
			val := "—"
			if c.Value != nil {
				val = fmt.Sprintf("%v", c.Value)
			}
			scoreStr := "missing"
			if !c.Missing {
				scoreStr = fmt.Sprintf("%d", c.Score)
			}
			fmt.Fprintf(b, "| %s | %d | %s | %s |\n", name, c.Weight, scoreStr, val)
		}
	}
}

func renderTopEvents(b *strings.Builder, raw []byte) {
	var evs []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if json.Unmarshal(raw, &evs) == nil && len(evs) > 0 {
		for _, ev := range evs {
			fmt.Fprintf(b, "- [[event-%s]] %s\n", shortID(ev.ID), ev.Title)
		}
		return
	}
	b.WriteString("_当日无事件_\n")
}

// signPct 带符号的百分比(+0.59%)。
func signPct(v float64) string {
	if v >= 0 {
		return fmt.Sprintf("+%.2f%%", v)
	}
	return fmt.Sprintf("%.2f%%", v)
}

func strPtr(s *string) string {
	if s == nil {
		return "-"
	}
	return *s
}

func intOr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
