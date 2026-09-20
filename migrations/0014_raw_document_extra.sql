-- 事件类多源采集(issue #43 T1):raw_documents 增 extra 列,原样保存上游原始字段。
--
-- 动机(issue #43 §「上游免费给热度信号,akshare 全丢了」):
--   上游免费给的热度/分级/溯源字段被中间层丢弃后,**时序信号不可回溯补采**——
--   这些接口只回最近 20–50 条,今天不留,三个月后想加进排序也补不回来。
--   故 raw 层必须**原样存**上游原始 JSON,不以「现在用不上」为由裁剪。
--
-- 实测各源被丢/该留的字段(2026-09-20):
--   财联社  reading_num(阅读数)、level(A/B/C)、confirmed、shareurl、stock_list
--   金十    important(重要度)、data.source(一级源,如「新华社」)、tags、channel
--   新浪    like_nums、comment_list、tag、docurl、ext.mdocurl
--   东财    pinglun_Num 等既有字段(此前未落)
--
-- 与 content_hash 去重的关系:extra **不参与** content_hash(去重键仍为归一化正文),
-- 同一源同一正文重复采集不会因 extra 微变产生新行(UNIQUE(source_id, content_hash) 不变)。
ALTER TABLE raw_documents ADD COLUMN IF NOT EXISTS extra JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN raw_documents.extra IS '上游原始字段原样留存(热度/分级/溯源等);不参与去重键(issue #43)';
