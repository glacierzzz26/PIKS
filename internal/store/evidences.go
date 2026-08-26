package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

// CreateEvidence 为事件补证。插入后 bump 事件 updated_at(补证是事件更新,触发增量发布重写卡片)。
func (s *Store) CreateEvidence(ctx context.Context, ev *model.Evidence) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO evidences(event_id,claim,source_id,source_type,url,title,content,published_at,reliability)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		ev.EventID, ev.Claim, ev.SourceID, ev.SourceType, ev.URL, ev.Title, ev.Content,
		ev.PublishedAt, ev.Reliability).Scan(&id)
	if err == nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE events SET updated_at=now() WHERE id=$1`, ev.EventID)
	}
	return id, err
}

// ListEvidenceByEventID 返回某事件全部证据(发布卡片"证据"节用,按补证先后排列)。
func (s *Store) ListEvidenceByEventID(ctx context.Context, eventID string) ([]model.Evidence, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,event_id,claim,source_id,source_type,url,title,content,published_at,retrieved_at,reliability,created_at
		 FROM evidences WHERE event_id=$1 ORDER BY created_at`, eventID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Evidence])
}
