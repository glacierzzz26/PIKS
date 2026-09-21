-- 去重键按源的能力分派(issue #50 T4:公告接入)。
--
-- 动机(2026-09-20 实测,非估算):
--   raw_documents 原去重键 UNIQUE(source_id, content_hash) 以**归一化正文**为键。
--   快讯源(content=正文)适用;但公告驱动只存标题(正文接口被上游限流,见
--   docs/phase11/design/announcement-source.md),content 即标题的占位。
--   实测 2026-09-17 东财全市场公告 **1377 行却只有 1375 个不同 title**——
--   2 条同日同题不同 art_code 被标题键**静默合并**,永久丢失
--   (「关于N沈鼓(601091)盘中临时停牌的公告」/「000875电投绿能投资者关系管理信息20260917」)。
--   公告标题由交易所模板生成,重名是系统性的,不是偶发。
--
-- 修法:去重键按**源是否携带稳定上游标识**分派,不做一刀切:
--   1) external_id 为空 → 沿用 (source_id, content_hash)——快讯源行为**逐字不变**
--      (语料里 external_id 为 NULL 的行仍按正文去重,不因本迁移改变去重语义);
--   2) external_id 非空 → 按 (source_id, external_id, content_hash) 去重。
--      上游稳定标识(东财 art_code / 财联社 id / 新浪 id …)本就唯一,将它纳入键后,
--      同题不同公告不再互撞;同时保留 content_hash,使**上游改内容**时仍能捕获变化。
--
-- 等价性:UNIQUE 视 NULL 为彼此不同,故旧键 (source_id, content_hash) 无法直接表达
-- 「NULL 时退化为二列」;改用两条 partial unique index 表达同一语义,并删除旧约束。
-- 快讯源(external_id 非空且唯一,实测 external_id 重复数=0)在新区分下去重结果与旧键一致。
--
-- 经核查 content_hash 全仓仅用作去重键(无任何读取方),故本迁移不影响既有读者。

ALTER TABLE raw_documents DROP CONSTRAINT IF EXISTS raw_documents_source_id_content_hash_key;

-- (1) 无上游标识的源(如 file 保底驱动)：维持按正文去重。
CREATE UNIQUE INDEX IF NOT EXISTS uq_raw_docs_src_hash_noext
  ON raw_documents(source_id, content_hash)
  WHERE external_id IS NULL;

-- (2) 有上游标识的源(事件类多源 + 公告)：按上游标识 + 正文去重。
CREATE UNIQUE INDEX IF NOT EXISTS uq_raw_docs_src_ext_hash
  ON raw_documents(source_id, external_id, content_hash)
  WHERE external_id IS NOT NULL;

COMMENT ON COLUMN raw_documents.status IS
  'raw=待抽取 / processed=已抽取 / failed=抽取失败 / collected=已采集无需抽取(公告等原始事件源,issue #50)';
