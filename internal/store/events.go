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

// ⚠️ 逗号后**不得留空格**:events.go 的限定列名靠 strings.ReplaceAll(eventCols, ",", ",e.") 派生。
// human_verdict 为**人工标记**(issue #83 P-2):只读不写,引擎永不触碰。
const eventCols = `id,raw_document_id,title,event_type,summary,facts,affected,occurred_at,` +
	`confidence,status,human_verdict,pipeline_version,source_id,cluster_id,published_at,created_at,updated_at,valid_from,valid_to`

func (s *Store) CreateEvent(ctx context.Context, ev *model.Event) (string, error) {
	facts := emptyToArrayIfScalar(ev.Facts)
	affected := emptyToArrayIfScalar(ev.Affected)
	var id string
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO events(raw_document_id,title,event_type,summary,facts,affected,occurred_at,confidence,status,pipeline_version,source_id)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		ev.RawDocumentID, ev.Title, ev.EventType, ev.Summary, facts, affected,
		ev.OccurredAt, ev.Confidence, defaultStr(ev.Status, "extracted"), ev.PipelineVersion, ev.SourceID).
		Scan(&id)
	return id, err
}

// emptyToArrayIfScalar 把「非 JSON 数组」的值归一为 `[]`。
//
// ⚠️ 不能只判 `len(b) == 0`:`json.Marshal(nil)` 产出字面量 `null`(长度 4),
// 只看长度会放行 —— 这正是 issue #71 生产事故(entity-build 每轮报
// SQLSTATE 22023)的成因。改为按 **JSON 语义值**判定:空 / `null` → `[]`。
// 非数组的其它值(理论上不该出现)一并归一,方向安全:宁可存空数组,不可存标量。
func emptyToArrayIfScalar(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`[]`)
	}
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return json.RawMessage(`[]`)
	}
	if _, ok := probe.([]any); !ok {
		return json.RawMessage(`[]`)
	}
	return raw
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

// eventsListQuery 事件流只读投影查询。includeMerged 决定是否放行 `status='merged'`
// (被聚类并入代表的重复报道)。
//
// 默认 **排除 merged**,与发布/看板口径一致:merged 是「已被同一事件的其它报道吸收」的
// 冗余,不该在「高置信事件 Top N」「个案研究」这类**未按知识态过滤**的消费方里重复出现。
// 只有前端的**事件列表**需要它(下方 ListEventsForAPI 传 true),因为那是唯一按
// 「知识态」分区的消费方(issue #80:否则「已被合并」是恒空死选项)。
func eventsListQuery(includeMerged bool, sort string) string {
	statuses := "'extracted','verified','published'"
	if includeMerged {
		statuses += ",'merged'"
	}
	return `SELECT e.id,e.title,e.event_type,e.summary,e.facts,e.affected,e.occurred_at,e.created_at,
	        e.confidence,e.status, s.name AS source_name, rd.url AS source_url, e.cluster_id
	 FROM events e
	 JOIN sources s ON s.id=e.source_id
	 LEFT JOIN raw_documents rd ON rd.id=e.raw_document_id
	 WHERE e.status IN (` + statuses + `)
	 ` + eventOrderBy(sort)
}

// ListEventsForAPI 事件流(只读投影)—— **含**被聚类并入的 merged。
// sort 见 EventSort*(空 = 时间倒序)。
// cluster_id 供前端批量取「簇内各源来源」用(issue #48 T2);非簇成员为 NULL。
//
// ⚠️ **此处必须放行 merged**(issue #80):`GET /api/v1/events` 是**唯一**按知识态分区的
// 消费方(前端抽取态筛选),排除它会让「已被合并」成为恒空死选项。merged 行仍是知识库里
// 的真实事件,前端通过 status 如实区分,不冒充代表卡。
//
// ⚠️ **看板不能用它**:`handleAPIDashboard` 曾复用它取「高置信事件 Top 6」,那里**没有**
// 知识态筛选,merged 会作为重复项混入。故该处改用 `ListTopEventsForDashboard`。
func (s *Store) ListEventsForAPI(ctx context.Context, sort string) ([]EventForAPI, error) {
	rows, err := s.Pool.Query(ctx, eventsListQuery(true, sort))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[EventForAPI])
}

