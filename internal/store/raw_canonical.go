package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// raw 层转载组回填的取数/落库(issue #83 分期 P-4)。纯分组判据在 `internal/cluster/reprint_raw.go`,
// 本文件只管「读原始行 + 写回代表」(照仓库惯例:判据与 IO 分离)。

// RawDocForGrouping 分组所需的最小投影(只取指纹与代表选取用到的列,不拉正文之外的大字段)。
type RawDocForGrouping struct {
	ID          string    `db:"id"`
	SourceID    string    `db:"source_id"`
	Content     string    `db:"content"`
	RetrievedAt time.Time `db:"retrieved_at"`
	// CanonicalID 既有代表。非 NULL = 已冻结(P-2/P4 代表冻结纪律),本轮回填**不重选**。
	CanonicalID *string `db:"canonical_id"`
}

// ListRawDocumentsForGrouping 取用于 raw 层转载分组的文档。
//
// 🔴 只取 `origin_kind='pipeline'`:raw 层分组服务于**事件层展示**(哪些渠道报了同一篇稿),
// 而事件只从 pipeline 采集的文档抽出(P-2 门控)。realtime 行(P-5)不参与,
// 否则会把实时层的行并进正式管线的来源列表(同 P-2 门控的分道意图)。
//
// since 为零值时不限窗口;否则只取 `retrieved_at >= since`。不按 status 过滤 ——
// 转载组是**采集事实**,与是否已被抽取/被粗筛(processed/raw/failed/deferred)无关;
// 某行被粗筛掉也仍应出现在「谁报了这篇稿」里。
//
// 稳定序 `ORDER BY retrieved_at, id`:与 `RawGroups` 的代表选取同序,便于幂等核对。
func (s *Store) ListRawDocumentsForGrouping(ctx context.Context, since time.Time) ([]RawDocForGrouping, error) {
	q := `SELECT id, source_id, content, retrieved_at, canonical_id
	      FROM raw_documents WHERE origin_kind='pipeline'`
	args := []any{}
	if !since.IsZero() {
		q += ` AND retrieved_at >= $1`
		args = append(args, since)
	}
	q += ` ORDER BY retrieved_at, id`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[RawDocForGrouping])
}

// SetRawCanonicalIDs 把 [id, repID] 逐对写回 `canonical_id`,**只写仍为 NULL 的行**。
//
// 🔴 `AND canonical_id IS NULL` 是**代表冻结**的落地点(与 `event_clusters.canonical_event_id`
// 同一纪律,见迁移 0022 注释):已非 NULL 的行**永不重选** —— 否则「今天 A 代表、明天 B 代表」
// 会让 url 归属漂移。返回实际改动的行数(已冻结的行不计入),调用方据此判断是否幂等收敛。
func (s *Store) SetRawCanonicalIDs(ctx context.Context, pairs [][2]string) (int64, error) {
	if len(pairs) == 0 {
		return 0, nil
	}
	ids := make([]string, len(pairs))
	reps := make([]string, len(pairs))
	for i, p := range pairs {
		ids[i], reps[i] = p[0], p[1]
	}
	ct, err := s.Pool.Exec(ctx, `
		UPDATE raw_documents rd
		SET canonical_id = v.rep_id::uuid, updated_at = now()
		FROM unnest($1::text[], $2::text[]) AS v(id, rep_id)
		WHERE rd.id = v.id::uuid AND rd.canonical_id IS NULL`, ids, reps)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}
