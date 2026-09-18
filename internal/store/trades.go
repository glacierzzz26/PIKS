package store

// trades / positions 存取(交易功能,design trades.md;迁移 0010)。
// trades=交易记录(结构化事实),positions=持仓快照(只存只展示)。

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const tradeCols = `id,trade_date,code,name,side,price,qty,amount,source,attachment_id,note,review,created_at,updated_at`
const positionCols = `id,snapshot_date,code,name,qty,cost_price,price,market_value,pl,source,attachment_id,created_at`

const insertTradeSQL = `
	INSERT INTO trades (trade_date, code, name, side, price, qty, amount, source, attachment_id, note)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

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
		if _, err := tx.Exec(ctx, insertTradeSQL,
			t.TradeDate, t.Code, t.Name, t.Side, t.Price, t.Qty, t.Amount,
			t.Source, t.AttachmentID, t.Note); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// InsertTradeReturningID 单条入库并返回新行 id —— 决策记录(P6-4)建边需要 from_id。
func (s *Store) InsertTradeReturningID(ctx context.Context, t model.Trade) (string, error) {
	var id string
	if err := s.Pool.QueryRow(ctx, insertTradeSQL+` RETURNING id`,
		t.TradeDate, t.Code, t.Name, t.Side, t.Price, t.Qty, t.Amount,
		t.Source, t.AttachmentID, t.Note).Scan(&id); err != nil {
		return "", err
	}
	return id, nil
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

const accountSnapshotCols = `id,snapshot_date,total_asset,total_mv,float_pl,daily_pl,source,attachment_id,created_at,updated_at`

// UpsertAccountSnapshot 账户级快照落库(issue #19):同日重传覆盖。
// 四项指针原样写入(NULL = 截图没这个数,不可与 0 混)。snapshot_date UNIQUE,
// 故 ON CONFLICT 覆盖 —— 与 position_reviews 的一天一份语义一致。
func (s *Store) UpsertAccountSnapshot(ctx context.Context, a model.AccountSnapshot) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO account_snapshots (snapshot_date, total_asset, total_mv, float_pl, daily_pl, source, attachment_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (snapshot_date) DO UPDATE SET
		  total_asset = EXCLUDED.total_asset, total_mv = EXCLUDED.total_mv,
		  float_pl = EXCLUDED.float_pl, daily_pl = EXCLUDED.daily_pl,
		  source = EXCLUDED.source, attachment_id = EXCLUDED.attachment_id,
		  updated_at = now()`,
		a.SnapshotDate, a.TotalAsset, a.TotalMV, a.FloatPL, a.DailyPL, a.Source, a.AttachmentID)
	return err
}

// LatestAccountSnapshot 最近一个快照日的账户汇总;无则 (nil, nil)。
// 与 LatestPositions 同口径取 max(snapshot_date) —— 两者同日,页面并列展示。
func (s *Store) LatestAccountSnapshot(ctx context.Context) (*model.AccountSnapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+accountSnapshotCols+` FROM account_snapshots
		WHERE snapshot_date = (SELECT max(snapshot_date) FROM account_snapshots)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	a, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.AccountSnapshot])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// LatestPositionsBefore 周末前最近快照日的持仓(防未来函数:仅用快照时点之前数据)。
// before 为周结束时刻;取 snapshot_date < before 的最大日;无则空。
func (s *Store) LatestPositionsBefore(ctx context.Context, before time.Time) ([]model.Position, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+positionCols+` FROM positions
		WHERE snapshot_date = (
		  SELECT max(snapshot_date) FROM positions WHERE snapshot_date < $1
		)
		ORDER BY created_at DESC`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Position])
}

// ListTradesBetween 日期区间内的交易(周报聚合用),按 trade_date 升序。
func (s *Store) ListTradesBetween(ctx context.Context, start, end time.Time) ([]model.Trade, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+tradeCols+` FROM trades
		WHERE trade_date >= $1 AND trade_date < $2
		ORDER BY trade_date ASC, created_at ASC`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Trade])
}

// ListTradesByCode 某股票代码的全部成交(个股中心用,设计 frontend-ia §2.4),
// 按 trade_date 倒序,命中 idx_trades_code(迁移 0010);代码经 NormalizeCode 归一。
func (s *Store) ListTradesByCode(ctx context.Context, code string, limit int) ([]model.Trade, error) {
	q := `SELECT ` + tradeCols + ` FROM trades WHERE code=$1
	      ORDER BY trade_date DESC, created_at DESC`
	args := []any{NormalizeCode(code)}
	if limit > 0 {
		q += ` LIMIT $2`
		args = append(args, limit)
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Trade])
}

// LatestPositionByCode 某代码最近一次出现在持仓快照里的行(个股中心用)。
// 注意语义区别于 LatestPositions:后者是"全组合最近快照日的所有持仓",
// 本方法是"该股最近一次被快照到的持仓"。无则该股未持仓 → (nil, nil)。
func (s *Store) LatestPositionByCode(ctx context.Context, code string) (*model.Position, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+positionCols+` FROM positions
		WHERE code=$1
		ORDER BY snapshot_date DESC, created_at DESC LIMIT 1`, NormalizeCode(code))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	p, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Position])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
