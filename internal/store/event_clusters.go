package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

// CreateEventCluster 新建事件簇,返回 id。
func (s *Store) CreateEventCluster(ctx context.Context, c *model.EventCluster) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO event_clusters(title, status) VALUES($1,$2) RETURNING id`,
		c.Title, defaultStr(c.Status, "active")).Scan(&id)
	return id, err
}

func (s *Store) GetEventClusterByID(ctx context.Context, id string) (model.EventCluster, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,title,status,created_at,updated_at FROM event_clusters WHERE id=$1`, id)
	if err != nil {
		return model.EventCluster{}, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.EventCluster])
}
