package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const sourceCols = `id,name,source_type,config,status,created_at,updated_at`

func (s *Store) CreateSource(ctx context.Context, src *model.Source) error {
	cfg := src.Config
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO sources(name,source_type,config,status) VALUES($1,$2,$3,$4)`,
		src.Name, src.SourceType, cfg, defaultStr(src.Status, "active"))
	return err
}

func (s *Store) GetSourceByName(ctx context.Context, name string) (model.Source, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+sourceCols+` FROM sources WHERE name=$1`, name)
	if err != nil {
		return model.Source{}, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Source])
}

func (s *Store) ListActiveSources(ctx context.Context) ([]model.Source, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+sourceCols+` FROM sources WHERE status='active' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Source])
}

// PauseSource 源健康监控:连续失败后置为 paused。
func (s *Store) PauseSource(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE sources SET status='paused', updated_at=now() WHERE id=$1 AND status!='paused'`, id)
	return err
}
