-- 事件管线 P-2(issue #83 分期 P-2):4 个**非冗余**列 + 簇详情入口回填。
--
-- 范围说明(勿照抄 issue §三 P4 表的「8 项」字面)——P4 共 8 项,本迁移只补 4 列:
--   ✅ 已落地:extra(0014,上游原始字段)。
--   ⬜ 本迁移新增:
--     · raw_documents.origin_kind       —— 实时/正式分道闸(P-5 的前置契约)
--     · event_clusters.canonical_event_id —— 簇的详情入口(已由 ApplyClusters 算出、此前丢弃)
--     · raw_documents.canonical_id      —— raw 层转载组代表(**只建列**,填充属 P-8)
--     · events.human_verdict            —— 人工标记(**只建列**,引擎永不碰)
--   🔴 刻意不做(3 项,见 docs/phase11/design/event-pipeline-p2.md §0):
--     · raw_documents.origin  —— 已可从 extra->>'source' 读(issue §六 #7),不必建列;
--     · raw_documents.is_reprint —— P-1 已**读路径派生**(internal/cluster/reprint.go)。
--       ⚠️ 更根本:转载是**簇相对**属性(需 canonicalIdx 才能定「谁转载谁」),
--       列级布尔本就不良定义;
--     · events.corroboration —— P-1 已派生 independent_count。持久化会与 reexamine 的
--       MergeClusters 漂移(簇成员数一变,印证度就得重算 → 两处真源)。
--
-- 🔴 origin_kind 的契约(本迁移只为把门装上,今日无实时行):
--   worker(ListRawPendingStatus)与 reconcile 的**全部 raw 层查询**一律只认
--   `origin_kind='pipeline'`。未来 P-5 的实时层落 'realtime' ⇒ 结构上**无法**被抽进 events
--   (worker 只取 pipeline),也无法进对账。
--   ⚠️ 去重键(迁移 0016 的两条 partial unique index)**不含 origin_kind** —— 这是 issue 的
--   有意设计(跨管线统一去重)。副作用:P-5 必须补一步「realtime → pipeline 提升」,
--   否则先落的 realtime 行会把正式行的 ON CONFLICT DO NOTHING 吃掉(该条永远抽不出事件)。
--   本迁移**不**实现提升 —— 那是 P-5 的活。
--
-- 🔴 human_verdict 的纪律:人工标记,**引擎永不写**。幂等重跑(MergeClusters 的
--   `SET status=CASE…`、SetEventCluster 的 `status='merged'`)都不得触碰本列。
--   取值域**本版不定**(待 P-3/P-4),故**故意不加 CHECK** —— 加了会挡住将来的人工取值。

-- (1) 实时/正式分道闸。NOT NULL DEFAULT 字面量 ⇒ PG11+ 仅改元数据,不重写表。
ALTER TABLE raw_documents ADD COLUMN IF NOT EXISTS origin_kind TEXT NOT NULL DEFAULT 'pipeline';

ALTER TABLE raw_documents DROP CONSTRAINT IF EXISTS raw_documents_origin_kind_check;
ALTER TABLE raw_documents ADD CONSTRAINT raw_documents_origin_kind_check
  CHECK (origin_kind IN ('pipeline','realtime'));

COMMENT ON COLUMN raw_documents.origin_kind IS
  'pipeline=正式管线采集(worker 抽取候选)/ realtime=实时层(P-5,只落 raw、不进 events)。'
  'worker 与 reconcile 的全部 raw 层查询一律加 origin_kind=''pipeline'' —— 今日无 realtime 行,'
  '过滤是**契约**而非优化。⚠️ 去重键(0016)不含本列,P-5 须补「realtime→pipeline 提升」,'
  '否则实时行会抢吃掉正式行的去重槽。';

-- (2) 簇的详情入口事件(issue P4 #8)。ApplyClusters 早已算出 canonical 却丢弃,此处持久化。
--     无 FK(与决策边 relationships 同纪律):消费方须处理悬空/缺失,不得回退猜首个事件。
ALTER TABLE event_clusters ADD COLUMN IF NOT EXISTS canonical_event_id UUID;

COMMENT ON COLUMN event_clusters.canonical_event_id IS
  '簇详情入口事件(P8 / /events/:id / based_on 决策边)。NULL = 该簇无在产成员'
  '(被 MergeClusters 吸走的簇),消费方必须处理 NULL,不得回退猜首个事件。'
  '⚠️ 代表选定后**冻结**(issue P4「代表漂移」纪律):reexamine 把更老的簇并入 survivor 时'
  '**不重选**(故可能 ≠「最早非 merged 成员」,这是允许的);P-3 改 P6 选取规则时须自行补迁移重算。';

-- (3) raw 层转载组代表(P8 用)。本迁移**只建列**,不分组、不填充 —— 本版无消费方。
ALTER TABLE raw_documents ADD COLUMN IF NOT EXISTS canonical_id UUID;

COMMENT ON COLUMN raw_documents.canonical_id IS
  'raw 层转载组代表行;NULL = 自己是代表(默认)。issue #83 P8 的「哪些渠道报道 + 各自 url」'
  '走本列(raw 层是全集,事件层是子集)。⚠️ 分组与回填是 **P-8 的活**,本版全为 NULL;'
  '在 P-8 落地前**不得**实现基于本列的读路径(会显示 0 个来源)。';

-- (4) 人工标记(P4 #7)。**引擎永不触碰** —— 幂等重跑只动 status/cluster_id/updated_at。
ALTER TABLE events ADD COLUMN IF NOT EXISTS human_verdict TEXT;

COMMENT ON COLUMN events.human_verdict IS
  '人工标记(issue P4 #7)。**引擎永不触碰** —— 幂等重跑(含 MergeClusters 的 SET status=CASE)'
  '只动 status/cluster_id/updated_at。NULL = 无人工意见(默认,**≠ 否定**)。'
  '取值域待 P-3/P-4 定义,故**不加 CHECK**。';

-- 回填:每个簇取「最早创建、同则更高置信」的**非 merged** 成员 —— 与
-- internal/cluster/cluster.go ApplyClusters 的比较器**逐字一致**(created_at ASC, confidence DESC)。
-- 用 `<> 'merged'` 而非白名单:与 countActiveMembers 同口径,且新增 status 值时不漏(issue §2.1 #5 教训)。
-- 无在产成员的簇(已被 MergeClusters 吸走)保持 NULL —— 合法状态,非漏回填。
-- 幂等:重复执行结果相同。
UPDATE event_clusters c
SET canonical_event_id = m.event_id
FROM (
  SELECT DISTINCT ON (cluster_id) cluster_id, id AS event_id
  FROM events
  WHERE cluster_id IS NOT NULL
    AND status <> 'merged'
  ORDER BY cluster_id, created_at ASC, confidence DESC
) m
WHERE c.id = m.cluster_id
  AND c.canonical_event_id IS DISTINCT FROM m.event_id;
