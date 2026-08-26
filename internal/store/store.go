package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store 聚合所有表的 repository,唯一依赖 pgx pool。
type Store struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
