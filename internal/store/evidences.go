package store

import (
	"context"

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
