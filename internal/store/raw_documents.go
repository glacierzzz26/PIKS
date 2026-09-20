package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"piks/internal/model"
)

const rawDocCols = `id,source_id,external_id,url,title,content,content_hash,` +
	`published_at,retrieved_at,status,pipeline_version,error,extra,created_at`

// InsertRawDocument 幂等插入;命中 (source_id, content_hash) 唯一约束时返回 (false, nil)。
// 注意:ON CONFLICT DO NOTHING 冲突时无错误,须用 RowsAffected()==1 判断是否真插入。
// extra 为该源上游原始字段原样留存(issue #43);传 nil 落 '{}'。
func (s *Store) InsertRawDocument(ctx context.Context, doc *model.RawDocument) (bool, error) {
	extra := doc.Extra
	if len(extra) == 0 {
		extra = json.RawMessage(`{}`)
	}
	ct, err := s.Pool.Exec(ctx,
		`INSERT INTO raw_documents(source_id,external_id,url,title,content,content_hash,published_at,status,extra)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (source_id, content_hash) DO NOTHING`,
		doc.SourceID, doc.ExternalID, doc.URL, doc.Title, doc.Content,
		doc.ContentHash, doc.PublishedAt, defaultStr(doc.Status, "raw"), extra)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return false, nil // 并发竞态下的重复
		}
		return false, err
	}
	return ct.RowsAffected() == 1, nil
}

func (s *Store) ListRawPending(ctx context.Context, limit int) ([]model.RawDocument, error) {
	return s.ListRawPendingStatus(ctx, limit, false)
}

// ListRawPendingStatus 取待处理文档;includeFailed=true 时含 failed(重试场景)。
func (s *Store) ListRawPendingStatus(ctx context.Context, limit int, includeFailed bool) ([]model.RawDocument, error) {
	q := `SELECT ` + rawDocCols + ` FROM raw_documents WHERE status='raw'`
	if includeFailed {
		q = `SELECT ` + rawDocCols + ` FROM raw_documents WHERE status IN ('raw','failed')`
	}
	q += ` ORDER BY retrieved_at LIMIT $1`
	rows, err := s.Pool.Query(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.RawDocument])
}

func (s *Store) MarkRawProcessed(ctx context.Context, id string, pipelineVersion string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE raw_documents SET status='processed', pipeline_version=$2, error=NULL, updated_at=now() WHERE id=$1`,
		id, pipelineVersion)
	return err
}

func (s *Store) MarkRawFailed(ctx context.Context, id string, errMsg string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE raw_documents SET status='failed', error=$2 WHERE id=$1`, id, errMsg)
	return err
}

func (s *Store) GetRawDocumentByID(ctx context.Context, id string) (model.RawDocument, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+rawDocCols+` FROM raw_documents WHERE id=$1`, id)
	if err != nil {
		return model.RawDocument{}, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.RawDocument])
}

// FlashSort 快讯流排序维度。默认(空/FlashSortTime)= 时间倒序(最新在前)。
const (
	FlashSortTime      = "time"
	FlashSortImportant = "important"
)

// flashOrderBy 把排序维度映射为 SQL ORDER BY 子句(白名单)。
// 「重要优先」用**源自带重要度**(source_important:important=1 或 confirmed=1,见 ListRawDocumentsWithSource)。
// 此前以「已被抽取成事件(event_id IS NOT NULL)」作代理 —— 多源后源字段更准(issue #43),
// 且不再把「是否被抽取」误当「是否重要」。
func flashOrderBy(sort string) string {
	if sort == FlashSortImportant {
		return `ORDER BY source_important DESC, flash_at DESC`
	}
	return `ORDER BY flash_at DESC`
}

// RawDocWithSource 快讯流只读投影(api_v1):raw_documents + 来源名 + 关联事件 id + 原文 url。
type RawDocWithSource struct {
	ID      string    `db:"id"`
	FlashAt time.Time `db:"flash_at"`
	Title   string    `db:"title"`
	Source  string    `db:"source"`
	EventID *string   `db:"event_id"`
	URL     *string   `db:"url"`
	// Important 源自带的重要度(issue #43):上游 important=1 或 confirmed=1 即为真。
	// 快讯源此前无独立标记,只能拿「已被抽取成事件」近似 —— 多源后直接用源字段,更准。
	Important bool `db:"source_important"`
}

// ListRawDocumentsWithSource 全部快讯;被抽取成事件的行链上 event_id。
// sort 见 FlashSort*(空 = 时间倒序)。
// 一文档多事件时取最早事件;published_at 缺失时回退 retrieved_at/created_at。
// title 可空:多源后部分源(金十)无独立标题字段,标题由正文前段派生,派生失败即 NULL
// → COALESCE 到正文,保证快讯行始终可读(issue #43)。
func (s *Store) ListRawDocumentsWithSource(ctx context.Context, sort string) ([]RawDocWithSource, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, flash_at, title, source, event_id, url, source_important FROM (
			SELECT DISTINCT ON (rd.id)
				rd.id,
				COALESCE(rd.published_at, rd.retrieved_at, rd.created_at) AS flash_at,
				COALESCE(rd.title, rd.content) AS title,
				s.name AS source,
				ev.id AS event_id,
				rd.url,
				COALESCE((rd.extra->>'important')::int, 0) = 1
					OR COALESCE((rd.extra->>'confirmed')::int, 0) = 1 AS source_important
			FROM raw_documents rd
			JOIN sources s ON s.id=rd.source_id
			LEFT JOIN events ev ON ev.raw_document_id=rd.id
			ORDER BY rd.id, ev.created_at
		) t `+flashOrderBy(sort))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[RawDocWithSource])
}