// ListTopEventsForDashboard 看板「高置信事件」候选 —— **排除 merged**。
// 与事件列表(含 merged)分开,理由见 eventsListQuery 注释。
func (s *Store) ListTopEventsForDashboard(ctx context.Context) ([]EventForAPI, error) {
	rows, err := s.Pool.Query(ctx, eventsListQuery(false, EventSortConfidence))
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
// limit<=0 = 不限(取全部),供 cmd/cluster 默认用 —— 见 UnclusteredEventsTruncated。
//
// ⚠️ issue #75:「已扫描但无对端」的事件(cluster_scanned_at IS NOT NULL)不再进池,
// 否则池永不收敛、每轮重扫全部存量(生产实测 47% 滞留)。这是收敛的关键谓词。
//
// ⚠️ issue #53:limit>0 时是**硬截断**(ORDER BY created_at 取最旧 N 条)。调用方必须
// 用 UnclusteredEventsTruncated 显式检查是否被截断并记录,否则会静默漏聚类。
func (s *Store) ListUnclusteredEvents(ctx context.Context, limit int) ([]model.Event, error) {
	q := `SELECT ` + eventCols + ` FROM events
		 WHERE cluster_id IS NULL AND cluster_scanned_at IS NULL
		   AND status IN ('extracted','verified','published')
		 ORDER BY created_at`
	var rows pgx.Rows
	var err error
	if limit > 0 {
		rows, err = s.Pool.Query(ctx, q+` LIMIT $1`, limit)
	} else {
		rows, err = s.Pool.Query(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
}

// UnclusteredEventsTruncated 报告未聚类事件是否多于 limit(issue #53)。
// 调用方在 limit>0 时用它把「本次被 limit 截掉了多少」显式记录进 task_runs.meta,
// 不再让截断静默发生。limit<=0 = 不限,恒 false。
//
// ⚠️ 这里的谓词**必须与 ListUnclusteredEvents 逐字一致**(issue #75 加 cluster_scanned_at
// 时两处成对改)—— 两边漂移会让截断记账失真(报「没截断」而实际截了,或反之)。
func (s *Store) UnclusteredEventsTruncated(ctx context.Context, limit int) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM (
		   SELECT 1 FROM events
		   WHERE cluster_id IS NULL AND cluster_scanned_at IS NULL
		     AND status IN ('extracted','verified','published')
		   LIMIT $1
		 ) t`, limit+1).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > limit, nil
}

// MarkEventsScanned 给「本轮已比对过、最终不属于任何分量」的事件盖扫描水位(issue #75)。
//
// ⚠️ `AND cluster_id IS NULL` 是并发护栏:若事件在本轮进行中已被并发合并进某簇,
// 不能再给它盖水位戳(那会让它看起来像「扫过无对端」,而实际是已归簇)。
// ⚠️ 调用方只能传**本轮真正比对过**的事件 id。`-limit > 0` 截断时,池外未比对的事件
// 不得标记 —— 否则它们本轮没被看过却被判「无对端」,永久漏召回。
func (s *Store) MarkEventsScanned(ctx context.Context, ids []string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	ct, err := s.Pool.Exec(ctx,
		`UPDATE events SET cluster_scanned_at=now()
		 WHERE id = ANY($1) AND cluster_id IS NULL`, ids)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
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
	return s.ListActiveClusterRepresentativesSince(ctx, time.Time{})
}

// ListActiveClusterRepresentativesSince 同 ListActiveClusterRepresentatives,但只返回
// 「窗口内仍活跃」的簇:窗口口径 = 簇内成员的 **MAX(created_at)**(since 为零值 = 不限)。
//
// ⚠️ 为何是 MAX(member.created_at) 而非代表(最早成员)的 created_at(issue #75):
// 代表按定义是簇内**最早**成员(`:285` 的 ORDER BY e.created_at),按它开窗会把
// 「刚并入新成员的老簇」整个排除掉 —— 而那恰恰是最该进重审视池的(新成员可能与别的簇重复)。
//
// ⚠️ 为何不用 event_clusters.updated_at:它今日已不一致 —— MergeClusters 会 bump
// (`event_clusters.go:54`),但 CreateEventCluster(`:16`)、ApplyClusters 的成员并入
// (`cluster.go:380`)、reexamine.go 的并入(`:105`)都**不 bump**。
//
// 本查询让重审视池从「全部活跃簇(生产 3307 个,且每日递增)」收窄到
// 「窗口内活跃的簇」,与「未聚类窗口」共同把池钉成常数(见 reexamine.go)。
func (s *Store) ListActiveClusterRepresentativesSince(ctx context.Context, since time.Time) ([]ClusterRepresentative, error) {
	// eventCols 含 cluster_id 且与 event_clusters 同名列(id/title/status/created_at/updated_at)冲突,
	// JOIN 场景必须逐列加 e. 前缀。
	qualified := "e." + strings.ReplaceAll(eventCols, ",", ",e.")
	q := `SELECT DISTINCT ON (e.cluster_id) ` + qualified + `
		 FROM events e
		 JOIN event_clusters c ON c.id=e.cluster_id
		 WHERE c.status='active'
		   AND e.status IN ('extracted','verified','published')`
	args := []any{}
	if !since.IsZero() {
		q += `
		   AND e.cluster_id IN (
		     SELECT cluster_id FROM events
		     WHERE cluster_id IS NOT NULL
		     GROUP BY cluster_id
		     HAVING max(created_at) >= $1
		   )`
		args = append(args, since)
	}
	q += ` ORDER BY e.cluster_id, e.created_at, e.confidence DESC`
	rows, err := s.Pool.Query(ctx, q, args...)
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

// ScannedEventsSince 返回窗口内「已扫描但无对端」的事件(cluster_scanned_at IS NOT NULL)。
//
// 🔴 这是 issue #75 标记列方案的**成对另一半,不可省**:这些事件 cluster_id 仍为 NULL、
// 不是活跃簇代表,`ListActiveClusterRepresentatives*` 看不见它们。若重审视池只包含
// 「活跃簇代表 ∪ 未聚类事件(cluster_scanned_at IS NULL)」,它们就**从两处视野同时消失
// ⇒ 永久漏召回**。显式并入本查询的结果,标记列方案才与「建单例簇」召回等价。
//
// since 为零值 = 不限(窗口默认值由 cmd/cluster 的 -window-days 控制)。
func (s *Store) ScannedEventsSince(ctx context.Context, since time.Time) ([]model.Event, error) {
	q := `SELECT ` + eventCols + ` FROM events
		 WHERE cluster_id IS NULL AND cluster_scanned_at IS NOT NULL
		   AND status IN ('extracted','verified','published')`
	args := []any{}
	if !since.IsZero() {
		q += ` AND cluster_scanned_at >= $1`
		args = append(args, since)
	}
	q += ` ORDER BY created_at`
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
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
	// Content 该机构**代表正文**(去重后优先保留带 url 的那条),供读路径剥转载
	// (issue #83 P-1:转载 ≠ 独立源,靠簇内正文指纹分组判定)。不直接下发前端,
	// 仅在 `toEventItem` 内算独立来源数与「(转载)」标记。
	Content *string `db:"content"`
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
		       NULLIF(rd.extra->>'source', '') AS origin, rd.content
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

// ClusterMember 簇内一个成员的来源归属 + 事实句(issue #49 T3 冲突检测用)。
// Affected(issue #83 P-4):成员的影响实体**并集**来源 —— 簇合并视图要显示「整个簇涉及什么」,
// 而 canonical 单个成员的 affected 只是子集。⚠️ 行投影必须同步含 e.affected(RowToStructByName 严格匹配)。
type ClusterMember struct {
	EventID  string          `db:"event_id"`
	Source   string          `db:"source"`
	Facts    json.RawMessage `db:"facts"`
	Affected json.RawMessage `db:"affected"`
}

// ListClusterMembersWithFacts 取若干簇的全部成员的**机构名 + facts**(含 merged 成员)。
//
// 与 ListClusterSources 的分工:后者给前端「哪些机构报道了」(去重、带 url/origin);
// 本方法给冲突检测**逐成员的事实句** —— 冲突是「两家对同一个量给出不同的数」,
// 必须保留**每个成员各自**的 facts,不能按机构去重(同机构多次报道也可能自相矛盾)。
//
// 含 merged:被并入的成员正是另一家机构的版本;只取 canonical 等于放弃了比对对象 ——
// 而「不静默择一」正是本任务的红线(issue #49)。
func (s *Store) ListClusterMembersWithFacts(ctx context.Context, clusterIDs []string) (map[string][]ClusterMember, error) {
	if len(clusterIDs) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id AS event_id, e.cluster_id, s.name AS source, e.facts, e.affected
		FROM events e
		JOIN sources s ON s.id = e.source_id
		WHERE e.cluster_id = ANY($1)
		ORDER BY e.cluster_id, e.created_at`, clusterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		ClusterMember
		ClusterID string `db:"cluster_id"`
	}
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, err
	}
	out := make(map[string][]ClusterMember, len(clusterIDs))
	for _, r := range rs {
		out[r.ClusterID] = append(out[r.ClusterID], r.ClusterMember)
	}
	return out, nil
}

