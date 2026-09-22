package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"piks/internal/model"
)

// CreateRelationship 幂等写入;唯一约束重复时静默忽略。
// properties 为 NOT NULL:调用方未设置时以 '{}' 落库(而非 NULL,否则违反约束)。
func (s *Store) CreateRelationship(ctx context.Context, rel *model.Relationship) error {
	props := rel.Properties
	if len(props) == 0 || string(props) == "null" {
		props = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO relationships(from_type,from_id,to_type,to_id,rel_type,properties,confidence,source,valid_from,valid_to)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (from_type, from_id, to_type, to_id, rel_type) DO NOTHING`,
		rel.FromType, rel.FromID, rel.ToType, rel.ToID, rel.RelType, props,
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

// graphExcludedRelTypes 不进关系图谱的边类型(P6-4)。
// 决策边(from_type='trade')属于"我的决策"链,不是实体关系网络的一环;
// 若不过滤会以无名节点混入图谱统计。图谱投影与图谱端点共用此过滤。
const graphRelFilter = `rel_type NOT IN ('decided_by','based_on')`

// ListAllRelationships 全部关系(前端关系图谱投影,api_v1)。
// 端点类型(event→entity / entity→entity)由前端按节点 id 集合自行过滤;
// 决策边(decided_by/based_on)由服务端在此排除(P6-4,见 graphRelFilter)。
func (s *Store) ListAllRelationships(ctx context.Context) ([]model.Relationship, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+relCols+` FROM relationships WHERE `+graphRelFilter+` ORDER BY from_id, to_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Relationship])
}

// ListRelationshipsFrom 某 from 源的全部出边(决策记录 P6-4:trade → 研报/事件/笔记)。
// types 为空取全部 rel_type;非空时仅取指定类型。
func (s *Store) ListRelationshipsFrom(ctx context.Context, fromType, fromID string, types ...string) ([]model.Relationship, error) {
	q := `SELECT ` + relCols + ` FROM relationships WHERE from_type=$1 AND from_id=$2`
	args := []any{fromType, fromID}
	if len(types) > 0 {
		q += ` AND rel_type = ANY($3)`
		args = append(args, types)
	}
	q += ` ORDER BY rel_type`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Relationship])
}

// ListRelationshipsFromIDs 批量出边(个股中心一次取全部交易的决策边,免 N 次往返)。
func (s *Store) ListRelationshipsFromIDs(ctx context.Context, fromType string, fromIDs []string, types ...string) ([]model.Relationship, error) {
	if len(fromIDs) == 0 {
		return nil, nil
	}
	q := `SELECT ` + relCols + ` FROM relationships WHERE from_type=$1 AND from_id = ANY($2)`
	args := []any{fromType, fromIDs}
	if len(types) > 0 {
		q += ` AND rel_type = ANY($3)`
		args = append(args, types)
	}
	q += ` ORDER BY from_id, rel_type`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Relationship])
}

// ListAffectedTermEvents affected 词 → 事件 id 映射(实体构建用)。
//
// ⚠️ 必须加 `jsonb_typeof(affected) = 'array'` 守卫(issue #71):历史行里存在
// JSON `null` 的 affected,而 `jsonb_array_elements_text` 遇标量即报
// SQLSTATE 22023「cannot extract elements from a scalar」,会让 entity-build
// **整条命令崩掉**(不是跳过坏行)。写入侧已修(emptyToArrayIfScalar),此处是
// 纵深防御 —— 即便再有标量落库,也只被跳过、不再崩命令。
func (s *Store) ListAffectedTermEvents(ctx context.Context) (map[string][]string, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT elem AS term, e.id AS event_id
		FROM events e, jsonb_array_elements_text(e.affected) elem
		WHERE jsonb_typeof(e.affected) = 'array'`)
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
