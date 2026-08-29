package store

// 周报 AI 综述缓存(iter4 D26,design weekly-ai-summary.md)。
// 综述每周一次、高智档生成,落 weekly_summaries 表按 ISO 周缓存;重新生成 = UPSERT 覆盖。

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// WeeklySummary 某周 AI 综述(缓存行)。
type WeeklySummary struct {
	ID        string    `db:"id"`
	Week      string    `db:"week"`
	Summary   string    `db:"summary"`
	Model     string    `db:"model"`
	Tokens    int64     `db:"tokens"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// GetWeeklySummary 按周取综述;无缓存返回 nil。
func (s *Store) GetWeeklySummary(ctx context.Context, week string) (*WeeklySummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,week,summary,model,tokens,created_at,updated_at FROM weekly_summaries WHERE week=$1`, week)
	if err != nil {
		return nil, err
	}
	w, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[WeeklySummary])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// ListWeeklySummaries 全部周综述,按周倒序(React 周报页投影)。
func (s *Store) ListWeeklySummaries(ctx context.Context) ([]WeeklySummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,week,summary,model,tokens,created_at,updated_at FROM weekly_summaries ORDER BY week DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[WeeklySummary])
}

// GetWeeklySummaryByID 按 id 取周综述;无则 nil(React 周报阅读页经 /notes/:id 回退)。
func (s *Store) GetWeeklySummaryByID(ctx context.Context, id string) (*WeeklySummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,week,summary,model,tokens,created_at,updated_at FROM weekly_summaries WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	w, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[WeeklySummary])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// UpsertWeeklySummary 写/覆盖某周综述(week UNIQUE,重新生成覆盖不新增行)。
func (s *Store) UpsertWeeklySummary(ctx context.Context, week, summary, model string, tokens int64) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO weekly_summaries(week, summary, model, tokens, updated_at)
		 VALUES($1,$2,$3,$4, now())
		 ON CONFLICT (week) DO UPDATE
		   SET summary=EXCLUDED.summary, model=EXCLUDED.model, tokens=EXCLUDED.tokens, updated_at=now()`,
		week, summary, model, tokens)
	return err
}
