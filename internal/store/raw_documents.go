package store

import (
	"context"
	"errors"

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
	rows, err := s.Pool.Query(ctx,
		`SELECT `+rawDocCols+` FROM raw_documents WHERE status='raw' ORDER BY retrieved_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.RawDocument])
}

func (s *Store) MarkRawProcessed(ctx context.Context, id string, pipelineVersion string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE raw_documents SET status='processed', pipeline_version=$2, updated_at=now() WHERE id=$1`,
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
