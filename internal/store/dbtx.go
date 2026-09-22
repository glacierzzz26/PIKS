package store

// DBTX 是 *pgxpool.Pool 与 pgx.Tx 的公共子集(刻意只声明用得到的方法)。
// 用途:同一段 SQL 逻辑既能在连接池上跑,也能在事务里跑 —— 自选同步需要
// 「整轮原子」(跨 entities + watchlist_entries 两张表,任一步失败回滚)。
//
// 用法:公开方法(如 UpsertWatchlistEntry)在池上跑;*Tx 变体接收 DBTX,
// 由调用方用 Store.Begin/Commit 包住多步(见 internal/watchsync/apply.go)。

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBTX 池或事务(两者都满足)。
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Tx 一次事务(内嵌 pgx.Tx,只暴露 Commit/Rollback 语义)。
type Tx struct{ pgx.Tx }

// Begin 开一个事务。调用方**必须** defer Rollback(Commit 后 Rollback 是 no-op)。
func (s *Store) Begin(ctx context.Context) (*Tx, error) {
	t, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &Tx{t}, nil
}

// 编译期确认两种实现都满足 DBTX(改签名时立刻报错,而非运行期才发现)。
var (
	_ DBTX = (*pgxpool.Pool)(nil)
	_ DBTX = (*Tx)(nil)
)
