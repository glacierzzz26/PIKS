// Package model 定义与 7 张表一一对应的领域结构(db 标签供 pgx RowToStructByName 使用)。
package model

import (
	"encoding/json"
	"time"
)

type Source struct {
	ID         string          `db:"id"`
	Name       string          `db:"name"`
	SourceType string          `db:"source_type"`
	Config     json.RawMessage `db:"config"`
	Status     string          `db:"status"`
	CreatedAt  time.Time       `db:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at"`
}

type RawDocument struct {
	ID          string     `db:"id"`
	SourceID    string     `db:"source_id"`
	ExternalID  *string    `db:"external_id"`
	URL         *string    `db:"url"`
	Title       *string    `db:"title"`
	Content     string     `db:"content"`
	ContentHash string     `db:"content_hash"`
	PublishedAt *time.Time `db:"published_at"`
	RetrievedAt time.Time  `db:"retrieved_at"`
	Status      string     `db:"status"`
	Grade       *string    `db:"grade"` // 公告分级(issue #68);快讯源为 NULL
	// OriginKind 实时/正式分道闸(issue #83 P-2,迁移 0021):pipeline/realtime。
	// 空值由 store 落 'pipeline'(defaultStr)。worker 与 reconcile 只处理 'pipeline'。
	OriginKind string `db:"origin_kind"`
	// CanonicalID raw 层转载组代表行(P-8 用);NULL = 自己是代表。
	// ⚠️ 本版**全为 NULL**(只建列),P-8 落地前不得基于它实现读路径。
	CanonicalID     *string         `db:"canonical_id"`
	PipelineVersion *string         `db:"pipeline_version"`
	Error           *string         `db:"error"`
	Extra           json.RawMessage `db:"extra"` // 上游原始字段原样留存(issue #43);缺省 {}
	CreatedAt       time.Time       `db:"created_at"`
}

