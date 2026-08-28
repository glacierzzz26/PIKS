package store

// trades / positions 存取(交易功能,design trades.md;迁移 0010)。
// trades=交易记录(结构化事实),positions=持仓快照(只存只展示)。

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const tradeCols = `id,trade_date,code,name,side,price,qty,amount,source,attachment_id,note,review,created_at,updated_at`
const positionCols = `id,snapshot_date,code,name,qty,cost_price,price,market_value,pl,source,attachment_id,created_at`

// InsertTrades 批量入库交易(事务内逐条 INSERT)。
func (s *Store) InsertTrades(ctx context.Context, ts []model.Trade) error {
	if len(ts) == 0 {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, t := range ts {
		if _, err := tx.Exec(ctx, `
			INSERT INTO trades (trade_date, code, name, side, price, qty, amount, source, attachment_id, note)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			t.TradeDate, t.Code, t.Name, t.Side, t.Price, t.Qty, t.Amount,
			t.Source, t.AttachmentID, t.Note); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListTrades 交易列表(按交易日期倒序,同日期按创建倒序)。
func (s *Store) ListTrades(ctx context.Context, limit int) ([]model.Trade, error) {
	q := `SELECT ` + tradeCols + ` FROM trades ORDER BY trade_date DESC, created_at DESC`
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
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Trade])
}

// GetTrade 按 id 取交易。
func (s *Store) GetTrade(ctx context.Context, id string) (*model.Trade, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+tradeCols+` FROM trades WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	t, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Trade])
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SetTradeReview 写入 AI 复盘(覆盖),updated_at=now。
func (s *Store) SetTradeReview(ctx context.Context, id string, review []byte) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE trades SET review=$2, updated_at=now() WHERE id=$1`, id, review)
	return err
}

// TradeExists 判断同日期同代码同方向同数量交易是否已存在(导入去重提示用)。
func (s *Store) TradeExists(ctx context.Context, date time.Time, code, side string, qty int) (bool, error) {
	var one int
	err := s.Pool.QueryRow(ctx,
		`SELECT 1 FROM trades WHERE trade_date=$1 AND code=$2 AND side=$3 AND qty=$4 LIMIT 1`,
		date, code, side, qty).Scan(&one)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// InsertPositions 批量入库持仓快照。
func (s *Store) InsertPositions(ctx context.Context, ps []model.Position) error {
	if len(ps) == 0 {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, p := range ps {
		if _, err := tx.Exec(ctx, `
			INSERT INTO positions (snapshot_date, code, name, qty, cost_price, price, market_value, pl, source, attachment_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			p.SnapshotDate, p.Code, p.Name, p.Qty, p.CostPrice, p.Price, p.MarketValue, p.PL, p.Source, p.AttachmentID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// LatestPositions 最近一个快照日的持仓(按 snapshot_date 最大;同日按创建倒序)。
func (s *Store) LatestPositions(ctx context.Context) ([]model.Position, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+positionCols+` FROM positions
		WHERE snapshot_date = (SELECT max(snapshot_date) FROM positions)
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Position])
}
