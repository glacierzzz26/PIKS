package store

// entities 存取(迭代 3,设计 §2.1;迁移 0004)。

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const entityCols = `id,type,name,aliases,description,detail,status,created_at,updated_at`

// NormalizeStockName 归一股票名(issue #6):去掉「字间空格」。
//
// 背景:东财涨停池接口对**历史改名票**返回的名字带空格(「金 螳 螂」「南 京 港」),
// entity-build 原样落库 → 实体名与正式名对不上,且与交易截图导入的无空格名
// 分叉成同码重复实体。实体主键是 (type,name),名称一脏就再难匹配。
//
// 规则:**仅当**去掉空格后全是 CJK 才归一 —— 股票名里出现空格只在东财加标记这一
// 种情形;英文多词名(Hugging Face / SB Energy)的空格是有意义的,不能碰,
// 故它们不满足「全 CJK」而原样返回。全角空格一并视作空白。
func NormalizeStockName(name string) string {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, name)
	if compact == "" {
		return strings.TrimSpace(name)
	}
	for _, r := range compact {
		if !unicode.Is(unicode.Han, r) {
			return strings.TrimSpace(name) // 含非汉字 → 多词英文名等,空格有意义,不动
		}
	}
	return compact
}

// GetCompanyEntityByCode 按 6 位代码取公司实体(设计 frontend-ia §2.4)。
// 代码经 NormalizeCode 归一(与 research_runs/交易补全对齐)。同 code 多实体时取最早创建的一行。
// 未建实体返回 (nil, nil) —— 个股中心以 code 为主键,entity 是可选的富化,不因缺实体报错。
func (s *Store) GetCompanyEntityByCode(ctx context.Context, code string) (*model.Entity, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+entityCols+` FROM entities
		 WHERE type='company' AND detail->>'code'=$1
		 ORDER BY created_at LIMIT 1`, NormalizeCode(code))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	e, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Entity])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// EnsureCompanyEntity 交易股票实体补全(design trades.md §2.3):按 name 或 detail->>code
// 查 type='company';缺则建(detail={code,source:'trade-import'} 标注来源,不编造描述)。返回实体 id。
//
// 名称先过 NormalizeStockName(issue #6):截图导入用无空格正式名,entity-build 曾落
// 带空格名 → 两版分叉成同码重复。归一后即可命中既有实体。name 查询再按「去空白相等」
// 兜底,兼容库里已存的空格脏行(清理脚本落地前也先合并而非新建)。
func (s *Store) EnsureCompanyEntity(ctx context.Context, code, name string) (string, error) {
	name = NormalizeStockName(name)
	var id string
	// 先按 name(规范名),再按 detail 里的代码(同名不同代码防撞)。
	// ⚠️ 每条查询各自绑定参数:代码那版必须绑 code(曾误绑 name,查的是 detail->>'code'=名称)。
	for _, q := range []struct {
		sql string
		arg string
	}{
		{`SELECT id FROM entities WHERE type='company'
		    AND regexp_replace(name, '\s', '', 'g') = $1
		    ORDER BY created_at LIMIT 1`, name},
		{`SELECT id FROM entities WHERE type='company' AND detail->>'code'=$1
		    ORDER BY created_at LIMIT 1`, code},
	} {
		err := s.Pool.QueryRow(ctx, q.sql, q.arg).Scan(&id)
		if err == nil {
			return id, nil
		}
		if err != pgx.ErrNoRows {
			return "", err
		}
	}
	// 缺 → 建(名称/代码来自截图/录入,来源可审计)。
	detail, _ := json.Marshal(map[string]string{"code": code, "source": "trade-import"})
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO entities (type, name, aliases, description, detail, status)
		VALUES ('company', $1, '[]'::jsonb, NULL, $2, 'active') RETURNING id`,
		name, detail).Scan(&id)
	return id, err
}

// UpsertEntity 按 (type,name) upsert(设计 §2.1 UNIQUE)。aliases/detail 用新值覆盖。
// 状态语义:显式传 status 才写 state;空 status = 「保持既有」(新建时落 'active')。
// ⚠️ entity-build 构造实体不设 status,若把空当 'active' 会每日清空自选(watch)——见设计 frontend-ia §2.3。
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
	explicitStatus := strings.TrimSpace(e.Status) != ""
	insertStatus := defaultStr(e.Status, "active")

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
		// 空 status 不下发 status 列(保留用户态 watch/archived);churn 比较仅看显式 status。
		if jsonEqual(existing.Aliases, aliases) && jsonEqual(existing.Detail, detail) &&
			(!explicitStatus || existing.Status == e.Status) {
			return existing.ID, false, nil // 无变更,跳过写
		}
		if explicitStatus {
			_, err = s.Pool.Exec(ctx,
				`UPDATE entities SET aliases=$2, description=$3, detail=$4, status=$5, updated_at=now()
				 WHERE id=$1`,
				existing.ID, aliases, e.Description, detail, e.Status)
		} else {
			_, err = s.Pool.Exec(ctx,
				`UPDATE entities SET aliases=$2, description=$3, detail=$4, updated_at=now()
				 WHERE id=$1`,
				existing.ID, aliases, e.Description, detail)
		}
		return existing.ID, false, err
	case errors.Is(err, pgx.ErrNoRows):
		var id string
		err = s.Pool.QueryRow(ctx, `
			INSERT INTO entities (type, name, aliases, description, detail, status)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
			e.Type, e.Name, aliases, e.Description, detail, insertStatus).Scan(&id)
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

// ListEntitiesByIDs 按 id 批量取(供 publish.BuildEntityCardData 按关系反查 partner 实体)。
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
	rows, err := s.Pool.Query(ctx, `SELECT `+entityCols+` FROM entities ORDER BY type, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
}

// ListEntitiesByStatus 按状态取实体(设计 frontend-ia §2.3:自选 = status='watch')。
func (s *Store) ListEntitiesByStatus(ctx context.Context, status string) ([]model.Entity, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+entityCols+` FROM entities WHERE status=$1 ORDER BY name`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Entity])
}

// CompanyNamesByCodes 批量取 6 位代码 → 公司名(展示用「名称(代码)」,issue #2)。
// 同名多行时取最早创建的一行(与 GetCompanyEntityByCode 一致);未建实体的 code 不在结果里。
func (s *Store) CompanyNamesByCodes(ctx context.Context, codes []string) (map[string]string, error) {
	out := map[string]string{}
	if len(codes) == 0 {
		return out, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT DISTINCT ON (detail->>'code') detail->>'code', name FROM entities
		 WHERE type='company' AND detail->>'code' = ANY($1)
		 ORDER BY detail->>'code', created_at`, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var code, name string
		if err := rows.Scan(&code, &name); err != nil {
			return nil, err
		}
		out[code] = name
	}
	return out, rows.Err()
}

// SetEntityStatus 显式置状态(自选镜像:watch 加入 / archived 移出)。返回是否命中实体。
func (s *Store) SetEntityStatus(ctx context.Context, id, status string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE entities SET status=$2, updated_at=now() WHERE id=$1`, id, status)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
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
