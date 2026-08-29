package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"piks/internal/model"
)

const rawDocCols = `id,source_id,external_id,url,title,content,content_hash,` +
	`published_at,retrieved_at,status,pipeline_version,error,created_at`

// InsertRawDocument 幂等插入;命中 (source_id, content_hash) 唯一约束时返回 (false, nil)。
// 注意:ON CONFLICT DO NOTHING 冲突时无错误,须用 RowsAffected()==1 判断是否真插入。
func (s *Store) InsertRawDocument(ctx context.Context, doc *model.RawDocument) (bool, error) {
	ct, err := s.Pool.Exec(ctx,
		`INSERT INTO raw_documents(source_id,external_id,url,title,content,content_hash,published_at,status)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (source_id, content_hash) DO NOTHING`,
		doc.SourceID, doc.ExternalID, doc.URL, doc.Title, doc.Content,
		doc.ContentHash, doc.PublishedAt, defaultStr(doc.Status, "raw"))
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

// RawDocWithSource 快讯流只读投影(api_v1):raw_documents + 来源名 + 关联事件 id。
type RawDocWithSource struct {
	ID      string    `db:"id"`
	FlashAt time.Time `db:"flash_at"`
	Title   string    `db:"title"`
	Source  string    `db:"source"`
	EventID *string   `db:"event_id"`
}

// ListRawDocumentsWithSource 全部快讯(按发生时间倒序);被抽取成事件的行链上 event_id。
// 一文档多事件时取最早事件;published_at 缺失时回退 retrieved_at/created_at。
func (s *Store) ListRawDocumentsWithSource(ctx context.Context) ([]RawDocWithSource, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, flash_at, title, source, event_id FROM (
			SELECT DISTINCT ON (rd.id)
				rd.id,
				COALESCE(rd.published_at, rd.retrieved_at, rd.created_at) AS flash_at,
				rd.title,
				s.name AS source,
				ev.id AS event_id
			FROM raw_documents rd
			JOIN sources s ON s.id=rd.source_id
			LEFT JOIN events ev ON ev.raw_document_id=rd.id
			ORDER BY rd.id, ev.created_at
		) t ORDER BY flash_at DESC NULLS LAST`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[RawDocWithSource])
}
