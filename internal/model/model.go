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
	CreatedAt       time.Time       `db:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at"`
	ValidFrom       *time.Time      `db:"valid_from"`
	ValidTo         *time.Time      `db:"valid_to"`
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
