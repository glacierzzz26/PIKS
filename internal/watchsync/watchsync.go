// Package watchsync 把同花顺「我的自选」镜像进 PIKS(策略 + 落库编排)。
//
// 分层:
//
//	本文件(watchsync.go)= **纯策略**(Filter/Merge/Diff/DueSlot/SlotKey),
//	  不碰 DB、不碰网络 —— 全部可离线表驱动单测。
//	apply.go = 编排(取名 → 建实体 → 置状态 → 落价/日),依赖 store + 上游接口。
//
// 🔴 边界:本包**不 import internal/web**(web 拖 HTTP/AI 依赖,会把整包塞进 tools 镜像)。
// 🔴 只读上游:绝不调用任何同花顺写接口(见 internal/ths 包红线)。
// 🔴 成员资格真源 = entities.status('watch'),本包只**补**加入价/日到独立表。
package watchsync

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"piks/internal/store"
	"piks/internal/ths"
)

// ── 过滤:上游名单 → 可入 PIKS 的 A 股 ─────────────────────────────────────

// 丢弃原因(留档进 task_runs.meta.dropped[],便于解释「为什么少了 5 只」)。
const (
	ReasonNoCode     = "no_code"           // 上游没给 code
	ReasonNotStock   = "not_stock_code"    // 非 6 位数字(指数/自定义代码)
	ReasonMarketExcl = "market_not_ashare" // marketid 不在 A 股白名单(港美股/期货/指数)
)

// Dropped 被过滤掉的上游条目。
type Dropped struct {
	Code     string `json:"code,omitempty"`
	MarketID string `json:"market_id,omitempty"`
	Reason   string `json:"reason"`
}

// Filter 上游名单 → (保留的 A 股, 丢弃的带原因)。同 code 去重(后出现者胜,与
// trades.go 的 buildWatchPreview 同口径)。顺序保持上游序。
//
// 🔴 白名单只放 A 股权益类(ths.IsAShare):指数/港美股/期货一律挡在实体库之外
// —— 这是「不把 N225/KS11 灌进 entities」的唯一闸门。
func Filter(in []ths.SelfStock) (kept []ths.SelfStock, dropped []Dropped) {
	seen := map[string]bool{}
	for _, s := range in {
		code := store.NormalizeCode(strings.TrimSpace(s.Code))
		switch {
		case code == "":
			dropped = append(dropped, Dropped{Code: code, MarketID: s.MarketID, Reason: ReasonNoCode})
			continue
		case !store.IsStockCode(code):
			dropped = append(dropped, Dropped{Code: code, MarketID: s.MarketID, Reason: ReasonNotStock})
			continue
		case !ths.IsAShare(s.MarketID):
			dropped = append(dropped, Dropped{Code: code, MarketID: s.MarketID, Reason: ReasonMarketExcl})
			continue
		}
		if seen[code] {
			continue // 上游重复,去重
		}
		seen[code] = true
		kept = append(kept, ths.SelfStock{Code: code, MarketID: strings.TrimSpace(s.MarketID)})
	}
	return kept, dropped
}

// ── 合并:名单 × 元数据 → 待落库条目 ────────────────────────────────────────

// Entry 一只待同步的自选股(名单 + 加入价/日)。
type Entry struct {
	Code     string
	MarketID string
	Market   string     // SH/SZ/KC/CYB/BJ
	Price    *float64   // nil = 上游未给(不是 0)
	AddedOn  *time.Time // nil = 上游未给
	Raw      []byte     // 上游原文 {C,M,P,T},JSON 留档
}

// Merge 把 selfstock_detail 的价/日挂到名单上。**元数据缺失不致命** —— 该 code 仍
// 作为「在自选」同步,只是价/日为 NULL(detail 接口单独失败时的降级路径)。
func Merge(kept []ths.SelfStock, details []ths.Detail) (entries []Entry, priceMissing int) {
	byCode := make(map[string]ths.Detail, len(details))
	for _, d := range details {
		byCode[store.NormalizeCode(strings.TrimSpace(d.Code))] = d
	}
	entries = make([]Entry, 0, len(kept))
	for _, s := range kept {
		e := Entry{
			Code:     s.Code,
			MarketID: s.MarketID,
			Market:   ths.MarketAbbr(s.MarketID),
		}
		if d, ok := byCode[s.Code]; ok {
			e.Price, e.AddedOn = d.Price, d.AddedOn
			e.Raw = marshalRaw(d.Raw)
		}
		if e.Price == nil {
			priceMissing++ // ⚠️ 计数只为可见性:缺价是常态(上游未给),不是错误
		}
		entries = append(entries, e)
	}
	return entries, priceMissing
}

// ── 差量:现有自选 × 本轮快照 → 变更计划 ────────────────────────────────────

// 变更类型。
const (
	ChangeAdd    = "add"    // 上游有、本地无
	ChangeKeep   = "keep"   // 两边都有
	ChangeRemove = "remove" // 本地有、上游无(镜像移出)
)

// Existing 本地当前在自选的成员(来源 entities.status='watch')。
type Existing struct {
	Code     string
	EntityID string
}

// Change 一条变更。
type Change struct {
	Kind     string
	Code     string
	EntityID string // remove 时必填(置 archived)
	Entry    Entry  // add/keep 时的待落库条目
}