// ClusterRawSource 簇的一个 raw 层来源(issue #83 P-4 / P8)。
//
// 与 `ClusterSource` 的区别(为何 P-4 新增而非改老的):
//   - `ClusterSource` 走**事件层**:每机构一条(去重)、取该机构代表 url/正文。事件层是 raw 层的**子集**
//     —— 某家报了但没被抽出事件时,它根本不在这里。
//   - `ClusterRawSource` 走 **raw 层全集**:簇内成员的 raw 行 → 其转载组代表(`COALESCE(canonical_id,id)`)
//     → 该代表名下**全部** raw 行。于是「同一篇稿被 5 家转发」这里能列出 5 家各自 url,
//     即便其中某家没抽成事件。这是 P8 红线「链接取 raw 层全集」的落地点。
//
// 每行 = 一条 raw 文档(同机构可多行,不合并);「该机构计一票」由调用方按 Source 去重满足。
type ClusterRawSource struct {
	Source string  `db:"source"` // 机构名
	URL    *string `db:"url"`    // 该机构这条 raw 的原文链接;NULL = 该源无外链(绝不造链接)
	// Origin 上游自带的一级源名(金十/同花顺 extra.source,如「新华社」):「这条转述的是谁」。
	// ⚠️ 与 URL 的配对**不保证同机构** —— 金十的 url 就是一级源的链接(见 collector/jin10.go),
	// 故前端须把「渠道名」与「一级源名」分区展示,不得把一级源链接挂在渠道名下。
	Origin *string `db:"origin"`
	// IsRep 本行是否为其转载组的**代表行**(rd.id == COALESCE(canonical_id,id))。
	// 代表行不计入「转载」;非代表行 = 转载(issue #83 P-1 判据的 raw 层快照)。
	// ⚠️ 未回填 canonical_id 时**每行都是代表**(IsRep 恒 true)⇒ 无转载标记 —— 合法退化。
	IsRep bool `db:"is_rep"`
	// Canonical 该行的转载组代表 id(COALESCE(canonical_id,id)):同组行共享此值,前端可归并显示。
	Canonical string `db:"canonical"`
}

