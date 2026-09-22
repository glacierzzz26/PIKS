-- 聚类「扫描水位」标记(issue #75:「cmd/cluster 全量重算不收敛」的根因修复)。
--
-- 动机(生产实测 2026-09-22):
--   候选池 = `cluster_id IS NULL`(`internal/store/events.go:220`),而 `BuildComponents`
--   只保留 `len(m) >= 2` 的分量(`internal/cluster/cluster.go:344`)——**从未匹配上对端的事件
--   永远拿不到 cluster_id**,于是永远留在池里,每轮被重新 O(pool²) 配对并送 LLM 确认。
--   生产 `unclustered = 1915 / 4083`(47%),单日实耗 ≈ 2350 万 token,把自建网关打成持续 429。
--
-- 修法:**给「本轮已扫过、且确实无对端」的事件盖一个水位戳**,下轮不再进池。
--   `cluster_scanned_at IS NULL` = 尚未扫描过 ⇒ 进候选池。
--   `cluster_scanned_at = ts`   = 已扫描过且当时无对端 ⇒ 不进正常 pass 池。
--
-- 🔴 关键纪律(踩过即漏数据,勿删):
--   本列**只解「正常 pass 的池边界」,不解「后续召回」**。被标记的事件 `cluster_id` **仍为 NULL**,
--   因此它**不是**活跃簇代表,`ListActiveClusterRepresentatives` 看不见它。若只把它从
--   `ListUnclusteredEvents` 排除,它会**同时从正常 pass 与重审视(reexamine)两处视野消失
--   ⇒ 永久漏召回**。故重审视的池查询**必须显式并入「窗口内的已标记事件」**
--   (`internal/cluster/reexamine.go`,谓词 `cluster_scanned_at IS NOT NULL`)。
--   换言之:标记列方案**只有成对实现才是召回等价的**,单独加列是错的。
--
-- 🔴 与时间窗成对(同样勿拆):reexamine 的池若不收窗,标记省下的正常 pass 开销会**原样搬到
--   reexamine**,且随每日新增递增(实测窗口口径下 `MAX(member.created_at)`,非簇表示的 created_at)。
--
-- ⚠️ 为何不建「单例簇」替代本列:单例簇会让每个事件都拿到 cluster_id,于是
--   ① `handleAPIEvents`(`internal/web/api_v1.go:216`)的 clusterIDs 数组从「多源簇数」涨到
--   **每个可见事件**(生产 4083),每次 GET /api/v1/events 都要把全部事件的 facts JSONB 读一遍;
--   ② 新增 ~1900 行**永不回收**的簇;
--   ③ 污染既有语义:`cluster_id IS NOT NULL` 在 `events.go:137`、`api_v1.go:47-49`、
--   `frontend/src/lib/types.ts:22-23` 三处均有明文注释「光看它分不清『就 1 家』和『还没聚类』」。
--
-- ⚠️ 召回边界(如实登记,不隐藏):相隔**超过 reexamine 窗口**才到达的重复事件不会被配对,
--   且两条都会被永久标记。实测(2026-09-22,生产库)**全部 746 个多源簇的成员 created_at 跨度
--   均 ≤ 1 天**(跨度 > 1 天的簇 = 0),故 `-window-days` 默认 7 有约 7 倍余量。
ALTER TABLE events ADD COLUMN IF NOT EXISTS cluster_scanned_at TIMESTAMPTZ;

-- 供 `ListUnclusteredEvents` 取池:谓词与查询完全一致(部分索引,只索引待扫事件)。
CREATE INDEX IF NOT EXISTS idx_events_cluster_scan_pending
  ON events(created_at)
  WHERE cluster_id IS NULL AND cluster_scanned_at IS NULL
    AND status IN ('extracted','verified','published');

COMMENT ON COLUMN events.cluster_scanned_at IS
  '聚类扫描水位(issue #75)。NULL = 尚未扫描;非 NULL = 已扫过且当时无对端,不再进正常 pass 池。'
  '⚠️ 只解正常 pass 池边界,**不解后续召回** —— 被标记事件 cluster_id 仍为 NULL、不是簇代表,'
  '故重审视(reexamine)池必须显式并入「窗口内的已标记事件」,否则永久漏召回。'
  '只标记「本轮真正比对过」的事件(limit 截断时池外事件不得标记)。';
