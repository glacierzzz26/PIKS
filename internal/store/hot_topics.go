package store

// 热榜快照读写(issue #68 D 层,表 hot_topic_items,迁移 0018)。
//
// 🔴 隔离红线:本文件**只碰 hot_topic_items**,绝不 join/写入 raw_documents 或 events ——
//   热度可被操纵,不得混入印证度(source_count/cluster_sources)或任何事件逻辑(设计 §6)。
//   若要「用热榜给事件排序」,那是被明确否决的做法,先读 docs/phase11/design/hot-topic.md 红线。

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const hotTopicCols = `id,source,rank,title,hot_value,url,extra,snapshot_at,created_at`

// InsertHotTopicItems 批量落一批快照(同批共用一个 snapshot_at,由调用方给定)。
// ⚠️ **无去重**(表无唯一约束):同一话题跨快照重复是**预期**行为 —— 热度走势靠它。
// 单事务批量插入;任一条失败整批回滚(快照应原子,不留半批)。
func (s *Store) InsertHotTopicItems(ctx context.Context, items []model.HotTopicItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, it := range items {
		extra := it.Extra
		if len(extra) == 0 {
			extra = json.RawMessage(`{}`)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO hot_topic_items(source,rank,title,hot_value,url,extra,snapshot_at)
			 VALUES($1,$2,$3,$4,$5,$6,$7)`,
			it.Source, it.Rank, it.Title, it.HotValue, it.URL, extra, it.SnapshotAt); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(items), nil
}

// ListHotTopicLatest 取**每个源**最新一批快照(按 source 分组取各自 max(snapshot_at)),
// 源内按 rank 升序。返回顺序按 source 名(稳定但无语义;前端按固定源清单分列)。
//
// 为何「每源各取最新」而非「取一个统一最新时刻」:两源采集时刻可能差几秒(串行采集),
// 用统一时刻会漏掉其一;各取最新才对。⚠️ 两列**各出各的,不合并**(设计 §6 方案 A)。
func (s *Store) ListHotTopicLatest(ctx context.Context) ([]model.HotTopicItem, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+hotTopicCols+` FROM (
			SELECT *, max(snapshot_at) OVER (PARTITION BY source) AS latest
			FROM hot_topic_items
		) t
		WHERE snapshot_at = latest
		ORDER BY source, rank`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.HotTopicItem])
}
