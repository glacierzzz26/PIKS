package store

import (
	"context"
	"fmt"

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

// MergeClusters 把 absorbID 簇整体并入 survivorID 簇(聚类重审视用,design cluster-quality §4.1):
//   - absorb 簇全部事件 cluster_id → survivorID;
//   - 其中非 merged 成员(原代表)status → 'merged',并 bump updated_at(增量发布据此删旧卡);
//   - absorb 簇行 status → 'merged'(保留审计痕迹)。
//
// absorbID 必须 ≠ survivorID;两步在事务内完成。
func (s *Store) MergeClusters(ctx context.Context, absorbID, survivorID string) error {
	if absorbID == survivorID {
		return fmt.Errorf("merge clusters: absorbID == survivorID (%s)", absorbID)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("merge clusters: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // 提交成功后回滚是无操作
	if _, err := tx.Exec(ctx,
		`UPDATE events
		 SET cluster_id=$2,
		     status = CASE WHEN status IN ('extracted','verified','published') THEN 'merged' ELSE status END,
		     updated_at=now()
		 WHERE cluster_id=$1`, absorbID, survivorID); err != nil {
		return fmt.Errorf("merge clusters: move events: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE event_clusters SET status='merged', updated_at=now() WHERE id=$1`, absorbID); err != nil {
		return fmt.Errorf("merge clusters: mark absorbed cluster merged: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("merge clusters: commit: %w", err)
	}
	return nil
}
