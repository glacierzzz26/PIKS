-- 热榜快照(issue #68 D 层:数据源分层的 D 层 —— 事件/题材热榜)。
--
-- 动机:
--   设计见 docs/phase11/design/hot-topic.md(源选型侦察见 issue #68 评论 2026-09-21)。
--   A 股**事件维度**热榜只有两家可用源,且都是**硬上限的小列表**:
--     同花顺话题榜        ~15 条,带真实 hot_value(数值)+ 关联个股
--     财联社首页热文 SSR  ~13 条,带真实 readNum(数值)
--   二者**粒度不同**(同花顺=题材/事件,财联社=文章/复盘),实测**同题对 = 0**
--   (用生产聚类同尺 NormalizeTitle+Jaccard@0.7 比对 615 个跨源对,J≥0.5 都是 0)。
--   → 故**各出各的、不合并、不加权、不排名**(方案 A);混排必然误导。
--
-- 🔴 红线(设计 §6 / §10,本表实现须逐条兑现):
--   1. 热度**只作展示/排序权重,绝不作为「重要性」判定依据**;
--   2. **不得与印证度(source_count / cluster_sources)合并计算** —— 二者正交
--      (印证度 = 几家在报;热度 = 多少关注);
--   3. 故本表**不进 raw_documents、不进聚类、不触碰 events 的任何印证/排序逻辑** ——
--      独立表 + 独立页,与事件链路**零交集**;
--   4. 热度**仅源内可比,跨源不可比**(同花顺 hot_value 与财联社 readNum 量纲不同;
--      财联社**榜内**自己就相差 9 倍,更不可跨源比)。
--
-- ⚠️ 为何独立成表而非塞进 market_snapshots.hot_topics(勿混):
--   `market_snapshots.hot_topics` 是**派生字段**(cmd/market-state 由「涨停行业 top5 +
--   当日事件 top3」算出),**不是**采集来的热榜源。两者同名不同物,故本表命名
--   `hot_topic_items` 刻意避开 `hot_topics`,防后来者误接。
--
-- ⚠️ 为何不加 collected_at 之外的额外去重键:
--   本表是**时序快照**表,同一话题在相邻快照里重复出现是**预期行为**(热度走势要靠这个),
--   故**不设唯一约束**,只按 (source, rank, snapshot_at) 建查询索引。
--   留存期(TTL)属 #45 续篇,本迁移不做(见设计文档「不做」)。
CREATE TABLE IF NOT EXISTS hot_topic_items (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source      TEXT NOT NULL,               -- ths-topic / cls-hot-article(枚举见 internal/collector/hottopic.go)
  rank        INT  NOT NULL,               -- 该源**榜内**名次(1-based);仅源内可比,跨源不可比
  title       TEXT NOT NULL,               -- 话题标题(原文,不改写)
  hot_value   BIGINT,                      -- 该源口径的热度数值;NULL = 上游没给(不猜、不填 0)
  url         TEXT,                        -- 原文/话题链接;NULL = 上游没给可点链接
  extra       JSONB NOT NULL DEFAULT '{}'::jsonb,  -- 上游原始字段原样留档(issue #43 惯例):
                                                   -- 同花顺关联个股 / 财联社 brief·author·ctime
  snapshot_at TIMESTAMPTZ NOT NULL,        -- 本轮采集时刻(同批同值,便于按批分列)
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 取「某源最新一批」用:WHERE source=$1 ORDER BY snapshot_at DESC, rank ASC。
CREATE INDEX IF NOT EXISTS idx_hot_topic_items_src_snap
  ON hot_topic_items(source, snapshot_at DESC, rank);

COMMENT ON TABLE hot_topic_items IS
  '热榜时序快照(issue #68 D 层)。⚠️ 热度仅源内可比、绝不作为重要性判定,不得与印证度'
  '(source_count/cluster_sources)合并计算 —— 独立表 + 独立页,与事件链路零交集(设计 §6 红线)。'
  '不进 raw_documents、不进聚类。同一话题跨快照重复是预期(热度走势靠它)。';

COMMENT ON COLUMN hot_topic_items.rank IS
  '该源榜内名次(1-based)。仅源内可比 —— 两源粒度不同(题材 vs 文章),不许跨源排名混排(设计 §6)。';
COMMENT ON COLUMN hot_topic_items.hot_value IS
  '该源口径热度:同花顺 hot_value / 财联社 readNum。NULL = 上游未给(不填 0 —— 0 与「没有」是两回事)。';
