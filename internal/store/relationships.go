package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"piks/internal/model"
)

// CreateRelationship 幂等写入;唯一约束重复时静默忽略。
func (s *Store) CreateRelationship(ctx context.Context, rel *model.Relationship) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO relationships(from_type,from_id,to_type,to_id,rel_type,properties,confidence,source,valid_from,valid_to)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (from_type, from_id, to_type, to_id, rel_type) DO NOTHING`,
		rel.FromType, rel.FromID, rel.ToType, rel.ToID, rel.RelType, rel.Properties,
		rel.Confidence, rel.Source, rel.ValidFrom, rel.ValidTo)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return nil // 并发竞态下的重复
	}
	return err
}
