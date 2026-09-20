package store

import (
	"context"
	"encoding/json"
	"sort"
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

// EventForAPI 前端事件流只读投影(api_v1):事件本体 + 来源名 + raw URL。
type EventForAPI struct {
	ID         string          `db:"id"`
	Title      string          `db:"title"`
	EventType  string          `db:"event_type"`
	Summary    *string         `db:"summary"`
	Facts      json.RawMessage `db:"facts"`
	Affected   json.RawMessage `db:"affected"`
	OccurredAt *time.Time      `db:"occurred_at"`
	CreatedAt  time.Time       `db:"created_at"`
	Confidence float64         `db:"confidence"`
	Status     string          `db:"status"`
	SourceName string          `db:"source_name"`
	SourceURL  *string         `db:"source_url"`
	ClusterID  *string         `db:"cluster_id"`
}

// EventSort 事件流排序维度。默认(空/EventSortTime)= 发生时间倒序(最新在前)。
//
// 时间口径统一为 COALESCE(occurred_at, created_at):occurred_at 可空(历史数据),
// 直接 ORDER BY occurred_at DESC 会把 NULL 甩到最前/最后,与「按时间排」直觉相悖;
// 空值事件按其 created_at 参与排序,即发布时刻。
const (
	EventSortTime       = "time"
	EventSortConfidence = "confidence"
)

// eventOrderBy 把排序维度映射为 SQL ORDER BY 子句(白名单,不接受任意输入)。
func eventOrderBy(sort string) string {
	if sort == EventSortConfidence {
		// 置信度倒序;同分再按时间倒序,保证分页顺序稳定。
		return `ORDER BY e.confidence DESC, COALESCE(e.occurred_at, e.created_at) DESC`
	}
	return `ORDER BY COALESCE(e.occurred_at, e.created_at) DESC`
}

// ListEventsForAPI 全部有效事件(extracted/verified/published)+ 来源名 + raw url。
// sort 见 EventSort*(空 = 时间倒序)。
// cluster_id 供前端批量取「簇内各源来源」用(issue #48 T2);非簇成员为 NULL。
func (s *Store) ListEventsForAPI(ctx context.Context, sort string) ([]EventForAPI, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT e.id,e.title,e.event_type,e.summary,e.facts,e.affected,e.occurred_at,e.created_at,
		        e.confidence,e.status, s.name AS source_name, rd.url AS source_url, e.cluster_id
		 FROM events e
		 JOIN sources s ON s.id=e.source_id
		 LEFT JOIN raw_documents rd ON rd.id=e.raw_document_id
		 WHERE e.status IN ('extracted','verified','published')
		 `+eventOrderBy(sort))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[EventForAPI])
}

// ListEventsByIDs 按 id 批量取事件(完整投影行),个股中心「相关事件」用。
// 顺序由调用方按需重排(本查询按 occurred_at/created_at 升序)。
func (s *Store) ListEventsByIDs(ctx context.Context, ids []string) ([]EventForAPI, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT e.id,e.title,e.event_type,e.summary,e.facts,e.affected,e.occurred_at,e.created_at,
		        e.confidence,e.status, s.name AS source_name, rd.url AS source_url, e.cluster_id
		 FROM events e
		 JOIN sources s ON s.id=e.source_id
		 LEFT JOIN raw_documents rd ON rd.id=e.raw_document_id
		 WHERE e.id = ANY($1) AND e.status IN ('extracted','verified','published')
		 ORDER BY e.occurred_at NULLS LAST, e.created_at`, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[EventForAPI])
}

// WatchEventRef 自选富化用:affects 到某实体的事件(带实体归属与发生日)。
type WatchEventRef struct {
	EntityID   string    `db:"entity_id"`
	EventID    string    `db:"event_id"`
	Title      string    `db:"title"`
	OccurredAt time.Time `db:"occurred_at"`
}

// ListEventsByEntityIDs 批量取 affects 到给定实体的事件(P6-3 自选富化)。
// 一次查询喂满首页「每只票最近的消息」,避免 N 次往返;零 schema(复用 relationships)。
func (s *Store) ListEventsByEntityIDs(ctx context.Context, entityIDs []string) ([]WatchEventRef, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT r.to_id AS entity_id, e.id AS event_id, e.title, e.occurred_at
		FROM relationships r
		JOIN events e ON e.id = r.from_id
		WHERE r.from_type='event' AND r.to_type='entity' AND r.rel_type='affects'
		  AND r.to_id = ANY($1)
		  AND e.status IN ('extracted','verified','published')
		  AND e.occurred_at IS NOT NULL
		ORDER BY e.occurred_at DESC`, entityIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[WatchEventRef])
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

// ClusterSource 簇内一个来源:机构名 + 原文链接 + 上游一级源标注(issue #48 T2)。
//
// 一个簇 = 同一真实事件的多条报道,每条报道来自一个机构(events.source_id → sources.name,
// T1 已改机构名);原文链接取该事件的 raw_document.url。
// Origin 为**上游自带的一级源**(金十 extra.source,实测出现「新华社」「央视新闻」等)——
// 它是「这条快讯转述的是谁」,与「我们从哪个机构采到」是两件事,如实在 UI 分区标注。
type ClusterSource struct {
	EventID string  `db:"event_id"`
	Source  string  `db:"source"`
	URL     *string `db:"url"`
	Origin  *string `db:"origin"`
}

// ListClusterSources 取若干簇的**全部成员**来源(含 status='merged' 的被并入成员)。
//
// 为何含 merged:簇内「各源来源」正是这些被合并成员贡献的 —— 若只取 canonical,一个跨 5 家
// 机构的簇只会显示 1 个来源,多源印证就白做了(issue #48 验收「簇内可见各源来源」)。
// 这也是唯一需要读 merged 事件的读路径(其余列表查询一律排除 merged,避免重复卡)。
//
// 去重:同一机构可能在簇内有多条(该机构对同一事件发了多次),按 (cluster_id, source) 去重 ——
// 展示的是「哪些机构报道了」,不是「有几条记录」;同机构仍优先保留带 url 的那条。
func (s *Store) ListClusterSources(ctx context.Context, clusterIDs []string) (map[string][]ClusterSource, error) {
	if len(clusterIDs) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT ON (e.cluster_id, s.name)
		       e.id AS event_id, e.cluster_id, s.name AS source, rd.url,
		       NULLIF(rd.extra->>'source', '') AS origin
		FROM events e
		JOIN sources s ON s.id = e.source_id
		LEFT JOIN raw_documents rd ON rd.id = e.raw_document_id
		WHERE e.cluster_id = ANY($1)
		ORDER BY e.cluster_id, s.name, (rd.url IS NULL), e.created_at`, clusterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		ClusterSource
		ClusterID string `db:"cluster_id"`
	}
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, err
	}
	// 簇内按机构名排序,输出稳定(前端渲染顺序不抖动)。
	out := make(map[string][]ClusterSource, len(clusterIDs))
	for _, r := range rs {
		out[r.ClusterID] = append(out[r.ClusterID], r.ClusterSource)
	}
	for k := range out {
		sort.Slice(out[k], func(a, b int) bool { return out[k][a].Source < out[k][b].Source })
	}
	return out, nil
}

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
