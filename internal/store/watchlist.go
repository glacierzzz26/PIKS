package store

// 自选加入价/加入日读写(表 watchlist_entries,迁移 0020)。
//
// 🔴 隔离红线:本文件**只碰 watchlist_entries**。自选「成员资格」真源是 entities.status,
//   本表**只**存加入价/日 —— 因 entities.detail 每轮被 entity-build 无条件覆盖(迁移 0020 头注)。
//   任何把加入价/日写回 entities.detail 的改动都是错的。

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const watchEntryCols = `code,entity_id,market,market_id,added_price,added_on,removed_at,first_seen_at,last_synced_at,extra,created_at,updated_at`

// UpsertWatchlistEntry 落一条自选元数据(幂等)。语义:
//
//   - **首次**:插入,removed_at 保持 NULL(当前在自选)。
//   - **在选重复同步(keep)**:价/日取上游新值;上游缺失(NULL)时回退既有值
//     (COALESCE)—— 防上游 detail 抖动/缺字段把历史抹成空。entity_id 同理 COALESCE。
//   - **re-add(曾移出后再加入)**:removed_at 清回 NULL,价/日**用上游新值覆盖**
//     (同花顺 re-add 会重新给价/日),extra.readd_count 自增(审计)。
//
// price/addedOn 为 nil 表示「上游未给」—— 存 NULL,**绝不填 0**。
func (s *Store) UpsertWatchlistEntry(
	ctx context.Context,
	code string,
	entityID *string,
	market, marketID string,
	price *float64,
	addedOn *time.Time,
	extra json.RawMessage,
) error {
	return UpsertWatchlistEntryTx(ctx, s.Pool, code, entityID, market, marketID, price, addedOn, extra)
}

// UpsertWatchlistEntryTx 同上,但在调用方给定的事务/池上执行(自选同步整轮原子)。
func UpsertWatchlistEntryTx(
	ctx context.Context,
	q DBTX,
	code string,
	entityID *string,
	market, marketID string,
	price *float64,
	addedOn *time.Time,
	extra json.RawMessage,
) error {
	if len(extra) == 0 {
		extra = json.RawMessage(`{}`)
	}
	_, err := q.Exec(ctx, `
		INSERT INTO watchlist_entries
			(code, entity_id, market, market_id, added_price, added_on, last_synced_at, extra)
		VALUES ($1,$2,$3,$4,$5,$6, now(), $7)
		ON CONFLICT (code) DO UPDATE SET
			entity_id = COALESCE(EXCLUDED.entity_id, watchlist_entries.entity_id),
			market    = EXCLUDED.market,
			market_id = EXCLUDED.market_id,
			-- 在选(removed_at IS NULL):上游有新值就用新值,缺则保留旧值(COALESCE)。
			-- re-add(removed_at IS NOT NULL):视为全新加入,直接取上游新值。
			added_price = CASE WHEN watchlist_entries.removed_at IS NOT NULL
			                   THEN EXCLUDED.added_price
			                   ELSE COALESCE(EXCLUDED.added_price, watchlist_entries.added_price) END,
			added_on    = CASE WHEN watchlist_entries.removed_at IS NOT NULL
			                   THEN EXCLUDED.added_on
			                   ELSE COALESCE(EXCLUDED.added_on, watchlist_entries.added_on) END,
			removed_at     = NULL,
			last_synced_at = now(),
			extra = jsonb_set(EXCLUDED.extra, '{readd_count}', to_jsonb(
				COALESCE((watchlist_entries.extra->>'readd_count')::int, 0)
				+ CASE WHEN watchlist_entries.removed_at IS NOT NULL THEN 1 ELSE 0 END)),
			updated_at = now()`,
		code, entityID, market, marketID, price, addedOn, extra)
	return err
}

// MarkWatchlistRemoved 把不在上游快照里的 code 标记为「已移出」:removed_at=now()。
// **不删行**(与 entities.status='archived' 同精神);已是移出态则不动(幂等,不刷时间戳)。
// 返回值为是否真的发生了状态变更(供 Stats 统计)。
func (s *Store) MarkWatchlistRemoved(ctx context.Context, code string) (bool, error) {
	return MarkWatchlistRemovedTx(ctx, s.Pool, code)
}

// MarkWatchlistRemovedTx 同上,但在调用方给定的事务/池上执行。
func MarkWatchlistRemovedTx(ctx context.Context, q DBTX, code string) (bool, error) {
	tag, err := q.Exec(ctx,
		`UPDATE watchlist_entries SET removed_at=now(), updated_at=now()
		 WHERE code=$1 AND removed_at IS NULL`, code)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// EntityIDByCode 取某 code 已建实体的 id;无 → (nil, nil)。名称未解析时不建实体,
// 故可能查不到(下轮补)。取最早创建的一行(与 GetCompanyEntityByCode 同口径)。
func (s *Store) EntityIDByCode(ctx context.Context, code string) (*string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`SELECT id FROM entities WHERE type='company' AND detail->>'code'=$1
		 ORDER BY created_at LIMIT 1`, code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// ListWatchlistEntries 取**当前在自选**的元数据行(removed_at IS NULL)。
// 供 /api/v1/watchlist 富化(按 code 索引)。
func (s *Store) ListWatchlistEntries(ctx context.Context) ([]model.WatchlistEntry, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+watchEntryCols+` FROM watchlist_entries WHERE removed_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.WatchlistEntry])
}

// TaskRunSlotDone 判定「某 command 在 since 之后是否已有一个 meta.slot=slot 的**成功**记录」。
// 常驻 watch-sync 重启去重的依据 —— ⚠️ 只有 success 才去重:**failed 必须允许重试**。
func (s *Store) TaskRunSlotDone(ctx context.Context, command, slot string, since time.Time) (bool, error) {
	var done bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM task_runs
			WHERE command=$1 AND meta->>'slot'=$2 AND started_at >= $3 AND status='success')`,
		command, slot, since).Scan(&done)
	return done, err
}

// CountTaskRunsSince 统计某 command 自 since 起、指定 status 的运行次数。
// watch-sync 用来实施「登录失败硬上限 6 次/日」(防重试风暴触发风控/锁号)。
func (s *Store) CountTaskRunsSince(ctx context.Context, command, status string, since time.Time) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM task_runs WHERE command=$1 AND status=$2 AND started_at >= $3`,
		command, status, since).Scan(&n)
	return n, err
}