// ListClusterRawSources 取若干簇的 **raw 层全集**来源(issue #83 P-4 / P8)。
//
// 取法(候选集 = 簇内事件指向的 raw 行的**转载组代表**):
//
//	member_raw:      events(该簇) JOIN raw_documents → 各成员 raw 行
//	reps:            member_raw 的 COALESCE(canonical_id, id) —— 去重后的代表集合
//	最终行:          与 reps 同组的**全部** raw_documents(不限成员、不限事件)
//
// 🔴 为何用 reps 而非直接 member_raw:同一篇稿被 5 家转发,只有 2 家抽出了事件;
// 直接取 member_raw 只见 2 家,取代表同组全集才见 5 家 —— 正是 P8 要修的那个「展示层回退到一家」。
//
// ⚠️ 依赖 `cmd/cluster-raw-link` 的回填(canonical_id);未回填时 COALESCE 退化为「自己就是代表」,
// 结果 = 事件层可见的那几家的 raw 行(**合法退化**,不报错、不为 0)。
// ⚠️ 只取 `origin_kind='pipeline'`:实时层(P-5)不进正式展示来源(与 P-2 门控同旨)。
func (s *Store) ListClusterRawSources(ctx context.Context, clusterIDs []string) (map[string][]ClusterRawSource, error) {
	if len(clusterIDs) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
		WITH member_raw AS (
			SELECT e.cluster_id, COALESCE(rd.canonical_id, rd.id) AS rep_id
			FROM events e
			JOIN raw_documents rd ON rd.id = e.raw_document_id
			WHERE e.cluster_id = ANY($1) AND rd.origin_kind = 'pipeline'
			GROUP BY e.cluster_id, COALESCE(rd.canonical_id, rd.id)
		)
		SELECT
			mr.cluster_id,
			s.name AS source,
			rd.url,
			NULLIF(rd.extra->>'source', '') AS origin,
			rd.id = COALESCE(rd.canonical_id, rd.id) AS is_rep,
			COALESCE(rd.canonical_id, rd.id)::text AS canonical
		FROM member_raw mr
		JOIN raw_documents rd ON COALESCE(rd.canonical_id, rd.id) = mr.rep_id
		JOIN sources s ON s.id = rd.source_id
		WHERE rd.origin_kind = 'pipeline'
		ORDER BY mr.cluster_id, s.name, (rd.url IS NULL), rd.created_at, rd.id`, clusterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		ClusterRawSource
		ClusterID string `db:"cluster_id"`
	}
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, err
	}
	out := make(map[string][]ClusterRawSource, len(clusterIDs))
	for _, r := range rs {
		out[r.ClusterID] = append(out[r.ClusterID], r.ClusterRawSource)
	}
	return out, nil
}

// ListClusterTitles 取若干簇的标题(id → title)。簇标题是 P8 的「展示单元标题」(issue P8 第 1 条):
// 抽屉里应显示簇级标题,而非某个成员事件的标题。无此字段的簇不返回。
func (s *Store) ListClusterTitles(ctx context.Context, clusterIDs []string) (map[string]string, error) {
	if len(clusterIDs) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT id, title FROM event_clusters WHERE id = ANY($1)`, clusterIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]string, len(clusterIDs))
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out[id] = title
	}
	return out, rows.Err()
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

