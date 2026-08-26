package store

// observations 存取(迭代 2,设计 §2.3;表 0001 已建)。指标字典见设计 §2.3。

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

// UpsertObservation 按 market+indicator+observed_at 去重;同值跳过返回 false,写入/更新返回 true。
func (s *Store) UpsertObservation(ctx context.Context, obs model.Observation) (bool, error) {
	var existing string
	err := s.Pool.QueryRow(ctx,
		`SELECT value FROM observations WHERE market=$1 AND indicator=$2 AND observed_at=$3`,
		obs.Market, obs.Indicator, obs.ObservedAt).Scan(&existing)
	switch {
	case err == nil:
		if existing == obs.Value {
			return false, nil // 同值跳过
		}
		_, err = s.Pool.Exec(ctx,
			`UPDATE observations SET value=$4, previous_value=$5, change=$6, source=$7
			 WHERE market=$1 AND indicator=$2 AND observed_at=$3`,
			obs.Market, obs.Indicator, obs.ObservedAt, obs.Value, obs.PreviousVal, obs.Change, obs.Source)
		return true, err
	case errors.Is(err, pgx.ErrNoRows):
		_, err = s.Pool.Exec(ctx,
			`INSERT INTO observations (event_id, market, indicator, value, previous_value, change, observed_at, source)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			obs.EventID, obs.Market, obs.Indicator, obs.Value, obs.PreviousVal, obs.Change, obs.ObservedAt, obs.Source)
		return true, err
	default:
		return false, err
	}
}

// ListObservationsByDate 某交易日(UTC 零点)的全部观测,供 market-state 聚合。
func (s *Store) ListObservationsByDate(ctx context.Context, day time.Time) ([]model.Observation, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,event_id,market,indicator,value,previous_value,change,observed_at,source,created_at
		 FROM observations WHERE observed_at >= $1 AND observed_at < $1 + interval '1 day'
		 ORDER BY market, indicator`,
		day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.Observation])
	return out, err
}
