package store

// market_snapshots 存取(迭代 2,设计 §2.1;迁移 0003)。

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

// UpsertMarketSnapshot 按 trade_date upsert(一日一行)。
func (s *Store) UpsertMarketSnapshot(ctx context.Context, snap *model.MarketSnapshot) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO market_snapshots (
			trade_date, index_json, turnover_amt, breadth, limit_up_count, limit_down_count,
			broken_limit_count, max_board, zt_pool, strong_yesterday, industry_dist,
			hot_topics, top_events, capital_flow, emotion_score, emotion_state,
			emotion_detail, my_judgment, evidence, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19, now())
		ON CONFLICT (trade_date) DO UPDATE SET
			index_json=EXCLUDED.index_json, turnover_amt=EXCLUDED.turnover_amt,
			breadth=EXCLUDED.breadth, limit_up_count=EXCLUDED.limit_up_count,
			limit_down_count=EXCLUDED.limit_down_count, broken_limit_count=EXCLUDED.broken_limit_count,
			max_board=EXCLUDED.max_board, zt_pool=EXCLUDED.zt_pool,
			strong_yesterday=EXCLUDED.strong_yesterday, industry_dist=EXCLUDED.industry_dist,
			hot_topics=EXCLUDED.hot_topics, top_events=EXCLUDED.top_events,
			capital_flow=EXCLUDED.capital_flow, emotion_score=EXCLUDED.emotion_score,
			emotion_state=EXCLUDED.emotion_state, emotion_detail=EXCLUDED.emotion_detail,
			my_judgment=EXCLUDED.my_judgment, evidence=EXCLUDED.evidence, updated_at=now()`,
		snap.TradeDate, snap.IndexJSON, snap.TurnoverAmt, snap.Breadth, snap.LimitUpCount, snap.LimitDownCount,
		snap.BrokenLimitCount, snap.MaxBoard, snap.ZTPool, snap.StrongYesterday, snap.IndustryDist,
		snap.HotTopics, snap.TopEvents, snap.CapitalFlow, snap.EmotionScore, snap.EmotionState,
		snap.EmotionDetail, snap.MyJudgment, snap.Evidence)
	return err
}

// GetMarketSnapshotByDate 读某日快照(昨日强势股等依赖前一日)。
func (s *Store) GetMarketSnapshotByDate(ctx context.Context, tradeDate time.Time) (*model.MarketSnapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id,trade_date,index_json,turnover_amt,breadth,limit_up_count,limit_down_count,
			broken_limit_count,max_board,zt_pool,strong_yesterday,industry_dist,hot_topics,top_events,
			capital_flow,emotion_score,emotion_state,emotion_detail,my_judgment,evidence,created_at,updated_at
		FROM market_snapshots WHERE trade_date=$1`, tradeDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snap, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.MarketSnapshot])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &snap, err
}

// ListMarketSnapshots 最近 N 日快照(倒序),供复盘/回填。
func (s *Store) ListMarketSnapshots(ctx context.Context, limit int) ([]model.MarketSnapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id,trade_date,index_json,turnover_amt,breadth,limit_up_count,limit_down_count,
			broken_limit_count,max_board,zt_pool,strong_yesterday,industry_dist,hot_topics,top_events,
			capital_flow,emotion_score,emotion_state,emotion_detail,my_judgment,evidence,created_at,updated_at
		FROM market_snapshots ORDER BY trade_date DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.MarketSnapshot])
}
