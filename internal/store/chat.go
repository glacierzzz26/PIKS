package store

// chat 对话会话/消息/附件存取(迭代 5-3,设计 §5.2;迁移 0008)。

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

// ChatRefs 答案引用(渲染成可点 chip 跳转事件卡/实体卡)。
type ChatRefs struct {
	Events   []ChatRef `json:"events"`
	Entities []ChatRef `json:"entities"`
}

type ChatRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// GetOrCreateChatSession 单人系统:恒用一个默认会话(首个消息时懒创建)。
func (s *Store) GetOrCreateChatSession(ctx context.Context) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`SELECT id FROM chat_sessions ORDER BY created_at LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return "", err
	}
	err = s.Pool.QueryRow(ctx,
		`INSERT INTO chat_sessions(title) VALUES('默认对话') RETURNING id`).Scan(&id)
	return id, err
}

// LatestChatSessionID 最近一个会话 id(只读,无会话返回 "");React 对话页投影用。
func (s *Store) LatestChatSessionID(ctx context.Context) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`SELECT id FROM chat_sessions ORDER BY created_at LIMIT 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ListChatMessages 按时间序返回会话全部消息(含 user 与 assistant)。
func (s *Store) ListChatMessages(ctx context.Context, sessionID string) ([]model.ChatMessage, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id,session_id,role,content,refs,attachments,created_at
		 FROM chat_messages WHERE session_id=$1 ORDER BY created_at, id`, sessionID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.ChatMessage])
}

// InsertChatMessage 存一条消息(refs/attachments 为 JSONB)。
func (s *Store) InsertChatMessage(ctx context.Context, m *model.ChatMessage) error {
	refs := m.Refs
	if len(refs) == 0 {
		refs = json.RawMessage(`{}`)
	}
	atts := m.Attachments
	if len(atts) == 0 {
		atts = json.RawMessage(`[]`)
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO chat_messages(session_id,role,content,refs,attachments)
		 VALUES($1,$2,$3,$4,$5)
		 ON CONFLICT DO NOTHING`,
		m.SessionID, m.Role, m.Content, refs, atts)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx,
		`UPDATE chat_sessions SET updated_at=now() WHERE id=$1`, m.SessionID)
	return err
}

// CreateAttachment 记录附件元数据,返回 id(文件本体由调用方落盘)。
func (s *Store) CreateAttachment(ctx context.Context, a *model.Attachment) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO attachments(filename,mime,size,path) VALUES($1,$2,$3,$4) RETURNING id`,
		a.Filename, a.MIME, a.Size, a.Path).Scan(&id)
	return id, err
}

// GetAttachment 按 id 取附件元数据(页面/API 回显截图用)。
func (s *Store) GetAttachment(ctx context.Context, id string) (*model.Attachment, error) {
	var a model.Attachment
	err := s.Pool.QueryRow(ctx,
		`SELECT id,filename,mime,size,path,created_at FROM attachments WHERE id=$1`, id).
		Scan(&a.ID, &a.Filename, &a.MIME, &a.Size, &a.Path, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ClearChatSession 清空会话历史(页面「清空」按钮;保留会话行)。
func (s *Store) ClearChatSession(ctx context.Context, sessionID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM chat_messages WHERE session_id=$1`, sessionID)
	return err
}
