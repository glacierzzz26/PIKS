package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

func (s *Store) CreateEvidence(ctx context.Context, ev *model.Evidence) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO evidences(event_id,claim,source_id,source_type,url,title,content,published_at,reliability)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		ev.EventID, ev.Claim, ev.SourceID, ev.SourceType, ev.URL, ev.Title, ev.Content,
		ev.PublishedAt, ev.Reliability).Scan(&id)
	return id, err
}

// GetEvidenceByEventID 返回某事件的第一条证据(发布卡片"证据"节用)。
func (s *Store) GetEvidenceByEventID(ctx context.Context, eventID string) (model.Evidence, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,event_id,claim,source_id,source_type,url,title,content,published_at,retrieved_at,reliability,created_at
		 FROM evidences WHERE event_id=$1 ORDER BY created_at LIMIT 1`, eventID)
	if err != nil {
		return model.Evidence{}, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Evidence])
}