// DocMeta 事件对应 raw 文档的建簇期元数据:是否有直接链接 + 正文(issue #83 P-3)。
//
// ⚠️ 与 `ClusterSource` 的分工:后者是**簇后**读路径(按 cluster_id 取,按机构去重),
// 本类型是**建簇前**用 —— `ApplyClusters` 收到的 `[]model.Event` 只有 `RawDocumentID` 外键,
// 没有 url/content,而 P6 代表选取规则(有链接 > 非转载 > 最早)在建簇那一刻就要这两样。
type DocMeta struct {
	URL     *string `db:"url"`
	Content *string `db:"content"`
}

// ListEventDocMeta 按事件 id 集合取各事件 raw 文档的 (url, content),供 P6 代表选取用。
//
// 只对**多成员分量**调用(单成员分量代表就是自己,无需判定),控制正文取数面。
// 未关联 raw 文档(LEFT JOIN 落空)的事件不在返回 map 中 —— 调用方按零值处理(无链接、无正文)。
func (s *Store) ListEventDocMeta(ctx context.Context, eventIDs []string) (map[string]DocMeta, error) {
	if len(eventIDs) == 0 {
		return map[string]DocMeta{}, nil
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id AS event_id, rd.url, rd.content
		FROM events e
		LEFT JOIN raw_documents rd ON rd.id = e.raw_document_id
		WHERE e.id = ANY($1)`, eventIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		EventID string `db:"event_id"`
		DocMeta
	}
	rs, err := pgx.CollectRows(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, err
	}
	out := make(map[string]DocMeta, len(rs))
	for _, r := range rs {
		out[r.EventID] = r.DocMeta
	}
	return out, nil
}

// ListEventsInWindow 取 [start, end) 内的非 merged 事件,按**原始到达时刻**倒序。
//
// 窗口锚 = **原始到达时刻** `COALESCE(rd.published_at, rd.retrieved_at, e.created_at)`
// (issue #83 分期 P-5 修订,P-3 §1.2 已预先授权重评)。早/晚档的理由是**阅读节奏**
// (早上看隔夜+盘前、晚上看全天),锚「我们何时**拿到**这条」才对得上读者的时间轴。
//
// 🔴 为什么不用 `e.created_at`(入库/抽取时刻):整条管线在收盘后一次跑,`created_at` 全落
// **晚**窗,早档(前一日 18:30 → 当日 09:15)恒空 —— 早/晚切分名存实亡。改锚原始到达后,
// 隔夜消息(前一日 18:30 → 当日 09:15 **发布/采集**)才真正落早榜。
// 兜底 `e.created_at` 仅用于 `raw_document_id IS NULL` 的极少数行(dev 实测 1 行)。
// ⚠️ `occurred_at` 仍**不**作窗口锚:它是事件**声称**发生的时间,常缺失、跨源口径不一。
//
// **不 LIMIT**(issue P1「窗口内全部合并事件,不截断」)—— 日量约百条量级,可控。
// 返回 `EventForAPI`(带来源名/链接/cluster_id),与事件流同一投影,供 `toEventItem` 复用。
func (s *Store) ListEventsInWindow(ctx context.Context, start, end time.Time) ([]EventForAPI, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT e.id,e.title,e.event_type,e.summary,e.facts,e.affected,e.occurred_at,e.created_at,
		       e.confidence,e.status, s.name AS source_name, rd.url AS source_url, e.cluster_id
		FROM events e
		JOIN sources s ON s.id=e.source_id
		LEFT JOIN raw_documents rd ON rd.id=e.raw_document_id
		WHERE e.status <> 'merged'
		  AND COALESCE(rd.published_at, rd.retrieved_at, e.created_at) >= $1
		  AND COALESCE(rd.published_at, rd.retrieved_at, e.created_at) <  $2
		ORDER BY COALESCE(rd.published_at, rd.retrieved_at, e.created_at) DESC`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByName[EventForAPI])
}
