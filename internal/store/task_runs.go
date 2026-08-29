package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

// StartTaskRun 记录一次命令执行开始,返回 run id。
func (s *Store) StartTaskRun(ctx context.Context, command string) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO task_runs(command) VALUES($1) RETURNING id`, command).Scan(&id)
	return id, err
}

// FinishTaskRun 收尾记录:状态、耗时、错误、元数据(计数/token)。
func (s *Store) FinishTaskRun(ctx context.Context, id int64, status, errMsg string, meta map[string]any) error {
	raw, _ := json.Marshal(meta)
	_, err := s.Pool.Exec(ctx,
		`UPDATE task_runs SET status=$2, ended_at=now(), error=$3, meta=$4 WHERE id=$1`,
		id, status, nullIfEmpty(errMsg), raw)
	return err
}

func (s *Store) GetTaskRunByID(ctx context.Context, id int64) (model.TaskRun, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,command,status,started_at,ended_at,error,meta,created_at FROM task_runs WHERE id=$1`, id)
	if err != nil {
		return model.TaskRun{}, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.TaskRun])
}

// ListTaskRuns 最近任务执行,按开始时间倒序(React 看板管线状态投影)。
func (s *Store) ListTaskRuns(ctx context.Context, limit int) ([]model.TaskRun, error) {
	q := `SELECT id,command,status,started_at,ended_at,error,meta,created_at FROM task_runs ORDER BY started_at DESC`
	args := []any{}
	if limit > 0 {
		q += ` LIMIT $1`
		args = append(args, limit)
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.TaskRun])
}

// TokensSince 统计 since 以来 task_runs 累计 AI token(预算护栏用)。
func (s *Store) TokensSince(ctx context.Context, since time.Time) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM((meta->>'ai_tokens')::bigint),0) FROM task_runs WHERE created_at >= $1`, since).Scan(&n)
	return n, err
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
