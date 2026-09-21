-- 公告分级标签(issue #68 A 层):raw_documents 增 grade 列。
--
-- 动机:
--   巨潮公告实测 ~1200 条/交易日(2026-09-15 实测 1800 条),前端消息页「公告」tab
--   全量平铺 —— 用户「看不过来」。分级后**必读+重要**约 11~20%,其余可折叠
--   (实测 5 个交易日可丢弃率 79.9%~87.5%,见 internal/announce/grade_test.go)。
--
-- ⚠️ 为什么分级只能靠标题(勿试图用 announcementType):
--   巨潮 announcementType 是**不可解数字码**("01010503||010112||010115||012325"),
--   announcementTypeName 实测逐条为 null、字典端点不可得
--   (见 internal/collector/cninfo_announce.go 文件头实测记录)。
--   故分级 = 纯规则标题关键词(internal/announce/grade.go),**不猜测类型码含义**。
--
-- ⚠️ 分级**不是进 LLM 的门槛**:公告现状 status='collected' —— 不进 worker(只取 'raw')、
--   不报对账异常(issue #50 T4 决定),故公告 LLM 成本本就是 0,分级**不产生成本收益**。
--   它的价值纯在**展示层折叠**。将来若要「必读+重要进抽取」是独立改判,须同步改
--   worker 取值口径 + reconcile 口径。
--
-- 取值与 CHECK 约束与 internal/announce.Grade 的返回值严格一致:
--   must / important / routine / noise
-- NULL = 未分级(非公告源,如快讯;或本迁移前的历史公告行)。
--
-- 索引:公告 tab 需要按级别折叠 / 过滤,故对公告行建 (grade) 部分索引。
-- 公告行少(~1200/日),索引成本可忽略。

ALTER TABLE raw_documents ADD COLUMN IF NOT EXISTS grade TEXT;

ALTER TABLE raw_documents DROP CONSTRAINT IF EXISTS raw_documents_grade_check;
ALTER TABLE raw_documents ADD CONSTRAINT raw_documents_grade_check
  CHECK (grade IS NULL OR grade IN ('must','important','routine','noise'));

COMMENT ON COLUMN raw_documents.grade IS
  '公告分级(issue #68 A 层,仅 source_type=announcement):must/important/routine/noise;'
  'NULL=未分级(快讯源或本迁移前历史行)。机器判定(Inference)非事实,UI 须如实标注。'
  '规则见 internal/announce/grade.go —— 只能靠标题,巨潮类型码不可解。';

-- 公告 tab 的级别过滤(折叠/展开)走此索引。
CREATE INDEX IF NOT EXISTS idx_raw_docs_grade
  ON raw_documents(grade) WHERE grade IS NOT NULL;
