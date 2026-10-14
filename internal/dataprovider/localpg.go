package dataprovider

import (
	"context"
	"database/sql"
)

// LocalPGProvider —— 已有本地投影（phase13 §3.3）。
//
// 数据面：PIKS 自己的 PostgreSQL 投影 —— 事件（`events` + 簇/印证）、市场快照
// （`market_snapshots`）、研报历史（`research_runs`）。这些**已在库中**，不需外采；
// 归位到本契约是 phase13 P13-2+ 的接线工作。
//
// ⚠️ **本轮为占位**：持有可选的 `*sql.DB`（供将来实现），`Fetch` 一律返回
// `ErrNotWired`（**绝不返回假数据**）。
type LocalPGProvider struct {
	db *sql.DB
}

// NewLocalPGProvider 构造占位实现。db 可为 nil（本轮仅占位，未使用）。
func NewLocalPGProvider(db *sql.DB) *LocalPGProvider { return &LocalPGProvider{db: db} }

// Name 实现 Provider。
func (p *LocalPGProvider) Name() string { return "localpg" }

// Supports 实现 Provider：本地投影能直接答的能力。
func (p *LocalPGProvider) Supports(cap Capability) bool {
	switch cap {
	case CapEvents, CapMarketBreadth, CapHotTopics, CapAnnouncements:
		return true
	}
	return false
}

// Fetch 占位：未接线。
func (p *LocalPGProvider) Fetch(_ context.Context, req FetchRequest) (Result, error) {
	return nil, ErrNotWired
}
