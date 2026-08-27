package store

// personal_notes 存取(迭代 5-2,Web 编辑,权威源=PG;迁移 0006)。
// 关联复用 relationships:from_type='personal_note',rel_type='references'(↔event/entity)。

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const noteCols = `id,type,slug,title,status,confidence,content,detail,author,updated_by,created_at,updated_at`

// CreatePersonalNote 新建笔记,返回 id。detail 缺省 {}
func (s *Store) CreatePersonalNote(ctx context.Context, n *model.PersonalNote) (string, error) {
	detail := n.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	var id string
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO personal_notes (type, slug, title, status, confidence, content, detail, author)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		n.Type, n.Slug, n.Title, defaultStr(n.Status, "hypothesis"), n.Confidence, n.Content, detail, defaultStr(n.Author, "me")).
		Scan(&id)
	return id, err
}

// UpdatePersonalNote 按 id 更新(title/status/confidence/content/detail),updated_at=now。
func (s *Store) UpdatePersonalNote(ctx context.Context, n *model.PersonalNote) error {
	detail := n.Detail
	if len(detail) == 0 {
		detail = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx, `
		UPDATE personal_notes
		SET title=$2, status=$3, confidence=$4, content=$5, detail=$6, updated_by=$7, updated_at=now()
		WHERE id=$1`,
		n.ID, n.Title, n.Status, n.Confidence, n.Content, detail, n.UpdatedBy)
	return err
}

// GetPersonalNote 按 id 取笔记。
func (s *Store) GetPersonalNote(ctx context.Context, id string) (*model.PersonalNote, error) {
	row, err := s.Pool.Query(ctx, `SELECT `+noteCols+` FROM personal_notes WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer row.Close()
	n, err := pgx.CollectOneRow(row, pgx.RowToStructByName[model.PersonalNote])
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// GetPersonalNoteBySlug 按 (type, slug) 取笔记(事件「我的理解」:type='note', slug='event-<id>')。
// 未找到返回 nil,nil。
func (s *Store) GetPersonalNoteBySlug(ctx context.Context, noteType, slug string) (*model.PersonalNote, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+noteCols+` FROM personal_notes WHERE type=$1 AND slug=$2`, noteType, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	n, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.PersonalNote])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// ListPersonalNotes 按 type 过滤列表(空=全部),status 非 archived 优先、按 updated_at 倒序。
func (s *Store) ListPersonalNotes(ctx context.Context, noteType string) ([]model.PersonalNote, error) {
	q := `SELECT ` + noteCols + ` FROM personal_notes`
	args := []any{}
	if noteType != "" {
		q += ` WHERE type=$1`
		args = append(args, noteType)
	}
	q += ` ORDER BY (status='archived'), updated_at DESC`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.PersonalNote])
}

// ListPersonalNotesBetween 区间内更新的笔记(周报聚合用),按 updated_at 倒序。
func (s *Store) ListPersonalNotesBetween(ctx context.Context, start, end time.Time) ([]model.PersonalNote, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+noteCols+` FROM personal_notes
		WHERE updated_at >= $1 AND updated_at < $2
		ORDER BY updated_at DESC`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.PersonalNote])
}

// ArchivePersonalNote 软删(保历史):status='archived'。
func (s *Store) ArchivePersonalNote(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE personal_notes SET status='archived', updated_at=now() WHERE id=$1`, id)
	return err
}

// NoteRef 笔记关联(引用到的事件/实体)。
type NoteRef struct {
	ToType string
	ToID   string
}

// ListNoteRefs 笔记 references 关系 + 目标标题/名称(渲染用)。
func (s *Store) ListNoteRefs(ctx context.Context, noteID string) ([]NoteRefDetail, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT r.to_type AS to_type, r.to_id AS to_id,
		       COALESCE(e.title, ent.name, '') AS target
		FROM relationships r
		LEFT JOIN events e ON r.to_type='event' AND e.id = r.to_id
		LEFT JOIN entities ent ON r.to_type='entity' AND ent.id = r.to_id
		WHERE r.from_type='personal_note' AND r.from_id=$1 AND r.rel_type='references'
		ORDER BY r.to_type, target`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[NoteRefDetail])
}

// NoteRefDetail 引用目标视图行。
type NoteRefDetail struct {
	ToType string `db:"to_type"`
	ToID   string `db:"to_id"`
	Target string `db:"target"`
}

// ReplaceNoteRefs 替换笔记的事件/实体引用(编辑时整组重建,幂等)。
func (s *Store) ReplaceNoteRefs(ctx context.Context, noteID string, refs []NoteRef) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`DELETE FROM relationships WHERE from_type='personal_note' AND from_id=$1 AND rel_type='references'`,
		noteID); err != nil {
		return err
	}
	for _, ref := range refs {
		if ref.ToID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO relationships (from_type, from_id, to_type, to_id, rel_type, source)
			VALUES ('personal_note', $1, $2, $3, 'references', 'web-editor')
			ON CONFLICT (from_type, from_id, to_type, to_id, rel_type) DO NOTHING`,
			noteID, ref.ToType, ref.ToID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
