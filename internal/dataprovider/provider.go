// Package dataprovider —— 研究平台的**数据源契约**（phase13 §3.3 逐字实现，随 P14-1 交付）。
//
// 定位：**能力导向，非源导向**。分析师按 Capability 取数，不知道背后是 datahub(MCP)、
// 免费源(akshare/已有链路) 还是本地 PG 投影。三个实现者：
//
//   - McpProvider       —— MCP client 连 datahub `POST /mcp`（本轮交付，主要实现）
//   - FreeSourceProvider—— 免费源兜底 + 事件/公告/热榜（已有链路归位，占位）
//   - LocalPGProvider   —— 已有投影（events / market_snapshots / 研报历史，占位）
//
// ⚠️ **本轮不接线到任何管线**（phase13 P13-2+ 才会有生产消费者）：P14-1 只交付
// 「接口 + 能力枚举 + McpProvider + 冒烟测试」。research 的 Python 取数已另走 HTTP
// （见 research/src/providers/factory.py），此 Go 框架是分析师/编排的**接口契约**。
package dataprovider

import (
	"context"
	"errors"
	"time"
)

// Capability 声明数据能力（照 phase13 §3.3 逐字）。
type Capability string

const (
	CapQuoteSeries        Capability = "quote_series"        // 行情序列（日/分钟）
	CapFinancials         Capability = "financials"          // 财报（历史 + 同比）
	CapValuationHistory   Capability = "valuation_history"   // PE/PB 等历史分位
	CapCapitalFlow        Capability = "capital_flow"        // 主力/北向/两融/龙虎榜
	CapIndustryMembers    Capability = "industry_members"    // 行业成分
	CapIndustryFinancials Capability = "industry_financials" // 行业财务聚合
	CapMacro              Capability = "macro"               // 宏观指标
	CapMarketBreadth      Capability = "market_breadth"      // 涨跌家数/涨停/成交额
	CapEvents             Capability = "events"              // 事件（已有链路）
	CapAnnouncements      Capability = "announcements"       // 公告（已有链路）
	CapHotTopics          Capability = "hot_topics"          // 热榜（已有链路）
)

// ErrUnsupported 该 Provider 不支持所请求的能力（路由据此换源，非致命）。
var ErrUnsupported = errors.New("dataprovider: 该 Provider 不支持此能力")

// ErrNotWired 占位实现的显式信号 —— **绝不返回假数据**：调用方据此如实标
// `数据不可得`，而不是静默得到空壳。
var ErrNotWired = errors.New("dataprovider: 占位实现，未接线（phase13 P13-2+ 落）")

// Provider 数据源契约。
//
// 编排按「主体覆盖面 + 数据新鲜度 + 可得性」选源：`Supports` 声明能力，`Fetch` 取数；
// 主源缺该票（如中小盘不在覆盖内）→ 降级兜底源 → 都无 → 如实标 `数据不可得`，
// **不静默、不造数**（phase13 §3.3「路由与诚实」）。
type Provider interface {
	Name() string
	Supports(cap Capability) bool
	Fetch(ctx context.Context, req FetchRequest) (Result, error)
}

// SubjectKind 主体类型（phase13 §3.1）。
type SubjectKind string

const (
	SubjectStock    SubjectKind = "stock"
	SubjectIndustry SubjectKind = "industry"
	SubjectMarket   SubjectKind = "market"
	SubjectMacro    SubjectKind = "macro"
	SubjectTopic    SubjectKind = "topic"
)

// Subject 研究主体。`AsOf` 是**硬字段**：任何序列数据按此日截断，禁止取到基准日
// 之后的数据（防未来函数）。
type Subject struct {
	Kind SubjectKind
	ID   string // "600519" / "sw/801080" / "A" / "macro/cn" / "topic/hot-ai"
	Name string
	AsOf time.Time
}

// FetchRequest 一次取数请求。区间/AsOf 零值表示不限。
type FetchRequest struct {
	Capability Capability
	Subject    Subject
	Start      time.Time      // 区间起（含）；零值 = 不限
	End        time.Time      // 区间止（含）；零值 = 不限
	Limit      int            // 最近 N 条；0 = 不限
	Params     map[string]any // 透传额外参数（如 industry、top、fields）
}

// asOf 返回截断基准日：显式请求值优先，否则退回主体的 AsOf。
func (r FetchRequest) asOf() time.Time {
	if !r.End.IsZero() && r.Subject.AsOf.IsZero() {
		return r.End
	}
	return r.Subject.AsOf
}

// Result 是 Fetch 的返回联合（Series | Table | Object）。
type Result interface{ isResult() }

// Point 序列上的一个时点。Values 以列名为键（如 close/open/level/yoy）。
type Point struct {
	Period time.Time
	Values map[string]float64
}

// Series 时序结果（行情/指数/宏观）。
type Series struct {
	Subject Subject
	Points  []Point
	Meta    Meta
}

func (Series) isResult() {}

// Table 行列结果（财报/成分/公告）。
type Table struct {
	Columns []string
	Rows    []map[string]any
	Meta    Meta
}

func (Table) isResult() {}

// Object 单对象结果（如一条估值快照）。
type Object struct {
	Fields map[string]any
	Meta   Meta
}

func (Object) isResult() {}

// Meta 取数元信息，随结果下传（诚实标注来源与陈旧）。
type Meta struct {
	Source string    // "datahub:mcp" / "akshare" / "localpg" …
	Stale  bool      // 源标注为陈旧
	Cached bool      // 命中上游缓存
	Rows   int       // 原始行数
	AsOf   time.Time // 实际截断基准日
}
