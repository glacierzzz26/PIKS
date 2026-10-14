package dataprovider

import "context"

// FreeSourceProvider —— 免费源兜底 + 事件/公告/热榜（已有链路归位）。
//
// 定位（phase13 §3.3）：主源（datahub/MCP）缺该票时降级到此；以及承接 PIKS **已有**的
// 采集链路（akshare 取数 + 事件/公告/热榜）。这些链路在 Python（research）与 Go 管线
// （internal/collector、hot_topic_items）中**已存在**，归位到本契约是 phase13 P13-2+
// 的接线工作。
//
// ⚠️ **本轮为占位**：声明能力但 `Fetch` 一律返回 `ErrNotWired`（**绝不返回假数据**）。
// 调用方据此如实降级/标 `数据不可得`。
type FreeSourceProvider struct{}

// NewFreeSourceProvider 构造占位实现。
func NewFreeSourceProvider() *FreeSourceProvider { return &FreeSourceProvider{} }

// Name 实现 Provider。
func (p *FreeSourceProvider) Name() string { return "freesource" }

// Supports 实现 Provider：免费源可兜底的能力（行情/财务/宏观）+ 已有的事件/公告/热榜。
func (p *FreeSourceProvider) Supports(cap Capability) bool {
	switch cap {
	case CapQuoteSeries, CapFinancials, CapValuationHistory, CapCapitalFlow,
		CapIndustryMembers, CapIndustryFinancials, CapMacro, CapMarketBreadth,
		CapEvents, CapAnnouncements, CapHotTopics:
		return true
	}
	return false
}

// Fetch 占位：未接线。
func (p *FreeSourceProvider) Fetch(_ context.Context, req FetchRequest) (Result, error) {
	return nil, ErrNotWired
}
