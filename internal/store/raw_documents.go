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

// ⚠️ 逗号后**不得留空格**:events.go 的限定列名靠 `strings.ReplaceAll(cols, ",", ",e.")` 派生。
const rawDocCols = `id,source_id,external_id,url,title,content,content_hash,` +
	`published_at,retrieved_at,status,grade,origin_kind,canonical_id,pipeline_version,error,extra,created_at`

// InsertRawDocument 幂等插入;命中去重索引时返回 (false, nil)。
// 注意:ON CONFLICT DO NOTHING 冲突时无错误,须用 RowsAffected()==1 判断是否真插入。
// extra 为该源上游原始字段原样留存(issue #43);传 nil 落 '{}'。
//
// 去重键按源的能力分派(迁移 0016,issue #50):不带 external_id 的源按
// (source_id, content_hash) 去重;带 external_id 的源按
// (source_id, external_id, content_hash) 去重。两条皆为 **partial unique index**,
// 故这里**不能**写带键名的 ON CONFLICT(…)(partial index 无法作为推断目标,
// 且两侧键不同无法用同一条表达式覆盖)。用无目标的 ON CONFLICT DO NOTHING:
// 任一唯一索引冲突即跳过 —— 语义明确,且新增去重维度时无需再改此处。
func (s *Store) InsertRawDocument(ctx context.Context, doc *model.RawDocument) (bool, error) {
	extra := doc.Extra
	if len(extra) == 0 {
		extra = json.RawMessage(`{}`)
	}
	ct, err := s.Pool.Exec(ctx,
		`INSERT INTO raw_documents(source_id,external_id,url,title,content,content_hash,published_at,status,grade,origin_kind,canonical_id,extra)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 ON CONFLICT DO NOTHING`,
		doc.SourceID, doc.ExternalID, doc.URL, doc.Title, doc.Content,
		doc.ContentHash, doc.PublishedAt, defaultStr(doc.Status, "raw"), doc.Grade,
		defaultStr(doc.OriginKind, "pipeline"), doc.CanonicalID, extra)
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
//
// ⚠️ origin_kind='pipeline' 是**契约**(issue #83 P-2):worker 只抽取正式管线采集的文档,
// 实时层(P-5,origin_kind='realtime')结构上无法被抽进 events。今日无 realtime 行,
// 该过滤是 no-op,但**不得**因「当前无影响」删掉 —— 它是 P-5 落地后唯一的分道闸。
func (s *Store) ListRawPendingStatus(ctx context.Context, limit int, includeFailed bool) ([]model.RawDocument, error) {
	q := `SELECT ` + rawDocCols + ` FROM raw_documents WHERE status='raw' AND origin_kind='pipeline'`
	if includeFailed {
		q = `SELECT ` + rawDocCols + ` FROM raw_documents WHERE status IN ('raw','failed') AND origin_kind='pipeline'`
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
//
// ⚠️ **排除 source_type='announcement'**(issue #50):公告是官方披露、非「快讯」语义,
// 且有独立的 /api/v1/announcements 投影。不加此过滤,公告会混进快讯 tab
// (编译期无强制,只有这条 SQL 把关)。
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
			WHERE s.source_type <> 'announcement'
			ORDER BY rd.id, ev.created_at
		) t `+flashOrderBy(sort))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[RawDocWithSource])
}

// RawDocAnnouncement 公告只读投影(api_v1):raw_documents 里的原始事件源,
// 按 sources.source_type='announcement' 过滤(issue #50)。
// SecCode/SecName/PageColumn 取自采集时留存的 extra(巨潮列);缺失时为空,不造值。
type RawDocAnnouncement struct {
	ID          string    `db:"id"`
	AnnouncedAt time.Time `db:"announced_at"`
	Title       string    `db:"title"`
	Source      string    `db:"source"`
	URL         *string   `db:"url"`
	SecCode     *string   `db:"sec_code"`
	SecName     *string   `db:"sec_name"`
	PageColumn  *string   `db:"page_column"`
	Grade       *string   `db:"grade"` // 分级(issue #68);历史行为 NULL
}

// ListAnnouncementsWithSource 公告流(仅 source_type='announcement'),时间倒序。
// 时间口径与快讯一致:published_at 缺失回退 retrieved_at/created_at。
// grade(issue #68 A 层)随投影下发,供前端按级折叠;NULL=未分级(本迁移前的历史行)。
func (s *Store) ListAnnouncementsWithSource(ctx context.Context) ([]RawDocAnnouncement, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT rd.id,
			COALESCE(rd.published_at, rd.retrieved_at, rd.created_at) AS announced_at,
			COALESCE(rd.title, rd.content) AS title,
			s.name AS source,
			rd.url,
			rd.extra->>'sec_code'    AS sec_code,
			rd.extra->>'sec_name'    AS sec_name,
			rd.extra->>'page_column' AS page_column,
			rd.grade
		FROM raw_documents rd
		JOIN sources s ON s.id=rd.source_id
		WHERE s.source_type='announcement'
		ORDER BY announced_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[RawDocAnnouncement])
}
