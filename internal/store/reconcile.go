package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReconIssue 对账异常项。
type ReconIssue struct {
	Category string `db:"category"`
	EntityID string `db:"entity_id"`
	Detail   string `db:"detail"`
}

// ReconStaleRaw raw 滞留超过 7 天未处理。
func (s *Store) ReconStaleRaw(ctx context.Context) ([]ReconIssue, error) {
	return s.recon(ctx, `SELECT 'stale_raw' AS category, id::text AS entity_id, '滞留未处理' AS detail
		FROM raw_documents WHERE status='raw' AND retrieved_at < now() - interval '7 days'`)
}

// ReconFailedRaw 抽取失败。
func (s *Store) ReconFailedRaw(ctx context.Context) ([]ReconIssue, error) {
	return s.recon(ctx, `SELECT 'failed_raw' AS category, id::text AS entity_id, COALESCE(error,'') AS detail
		FROM raw_documents WHERE status='failed'`)
}

// ReconProcessedNoEvent 已处理但未抽取到任何事件。
func (s *Store) ReconProcessedNoEvent(ctx context.Context) ([]ReconIssue, error) {
	return s.recon(ctx, `SELECT 'processed_no_event' AS category, r.id::text AS entity_id, r.title AS detail
		FROM raw_documents r
		LEFT JOIN events e ON e.raw_document_id = r.id
		WHERE r.status='processed' AND e.id IS NULL`)
}

// ReconOrphanEvent 事件指向不存在/被删的 raw_document。
func (s *Store) ReconOrphanEvent(ctx context.Context) ([]ReconIssue, error) {
	return s.recon(ctx, `SELECT 'orphan_event' AS category, e.id::text AS entity_id, e.title AS detail
		FROM events e LEFT JOIN raw_documents r ON r.id = e.raw_document_id
		WHERE e.raw_document_id IS NOT NULL AND r.id IS NULL`)
}

// ReconMissingEvidence 可抽取/可复核事件却无证据行。
func (s *Store) ReconMissingEvidence(ctx context.Context) ([]ReconIssue, error) {
	return s.recon(ctx, `SELECT 'missing_evidence' AS category, e.id::text AS entity_id, e.title AS detail
		FROM events e LEFT JOIN evidences v ON v.event_id = e.id
		WHERE e.status IN ('extracted','verified') AND v.id IS NULL`)
}

// ReconSilentSources active 源近 24h 无采集记录(静默失败)。
func (s *Store) ReconSilentSources(ctx context.Context) ([]ReconIssue, error) {
	return s.recon(ctx, `SELECT 'silent_source' AS category, s.id::text AS entity_id, s.name AS detail
		FROM sources s
		WHERE s.status='active'
		  AND NOT EXISTS (SELECT 1 FROM raw_documents r WHERE r.source_id = s.id AND r.retrieved_at > now() - interval '24 hours')`)
}

func (s *Store) recon(ctx context.Context, query string) ([]ReconIssue, error) {
	rows, err := s.Pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[ReconIssue])
}

// ReconDaily 每日对账行(前端 /recon 投影)。
type ReconDaily struct {
	Date      time.Time `db:"date"`
	Flashes   int       `db:"flashes"`
	Events    int       `db:"events"`
	Anomalies int       `db:"anomalies"`
}

// ListReconDaily 活跃数据日期范围的每日对账,按日倒序。
// anomalies = 当日抽取失败的快讯数(failed_raw);范围取 raw_documents/events/snapshots 的交并。
func (s *Store) ListReconDaily(ctx context.Context) ([]ReconDaily, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH ds AS (
			SELECT date_trunc('day', COALESCE(published_at, retrieved_at))::date AS d FROM raw_documents
			UNION
			SELECT date_trunc('day', COALESCE(occurred_at, created_at))::date AS d FROM events
			UNION
			SELECT trade_date FROM market_snapshots
		),
		days AS (
			SELECT generate_series(min(d), max(d), interval '1 day')::date AS d FROM ds
		)
		SELECT d AS date,
			(SELECT count(*) FROM raw_documents r
			 WHERE COALESCE(r.published_at, r.retrieved_at)::date = d) AS flashes,
			(SELECT count(*) FROM events e
			 WHERE e.status <> 'merged'
			   AND COALESCE(e.occurred_at, e.created_at)::date = d) AS events,
			(SELECT count(*) FROM raw_documents r
			 WHERE r.status = 'failed' AND COALESCE(r.published_at, r.retrieved_at)::date = d) AS anomalies
		FROM days ORDER BY d DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[ReconDaily])
}
