package store

// entities 存取(迭代 3,设计 §2.1;迁移 0004)。

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const entityCols = `id,type,name,aliases,description,detail,status,created_at,updated_at`

// UpsertEntity 按 (type,name) upsert(设计 §2.1 UNIQUE)。aliases/detail 用新值覆盖。
// 严格幂等:已存在且字段全同 → 不写库(零变更,重跑零 churn)。返回 (id, created bool, err)。
func (s *Store) UpsertEntity(ctx context.Context, e *model.Entity) (string, bool, error) {
	aliases := e.Aliases
	if len(aliases) == 0 {
		aliases = json.RawMessage(`[]`)
	}
	detail := e.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	status := defaultStr(e.Status, "active")

	var existing struct {
		ID      string
		Aliases json.RawMessage
		Detail  json.RawMessage
		Status  string
	}
	err := s.Pool.QueryRow(ctx,
		`SELECT id, aliases, detail, status FROM entities WHERE type=$1 AND name=$2`,
		e.Type, e.Name).
		Scan(&existing.ID, &existing.Aliases, &existing.Detail, &existing.Status)
	switch {
	case err == nil:
		if jsonEqual(existing.Aliases, aliases) && jsonEqual(existing.Detail, detail) && existing.Status == status {
			return existing.ID, false, nil // 无变更,跳过写
		}
		_, err = s.Pool.Exec(ctx,
			`UPDATE entities SET aliases=$2, description=$3, detail=$4, status=$5, updated_at=now()
			 WHERE id=$1`,
			existing.ID, aliases, e.Description, detail, status)
		return existing.ID, false, err
	case errors.Is(err, pgx.ErrNoRows):
		var id string
		err = s.Pool.QueryRow(ctx, `
			INSERT INTO entities (type, name, aliases, description, detail, status)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			e.Type, e.Name, aliases, e.Description, detail, status).Scan(&id)
		return id, true, err
	default:
		return "", false, err
	}
}

func jsonEqual(a, b json.RawMessage) bool {
	if len(a) == 0 {
		a = json.RawMessage(`{}`)
	}
	if len(b) == 0 {
		b = json.RawMessage(`{}`)
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ab, _ := json.Marshal(x)
	bb, _ := json.Marshal(y)
	return string(ab) == string(bb)
}

// GetEntityByName 按规范名精确取实体(可空)。
func (s *Store) GetEntityByName(ctx context.Context, entityType, name string) (*model.Entity, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+entityCols+` FROM entities WHERE type=$1 AND name=$2`, entityType, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	e, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Entity])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &e, err
}

// SearchEntityByName 按 name 或 aliases 匹配实体(受 type 约束)。返回全部命中(按名升序)。
// 用于 affected 词匹配:精确名或别名命中即返回。
func (s *Store) SearchEntityByName(ctx context.Context, entityType, name string) ([]model.Entity, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+entityCols+` FROM entities
		WHERE type=$1 AND (name=$2 OR aliases @> $2::jsonb)
		ORDER BY name`, entityType, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
}

// ListEntitiesByType 某类型全部实体。
func (s *Store) ListEntitiesByType(ctx context.Context, entityType string) ([]model.Entity, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+entityCols+` FROM entities WHERE type=$1 ORDER BY name`, entityType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
}

// GetEntityByID 单实体(web 实体卡 / 图谱点选面板)。
func (s *Store) GetEntityByID(ctx context.Context, id string) (*model.Entity, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+entityCols+` FROM entities WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	e, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Entity])
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListEntitiesByIDs 按 id 批量取(供 publisher 按关系反查实体)。
func (s *Store) ListEntitiesByIDs(ctx context.Context, ids []string) ([]model.Entity, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT `+entityCols+` FROM entities WHERE id = ANY($1) ORDER BY name`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
}

// ListAllEntities 全部实体(实体构建/发布 in-memory 索引用)。
func (s *Store) ListAllEntities(ctx context.Context) ([]model.Entity, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ` + entityCols + ` FROM entities ORDER BY type, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
}

// ListAllEntityNames 全部实体名+别名(实体构建 dedup 用,零 AI 种子去重)。
func (s *Store) ListAllEntityNames(ctx context.Context) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT name FROM entities`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (string, error) {
		var n string
		return n, r.Scan(&n)
	})
}
