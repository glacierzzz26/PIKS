package store

import (
	"context"

	"piks/internal/config"
)

// ListAppConfig 读取 app_config 全表,返回 key→value。
// AI 配置权威源(代码不再读环境变量);密钥在此表明文存储(lab 单机可信)。
func (s *Store) ListAppConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT key, value FROM app_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// UpsertAppConfig 写入/更新单个配置项。密钥留空由调用方过滤(不改动现值)。
func (s *Store) UpsertAppConfig(ctx context.Context, key, value string) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO app_config (key, value) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value)
	return err
}

// ApplyAppConfig 把 app_config 表合并进 cfg(AI 配置权威源 = 库,代码不读环境变量)。
func (s *Store) ApplyAppConfig(ctx context.Context, cfg *config.Config) error {
	m, err := s.ListAppConfig(ctx)
	if err != nil {
		return err
	}
	cfg.ApplyAppConfig(m)
	return nil
}
