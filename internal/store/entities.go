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
// 返回 (id, created bool, err)。
func (s *Store) UpsertEntity(ctx context.Context, e *model.Entity) (string, bool, error) {
	aliases := e.Aliases
	if len(aliases) == 0 {
		aliases = json.RawMessage(`[]`)
	}
	detail := e.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO entities (type, name, aliases, description, detail, status)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (type, name) DO UPDATE SET
			aliases=EXCLUDED.aliases, description=EXCLUDED.description,
			detail=EXCLUDED.detail, status=EXCLUDED.status, updated_at=now()
		RETURNING id`,
		e.Type, e.Name, aliases, e.Description, detail, defaultStr(e.Status, "active")).
		Scan(&id)
	if err != nil {
		return "", false, err
	}
	// 区分创建/更新:created_at 是否 = 本次插入。
	var created bool
	_ = s.Pool.QueryRow(ctx,
		`SELECT (created_at = updated_at) FROM entities WHERE id=$1`, id).Scan(&created)
	return id, created, nil
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