// Plan 一轮变更计划。
type Plan struct {
	Changes []Change
}

// Counts 按类型计数。
func (p Plan) Counts() (add, keep, remove int) {
	for _, c := range p.Changes {
		switch c.Kind {
		case ChangeAdd:
			add++
		case ChangeKeep:
			keep++
		case ChangeRemove:
			remove++
		}
	}
	return
}

// Diff 算变更。语义与交易截图导入 buildWatchPreview(internal/web/trades.go)同口径:
// **上游快照即权威** —— 本地有而上游无 = 移出(镜像语义,不做「保留本地新增」)。
//
// 顺序:add/keep 在前(按 code 升序),remove 在后(按 code 升序)—— 与截图导入一致,
// 便于人工核对同一套排序。
func Diff(existing []Existing, incoming []Entry) Plan {
	have := make(map[string]bool, len(existing))
	for _, x := range existing {
		have[x.Code] = true
	}
	var p Plan
	for _, e := range incoming {
		kind := ChangeAdd
		if have[e.Code] {
			kind = ChangeKeep
		}
		p.Changes = append(p.Changes, Change{Kind: kind, Code: e.Code, Entry: e})
	}
	incomingSet := make(map[string]bool, len(incoming))
	for _, e := range incoming {
		incomingSet[e.Code] = true
	}
	for _, x := range existing {
		if incomingSet[x.Code] {
			continue
		}
		p.Changes = append(p.Changes, Change{Kind: ChangeRemove, Code: x.Code, EntityID: x.EntityID})
	}
	sort.Slice(p.Changes, func(i, j int) bool {
		ri, rj := p.Changes[i].Kind == ChangeRemove, p.Changes[j].Kind == ChangeRemove
		if ri != rj {
			return !ri // add/keep 在前,remove 在后
		}
		return p.Changes[i].Code < p.Changes[j].Code
	})
	return p
}

// ── 时点:每日定点执行 + 重启去重 ──────────────────────────────────────────

// Slot 一个同步时点及其补跑宽容窗。
type Slot struct {
	At    string        // "HH:MM"(北京时间)
	Grace time.Duration // 超过 At+Grace 仍未跑 → 记 missed(不补陈旧快照)
}

// SlotKey 一个时点的当日唯一键,用于 task_runs.meta.slot 去重。
// 形如 "2026-09-22#09:00"。
func SlotKey(day, at string) string { return day + "#" + at }

// DueSlot 判断当前该跑哪个时点。语义:
//
//	now 早于某时点 → 该时点及之后都还没到,停止。
//	该时点已跑过(done=true)→ 跳过,继续看下一个。
//	已过时点 + 未超宽容窗 → **候选**(取最早的一个作为 due)。
//	已过时点 + 超窗 → 记入 missed(说明这段时间进程不在/一直在失败;不补跑,
//	  避免 14:00 补 09:00 的陈旧快照)。
//
// done 传判定函数(而非直接查库),使本函数保持纯函数、可离线单测。
// day 为北京时间日期 "2006-01-02";slots 需按时间升序。
func DueSlot(now time.Time, day string, slots []Slot, done func(slotKey string) bool) (due string, missed []string) {
	for _, sl := range slots {
		at, ok := parseHHMM(day, sl.At)
		if !ok {
			continue // 配置写错:跳过(启动时另做校验)
		}
		if now.Before(at) {
			break // 后续时点都还没到
		}
		key := SlotKey(day, sl.At)
		if done(key) {
			continue
		}
		if now.Sub(at) <= sl.Grace {
			if due == "" {
				due = key
			}
			continue
		}
		missed = append(missed, key)
	}
	return due, missed
}

// parseHHMM 把 "2006-01-02" + "HH:MM" 组成北京时间(东八区)的时刻。
func parseHHMM(day, hhmm string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02 15:04", day+" "+strings.TrimSpace(hhmm), cst)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// cst 北京时间固定偏移(与全项目各处一致;中国自 1991 年起无夏令时)。
var cst = time.FixedZone("CST", 8*3600)

// marshalRaw 把上游原始 detail 条目序列化留档(审计:能回答「当时的 P/T 原文是什么」)。
// 序列化失败返回 nil(留档失败不该阻断同步)。
func marshalRaw(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// BeijingDay 取 now 的北京日期串("2006-01-02")。
func BeijingDay(now time.Time) string { return now.In(cst).Format("2006-01-02") }

// ParseSlots 解析 "09:00,12:55,18:00" → []Slot。defaultGrace 为统一宽容窗;
// 末个时点(通常收盘后)用 tailGrace。格式非法 → error(启动即失败,不静默)。
func ParseSlots(spec string, defaultGrace, tailGrace time.Duration) ([]Slot, error) {
	var out []Slot
	for _, part := range strings.Split(spec, ",") {
		at := strings.TrimSpace(part)
		if at == "" {
			continue
		}
		if _, err := time.Parse("15:04", at); err != nil {
			return nil, fmt.Errorf("非法时点 %q(应形如 09:00): %w", at, err)
		}
		out = append(out, Slot{At: at, Grace: defaultGrace})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("时点列表为空")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	out[len(out)-1].Grace = tailGrace // 收盘后那轮容错更宽
	return out, nil
}
