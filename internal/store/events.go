package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/model"
)

const eventCols = `id,raw_document_id,title,event_type,summary,facts,affected,occurred_at,` +
	`confidence,status,pipeline_version,source_id,cluster_id,published_at,created_at,updated_at,valid_from,valid_to`

func (s *Store) CreateEvent(ctx context.Context, ev *model.Event) (string, error) {
	facts := ev.Facts
	if len(facts) == 0 {
		facts = json.RawMessage(`[]`)
	}
	affected := ev.Affected
	if len(affected) == 0 {
		affected = json.RawMessage(`[]`)
	}
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO events(raw_document_id,title,event_type,summary,facts,affected,occurred_at,confidence,status,pipeline_version,source_id)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		ev.RawDocumentID, ev.Title, ev.EventType, ev.Summary, facts, affected,
		ev.OccurredAt, ev.Confidence, defaultStr(ev.Status, "extracted"), ev.PipelineVersion, ev.SourceID).
		Scan(&id)
	return id, err
}

// ListEventsForPublish 返回已抽取/已验证、尚未发布的事件。
func (s *Store) ListEventsForPublish(ctx context.Context) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+eventCols+` FROM events
		 WHERE status IN ('extracted','verified')
		 ORDER BY occurred_at NULLS LAST, created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}

// EventForPublish 发布器视角的事件视图:事件字段 + 来源名(join sources)。
type EventForPublish struct {
	ID              string          `db:"id"`
	Title           string          `db:"title"`
	EventType       string          `db:"event_type"`
	Summary         *string         `db:"summary"`
	Facts           json.RawMessage `db:"facts"`
	Affected        json.RawMessage `db:"affected"`
	OccurredAt      *time.Time      `db:"occurred_at"`
	CreatedAt       time.Time       `db:"created_at"`
	Confidence      float64         `db:"confidence"`
	PipelineVersion *string         `db:"pipeline_version"`
	Status          string          `db:"status"`
	SourceName      string          `db:"source_name"`
	ClusterID       *string         `db:"cluster_id"`
	PublishedAt     *time.Time      `db:"published_at"`
	UpdatedAt       time.Time       `db:"updated_at"`
}

// ListEventsForPublishWithSource 发布候选 = 全部有效事件(extracted/verified/published)。
// 迭代 3 起全量返回:实体层更新后(新实体建成),已发布事件卡也要重渲染升级 wikilink。
// 幂等靠调用方 md5 内容比对(未变跳过写盘 → git 零提交),而非依赖 updated_at 增量。
func (s *Store) ListEventsForPublishWithSource(ctx context.Context) ([]EventForPublish, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT e.id,e.title,e.event_type,e.summary,e.facts,e.affected,e.occurred_at,e.created_at,
		        e.confidence,e.pipeline_version,e.status,e.cluster_id,e.published_at,e.updated_at,
		        s.name AS source_name
		 FROM events e JOIN sources s ON s.id=e.source_id
		 WHERE e.status IN ('extracted','verified','published')
		 ORDER BY e.occurred_at NULLS LAST, e.created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[EventForPublish])
}

// MarkEventPublished 标记事件已发布:只设 published_at,不改 status。
// status 恒表示知识状态(extracted/verified/merged),发布生命周期由 published_at 承载(设计 §3.4)。
// 好处:卡片 front matter 稳定,已发布事件即使 updated_at 被触碰,内容未变时渲染逐字节相同 → hash 跳过 → git 零提交。
func (s *Store) MarkEventPublished(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE events SET published_at=now(), updated_at=now() WHERE id=$1`, id)
	return err
}