type Event struct {
	ID            string          `db:"id"`
	RawDocumentID *string         `db:"raw_document_id"`
	Title         string          `db:"title"`
	EventType     string          `db:"event_type"`
	Summary       *string         `db:"summary"`
	Facts         json.RawMessage `db:"facts"`
	Affected      json.RawMessage `db:"affected"`
	OccurredAt    *time.Time      `db:"occurred_at"`
	Confidence    float64         `db:"confidence"`
	Status        string          `db:"status"`
	// HumanVerdict 人工标记(issue P4 #7,迁移 0021):**引擎永不触碰**,防幂等重跑冲掉。
	// NULL = 无人工意见(≠ 否定)。本版只建列,无写入口/无 UI。
	HumanVerdict    *string    `db:"human_verdict"`
	PipelineVersion *string    `db:"pipeline_version"`
	SourceID        *string    `db:"source_id"`
	ClusterID       *string    `db:"cluster_id"`
	PublishedAt     *time.Time `db:"published_at"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
	ValidFrom       *time.Time `db:"valid_from"`
	ValidTo         *time.Time `db:"valid_to"`
}

// EventCluster 事件簇:同一真实事件的报道集合(迭代 1,设计 D8)。
type EventCluster struct {
	ID     string `db:"id"`
	Title  string `db:"title"`
	Status string `db:"status"`
	// CanonicalEventID 簇的详情入口事件(issue P4 #8,迁移 0021):ApplyClusters 建簇时写入,
	// reexamine 不重选(冻结)。NULL = 无在产成员。无 FK,消费方须处理缺失。
	CanonicalEventID *string   `db:"canonical_event_id"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

type Evidence struct {
	ID          string     `db:"id"`
	EventID     *string    `db:"event_id"`
	Claim       string     `db:"claim"`
	SourceID    *string    `db:"source_id"`
	SourceType  *string    `db:"source_type"`
	URL         *string    `db:"url"`
	Title       *string    `db:"title"`
	Content     *string    `db:"content"`
	PublishedAt *time.Time `db:"published_at"`
	RetrievedAt time.Time  `db:"retrieved_at"`
	Reliability *string    `db:"reliability"`
	CreatedAt   time.Time  `db:"created_at"`
}

type Observation struct {
	ID          string    `db:"id"`
	EventID     *string   `db:"event_id"`
	Market      string    `db:"market"`
	Indicator   string    `db:"indicator"`
	Value       string    `db:"value"`
	PreviousVal *string   `db:"previous_value"`
	Change      *string   `db:"change"`
	ObservedAt  time.Time `db:"observed_at"`
	Source      *string   `db:"source"`
	CreatedAt   time.Time `db:"created_at"`
}

// MarketSnapshot 每日市场状态快照(迭代 2,设计 §2.1;架构 §9.7 Market + §9.8 Emotion)。
type MarketSnapshot struct {
	ID               string          `db:"id"`
	TradeDate        time.Time       `db:"trade_date"`
	IndexJSON        json.RawMessage `db:"index_json"`
	TurnoverAmt      *float64        `db:"turnover_amt"`
	Breadth          json.RawMessage `db:"breadth"`
	LimitUpCount     *int            `db:"limit_up_count"`
	LimitDownCount   *int            `db:"limit_down_count"`
	BrokenLimitCount *int            `db:"broken_limit_count"`
	MaxBoard         *int            `db:"max_board"`
	ZTPool           json.RawMessage `db:"zt_pool"`
	StrongYesterday  json.RawMessage `db:"strong_yesterday"`
	IndustryDist     json.RawMessage `db:"industry_dist"`
	HotTopics        json.RawMessage `db:"hot_topics"`
	TopEvents        json.RawMessage `db:"top_events"`
	CapitalFlow      json.RawMessage `db:"capital_flow"`
	EmotionScore     *float64        `db:"emotion_score"`
	EmotionState     *string         `db:"emotion_state"`
	EmotionDetail    json.RawMessage `db:"emotion_detail"`
	MyJudgment       *string         `db:"my_judgment"`
	Evidence         json.RawMessage `db:"evidence"`
	CreatedAt        time.Time       `db:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at"`
}

// Entity 统一实体(迭代 3,设计 §2.1;架构 §9.1)。type 判别 + detail JSONB。
type Entity struct {
	ID          string          `db:"id"`
	Type        string          `db:"type"`
	Name        string          `db:"name"`
	Aliases     json.RawMessage `db:"aliases"`
	Description *string         `db:"description"`
	Detail      json.RawMessage `db:"detail"`
	Status      string          `db:"status"`
	CreatedAt   time.Time       `db:"created_at"`
	UpdatedAt   time.Time       `db:"updated_at"`
}

type Relationship struct {
	ID         string          `db:"id"`
	FromType   string          `db:"from_type"`
	FromID     string          `db:"from_id"`
	ToType     string          `db:"to_type"`
	ToID       string          `db:"to_id"`
	RelType    string          `db:"rel_type"`
	Properties json.RawMessage `db:"properties"`
	Confidence *float64        `db:"confidence"`
	Source     *string         `db:"source"`
	CreatedAt  time.Time       `db:"created_at"`
	ValidFrom  *time.Time      `db:"valid_from"`
	ValidTo    *time.Time      `db:"valid_to"`
}

// HotTopicItem 热榜快照一条(issue #68 D 层,热榜时序表 hot_topic_items,迁移 0018)。
//
// 🔴 与 Event/MarketSnapshot 刻意隔离:热度**可被操纵**,只作展示/排序权重,
//
//	绝不作为「重要性」判定,也不得与印证度(source_count/cluster_sources)合并计算。
//	故本结构**不进 raw_documents、不进聚类、不参与 events 任何逻辑**(设计 §6 红线)。
//	⚠️ Rank 与 HotValue **仅源内可比**,跨源不可比(两源粒度/量纲不同)—— 展示层须分列。
type HotTopicItem struct {
	ID         string          `db:"id"`
	Source     string          `db:"source"`      // ths-topic / cls-hot-article
	Rank       int             `db:"rank"`        // 榜内名次(1-based)
	Title      string          `db:"title"`       // 话题标题(原文)
	HotValue   *int64          `db:"hot_value"`   // 该源口径热度;NULL = 上游未给(非 0)
	URL        *string         `db:"url"`         // 原文/话题链接;NULL = 上游未给
	Extra      json.RawMessage `db:"extra"`       // 上游原始字段留档
	SnapshotAt time.Time       `db:"snapshot_at"` // 本轮采集时刻(同批同值)
	CreatedAt  time.Time       `db:"created_at"`
}

// WatchlistEntry 同花顺「我的自选」的一条元数据(表 watchlist_entries,迁移 0020)。
//
// 🔴 成员资格**(在不在自选)**仍以 entities.status='watch' 为真源;本结构**只**承载
// 加入价/加入日 —— 因为 entities.detail 每轮被 entity-build 无条件覆盖,价/日不能进 detail。
// 🔴 RemovedAt 非空 = 已移出(不删行)。⚠️ AddedPrice/AddedOn 为 NULL 表示上游未给,**非 0**。
type WatchlistEntry struct {
	Code         string          `db:"code"`        // 6 位规范化代码(主键)
	EntityID     *string         `db:"entity_id"`   // entities.id;NULL = 名称未解析、实体未建
	Market       string          `db:"market"`      // SH/SZ/KC/CYB/BJ
	MarketID     string          `db:"market_id"`   // 上游原始 marketid
	AddedPrice   *float64        `db:"added_price"` // 加入价;NULL = 上游未给
	AddedOn      *time.Time      `db:"added_on"`    // 加入日;NULL = 上游未给
	RemovedAt    *time.Time      `db:"removed_at"`  // 移出时刻;NULL = 当前在自选
	FirstSeenAt  time.Time       `db:"first_seen_at"`
	LastSyncedAt time.Time       `db:"last_synced_at"`
	Extra        json.RawMessage `db:"extra"`
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
}

type TaskRun struct {
	ID        int64           `db:"id"`
	Command   string          `db:"command"`
	Status    string          `db:"status"`
	StartedAt time.Time       `db:"started_at"`
	EndedAt   *time.Time      `db:"ended_at"`
	Error     *string         `db:"error"`
	Meta      json.RawMessage `db:"meta"`
	CreatedAt time.Time       `db:"created_at"`
}

// PersonalNote 个人认知沉淀(belief/case/mistake/note),Web 编辑,权威源=PG。
type PersonalNote struct {
	ID         string          `db:"id"`
	Type       string          `db:"type"`
	Slug       string          `db:"slug"`
	Title      *string         `db:"title"`
	Status     string          `db:"status"`
	Confidence *float64        `db:"confidence"`
	Content    *string         `db:"content"`
	Detail     json.RawMessage `db:"detail"`
	Author     string          `db:"author"`
	UpdatedBy  *string         `db:"updated_by"`
	CreatedAt  time.Time       `db:"created_at"`
	UpdatedAt  time.Time       `db:"updated_at"`
}

// ChatMessage AI 对话消息(迭代 5-3,/chat)。refs 结构见 ChatRefs。
type ChatMessage struct {
	ID          string          `db:"id"`
	SessionID   string          `db:"session_id"`
	Role        string          `db:"role"`
	Content     string          `db:"content"`
	Refs        json.RawMessage `db:"refs"`
	Attachments json.RawMessage `db:"attachments"`
	CreatedAt   time.Time       `db:"created_at"`
}

// Attachment 截图附件元数据(文件本体存 data/uploads/,不进库)。
type Attachment struct {
	ID        string    `db:"id"`
	Filename  string    `db:"filename"`
	MIME      string    `db:"mime"`
	Size      int64     `db:"size"`
	Path      string    `db:"path"`
	CreatedAt time.Time `db:"created_at"`
}

// Trade 交易记录(交易功能,design trades.md;迁移 0010)。
// review = AI 带引用复盘 {review,refs,model,tokens,generated_at}。
type Trade struct {
	ID           string          `db:"id"`
	TradeDate    time.Time       `db:"trade_date"`
	Code         string          `db:"code"`
	Name         string          `db:"name"`
	Side         string          `db:"side"` // buy / sell
	Price        float64         `db:"price"`
	Qty          int             `db:"qty"`
	Amount       float64         `db:"amount"`
	Source       string          `db:"source"` // manual / screenshot
	AttachmentID *string         `db:"attachment_id"`
	Note         *string         `db:"note"`
	Review       json.RawMessage `db:"review"`
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
}

// Position 持仓快照(同花顺持仓截图;只存只展示,V1 不调 AI)。
type Position struct {
	ID           string    `db:"id"`
	SnapshotDate time.Time `db:"snapshot_date"`
	Code         string    `db:"code"`
	Name         string    `db:"name"`
	Qty          int       `db:"qty"`
	CostPrice    *float64  `db:"cost_price"`
	Price        *float64  `db:"price"`
	MarketValue  *float64  `db:"market_value"`
	PL           *float64  `db:"pl"`
	Source       string    `db:"source"`
	AttachmentID *string   `db:"attachment_id"`
	CreatedAt    time.Time `db:"created_at"`
}

// AccountSnapshot 账户级资金快照(迁移 0013,issue #19):同花顺「持仓」页顶部汇总。
// 四项全是指针 —— NULL = 「截图没有这个数」,与 0(确实为零)严格区分,禁止推断。
type AccountSnapshot struct {
	ID           string    `db:"id"`
	SnapshotDate time.Time `db:"snapshot_date"`
	TotalAsset   *float64  `db:"total_asset"` // 总资产(含现金)
	TotalMV      *float64  `db:"total_mv"`    // 总市值(不含现金)
	FloatPL      *float64  `db:"float_pl"`    // 浮动盈亏(累计)
	DailyPL      *float64  `db:"daily_pl"`    // 当日参考盈亏
	Source       string    `db:"source"`
	AttachmentID *string   `db:"attachment_id"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}
