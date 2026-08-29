package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
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

const relCols = `id,from_type,from_id,to_type,to_id,rel_type,properties,confidence,source,created_at,valid_from,valid_to`

// ListEntityRelationships 某实体参与的全部关系(实体卡渲染:相关事件/相关行业)。
func (s *Store) ListEntityRelationships(ctx context.Context, entityID string) ([]model.Relationship, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+relCols+` FROM relationships
		WHERE (from_type='entity' AND from_id=$1) OR (to_type='entity' AND to_id=$1)
		ORDER BY rel_type`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Relationship])
}

// ListRelationshipsFromTo 按 from/to 定向查询(实体构建/补链用)。
func (s *Store) ListRelationshipsFromTo(ctx context.Context, fromType, toType, relType string) ([]model.Relationship, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+relCols+` FROM relationships
		WHERE from_type=$1 AND to_type=$2 AND rel_type=$3
		ORDER BY from_id`, fromType, toType, relType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Relationship])
}

// ListAllRelationships 全部关系(前端关系图谱投影,api_v1)。
// 端点类型(event→entity / entity→entity)由前端按节点 id 集合自行过滤。
func (s *Store) ListAllRelationships(ctx context.Context) ([]model.Relationship, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+relCols+` FROM relationships ORDER BY from_id, to_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Relationship])
}

// ListAffectedTermEvents affected 词 → 事件 id 映射(实体构建用)。
func (s *Store) ListAffectedTermEvents(ctx context.Context) (map[string][]string, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT elem AS term, e.id AS event_id
		FROM events e, jsonb_array_elements_text(e.affected) elem`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var term, eventID string
		if err := rows.Scan(&term, &eventID); err != nil {
			return nil, err
		}
		out[term] = append(out[term], eventID)
	}
	return out, rows.Err()
}

// EventRef 事件引用(实体卡相关事件 / hot_topics event_ids)。
type EventRef struct {
	ID    string
	Title string
}

// ListEventsAffectingEntities rel_type='affects' 到指定实体的事件(去重)。
// 供:hot_topics 补 event_ids(§3.4)、实体卡"相关事件"节。
func (s *Store) ListEventsAffectingEntities(ctx context.Context, entityIDs []string) ([]EventRef, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT e.id, e.title
		FROM relationships r
		JOIN events e ON e.id = r.from_id
		WHERE r.from_type='event' AND r.to_type='entity' AND r.rel_type='affects'
		  AND r.to_id = ANY($1)
		ORDER BY e.title`, entityIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (EventRef, error) {
		var ref EventRef
		return ref, r.Scan(&ref.ID, &ref.Title)
	})
}

// GraphEdge 图谱 affects 边(事件→实体),web 关系图谱用。
type GraphEdge struct {
	EventID  string `db:"event_id"`
	EntityID string `db:"entity_id"`
}

// ListGraphEdges 全部 affects 边(事件→实体),供关系图谱与对话 grounding。
func (s *Store) ListGraphEdges(ctx context.Context) ([]GraphEdge, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.from_id AS event_id, r.to_id AS entity_id
		FROM relationships r
		WHERE r.from_type='event' AND r.to_type='entity' AND r.rel_type='affects'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (GraphEdge, error) {
		var e GraphEdge
		return e, r.Scan(&e.EventID, &e.EntityID)
	})
}

// ListEventAffectedEntities 某事件 affects 到的全部实体(事件卡"影响"、图谱邻居)。
func (s *Store) ListEventAffectedEntities(ctx context.Context, eventID string) ([]model.Entity, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT ent.* FROM entities ent
		JOIN relationships r ON r.to_type='entity' AND r.to_id=ent.id
		WHERE r.from_type='event' AND r.from_id=$1 AND r.rel_type='affects'
		ORDER BY ent.name`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
}