// ListUnclusteredEvents 返回未聚类候选(cluster_id IS NULL),供 cluster 命令使用。
// 含已发布但从未聚类的事件:新事件可能与该已发布事件是同一真实事件,需一并参与去重。
func (s *Store) ListUnclusteredEvents(ctx context.Context, limit int) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+eventCols+` FROM events
		 WHERE cluster_id IS NULL AND status IN ('extracted','verified','published')
		 ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}

// ListEventsByCluster 返回某簇全部成员(按 created_at 升序)。
func (s *Store) ListEventsByCluster(ctx context.Context, clusterID string) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+eventCols+` FROM events WHERE cluster_id=$1 ORDER BY created_at`, clusterID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}

// ClusterRepresentative 活跃簇的代表事件:簇内对外展示的 canonical 卡(cluster_id + 事件本体)。
type ClusterRepresentative struct {
	ClusterID string
	Event     model.Event
}

// ListActiveClusterRepresentatives 每个活跃簇返回一个代表事件,供聚类重审视 Pass
// (跨簇重复检测,design cluster-quality)使用。
// 代表 = 簇内 status IN ('extracted','verified','published') 的最早创建成员(同则更高置信),
// 即 ApplyClusters 选取的 canonical;event_clusters.status='merged' 的簇不再返回。
func (s *Store) ListActiveClusterRepresentatives(ctx context.Context) ([]ClusterRepresentative, error) {
	// eventCols 含 cluster_id 且与 event_clusters 同名列(id/title/status/created_at/updated_at)冲突,
	// JOIN 场景必须逐列加 e. 前缀。
	qualified := "e." + strings.ReplaceAll(eventCols, ",", ",e.")
	rows, err := s.Pool.Query(ctx,
		`SELECT DISTINCT ON (e.cluster_id) `+qualified+`
		 FROM events e
		 JOIN event_clusters c ON c.id=e.cluster_id
		 WHERE c.status='active'
		   AND e.status IN ('extracted','verified','published')
		 ORDER BY e.cluster_id, e.created_at, e.confidence DESC`)
	if err != nil {
		return nil, err
	}
	evs, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
	if err != nil {
		return nil, err
	}
	out := make([]ClusterRepresentative, 0, len(evs))
	for _, ev := range evs {
		if ev.ClusterID == nil {
			continue
		}
		out = append(out, ClusterRepresentative{ClusterID: *ev.ClusterID, Event: ev})
	}
	return out, nil
}

// SetEventCluster 把事件并入簇:设 cluster_id、可选改 status,并 bump updated_at(触发增量发布)。
func (s *Store) SetEventCluster(ctx context.Context, eventID, clusterID, status string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE events SET cluster_id=$2, status=$3, updated_at=now() WHERE id=$1`,
		eventID, clusterID, status)
	return err
}

// SetEventClusterNoTouch 仅设 cluster_id(不改 status/updated_at)。
// 用于已发布 canonical:聚类不应触发无谓的卡片重写与 git 噪音。
func (s *Store) SetEventClusterNoTouch(ctx context.Context, eventID, clusterID string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE events SET cluster_id=$2 WHERE id=$1`, eventID, clusterID)
	return err
}

// ListMergedPublished 已发布但被并入簇(需要从 vault 删除旧卡片)的事件。
func (s *Store) ListMergedPublished(ctx context.Context) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+eventCols+` FROM events WHERE status='merged' AND published_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}

func (s *Store) GetEventByID(ctx context.Context, id string) (model.Event, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+eventCols+` FROM events WHERE id=$1`, id)
	if err != nil {
		return model.Event{}, err
	}
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Event])
}

// ListEventsByDate 某自然日的已抽取事件(occurred_at 命中该日;occurred_at 为空时退回 created_at)。
// 供 market-state/daily-review 的 top_events(每日复盘第 11 项)与 hot_topics 派生。
func (s *Store) ListEventsByDate(ctx context.Context, day time.Time) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+eventCols+` FROM events
		WHERE status <> 'merged'
		  AND ((occurred_at >= $1 AND occurred_at < $1 + interval '1 day')
		   OR (occurred_at IS NULL AND created_at >= $1 AND created_at < $1 + interval '1 day'))
		ORDER BY occurred_at NULLS LAST, created_at DESC
		LIMIT 50`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}

// ListEventsBetween 区间事件(非 merged),按时间倒序(周报聚合用)。
func (s *Store) ListEventsBetween(ctx context.Context, start, end time.Time) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+eventCols+` FROM events
		WHERE status <> 'merged'
		  AND COALESCE(occurred_at, created_at) >= $1 AND COALESCE(occurred_at, created_at) < $2
		ORDER BY COALESCE(occurred_at, created_at) DESC
		LIMIT 200`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}

// ListEventsRecent 最近 N 条事件(非 merged),按时间倒序(笔记关联选择器用)。
func (s *Store) ListEventsRecent(ctx context.Context, limit int) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+eventCols+` FROM events
		WHERE status <> 'merged'
		ORDER BY COALESCE(occurred_at, created_at) DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}
