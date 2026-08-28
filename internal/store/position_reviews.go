package store

// 持仓 AI 诊断缓存(交易闭环,design trade-loop.md D-L1)。
// 诊断按 snapshot_date 缓存(一天一份),落 position_reviews 表;重新诊断 = UPSERT 覆盖。

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// PositionReview 某快照日的持仓诊断(缓存行)。
type PositionReview struct {
	ID           string          `db:"id"`
	SnapshotDate time.Time       `db:"snapshot_date"`
	Review       json.RawMessage `db:"review"`
	Model        string          `db:"model"`
	Tokens       int64           `db:"tokens"`
	CreatedAt    time.Time       `db:"created_at"`
	UpdatedAt    time.Time       `db:"updated_at"`
}

// GetPositionReview 按快照日取诊断;无缓存返回 nil。
func (s *Store) GetPositionReview(ctx context.Context, snapshotDate time.Time) (*PositionReview, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,snapshot_date,review,model,tokens,created_at,updated_at FROM position_reviews WHERE snapshot_date=$1`,
		snapshotDate)
	if err != nil {
		return nil, err
	}
	p, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[PositionReview])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

// UpsertPositionReview 写/覆盖某快照日诊断(snapshot_date UNIQUE,重新诊断覆盖不新增行)。
func (s *Store) UpsertPositionReview(ctx context.Context, snapshotDate time.Time, review json.RawMessage, model string, tokens int64) error {
	if len(review) == 0 {
		review = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO position_reviews(snapshot_date, review, model, tokens, updated_at)
		 VALUES($1,$2,$3,$4, now())
		 ON CONFLICT (snapshot_date) DO UPDATE
		   SET review=EXCLUDED.review, model=EXCLUDED.model, tokens=EXCLUDED.tokens, updated_at=now()`,
		snapshotDate, review, model, tokens)
	return err
}
