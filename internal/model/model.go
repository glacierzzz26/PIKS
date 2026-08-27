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
	ID              string          `db:"id"`
	SourceID        string          `db:"source_id"`
	ExternalID      *string         `db:"external_id"`
	URL             *string         `db:"url"`
	Title           *string         `db:"title"`
	Content         string          `db:"content"`
	ContentHash     string          `db:"content_hash"`
	PublishedAt     *time.Time      `db:"published_at"`
	RetrievedAt     time.Time       `db:"retrieved_at"`
	Status          string          `db:"status"`
	PipelineVersion *string         `db:"pipeline_version"`
	Error           *string         `db:"error"`
	CreatedAt       time.Time       `db:"created_at"`
}

type Event struct {
	ID              string          `db:"id"`
	RawDocumentID   *string         `db:"raw_document_id"`
	Title           string          `db:"title"`
	EventType       string          `db:"event_type"`
	Summary         *string         `db:"summary"`
	Facts           json.RawMessage `db:"facts"`
	Affected        json.RawMessage `db:"affected"`
	OccurredAt      *time.Time      `db:"occurred_at"`
	Confidence      float64         `db:"confidence"`
	Status          string          `db:"status"`
	PipelineVersion *string         `db:"pipeline_version"`
	SourceID        *string         `db:"source_id"`
	ClusterID       *string         `db:"cluster_id"`
	PublishedAt     *time.Time      `db:"published_at"`
	CreatedAt       time.Time       `db:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at"`
	ValidFrom       *time.Time      `db:"valid_from"`
	ValidTo         *time.Time      `db:"valid_to"`
}

// EventCluster 事件簇:同一真实事件的报道集合(迭代 1,设计 D8)。
type EventCluster struct {
	ID        string    `db:"id"`
	Title     string    `db:"title"`
	Status    string    `db:"status"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
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
	ID           string     `db:"id"`
	EventID      *string    `db:"event_id"`
	Market       string     `db:"market"`
	Indicator    string     `db:"indicator"`
	Value        string     `db:"value"`
	PreviousVal  *string    `db:"previous_value"`
	Change       *string    `db:"change"`
	ObservedAt   time.Time  `db:"observed_at"`
	Source       *string    `db:"source"`
	CreatedAt    time.Time  `db:"created_at"`
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
