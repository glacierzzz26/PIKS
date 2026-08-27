package store

// stats.go 知识库规模统计(web 看板页)。

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Counts 知识库规模(看板页底部四项)。
type Counts struct {
	Events        int `db:"events"`
	Entities      int `db:"entities"`
	Relationships int `db:"relationships"`
	Snapshots     int `db:"snapshots"`
}

// Counts 返回当前规模。merged 事件不计(已并入簇,不单独展示)。
func (s *Store) Counts(ctx context.Context) (Counts, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT
			(SELECT count(*) FROM events WHERE status <> 'merged')  AS events,
			(SELECT count(*) FROM entities)                         AS entities,
			(SELECT count(*) FROM relationships)                    AS relationships,
			(SELECT count(*) FROM market_snapshots)                 AS snapshots`)
	if err != nil {
		return Counts{}, err
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[Counts])
}
