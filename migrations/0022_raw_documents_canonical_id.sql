-- 事件管线 P-4(issue #83 分期 P-4):raw 层转载组代表列(canonical_id)的**索引** + 读路径契约。
--
-- 范围:
--   P-2(迁移 0021)已建 raw_documents.canonical_id 但**留空**,并明写「填充属 P-8」。
--   本期就是 P-8,补上**分组与回填**:
--     · 分组判据 = P-1 的正文指纹(**复用** internal/cluster/reprint.go 的 GroupReprints,
--       阈值 0.85,不另立第二把尺子 —— 见 docs/phase11/design/event-pipeline-p4.md §2);
--     · 回填器 = cmd/cluster-raw-link(纯函数 internal/cluster/reprint_raw.go);
--     · 本迁移**只建索引**,数据由命令回填(与本仓库其余迁移「DDL only」一致)。
--
-- 🔴 代表**冻结**(与 event_clusters.canonical_event_id 同一纪律):
--   已非 NULL 的行**永不重选** —— 只写仍为 NULL 的。否则「今天 A 代表、明天 B 代表」会让
--   url 归属漂移(issue §三 P4「幂等隐患:代表漂移」)。
--
-- 🔴 读路径的**前置条件**:基于本列的读路径(`store.ListClusterRawSources`)**只在回填之后**有意义。
--   空库/未跑回填时,COALESCE(canonical_id, id) 退化为「自己就是代表」—— 即 raw 全集里
--   每条各自成组。这是**合法退化**(等价于「没有转载合并」),不会报错、不会显示 0 个来源。
--   (P-2 注释里「会显示 0 个来源」是对「直接 JOIN canonical_id 而不 COALESCE」的告警;
--   本期的读路径一律用 COALESCE(canonical_id, id),故 NULL 是安全默认值。)
--
-- ⚠️ 与 ListClusterSources(簇层)的**判据关系**:后者是**事件层**实时跑 P-1 指纹(真源);
--   本列是 raw 层把同一判据**回填成的快照**。两者应一致,但因回填水位(采集后是否已跑
--   cluster-raw-link)可能短暂不同步 —— 登记为已知边界,不得静默。

-- 部分索引(照 0017/0018 风格):读路径按本列分组取数,只索引非 NULL 的行。
CREATE INDEX IF NOT EXISTS idx_raw_docs_canonical ON raw_documents(canonical_id)
  WHERE canonical_id IS NOT NULL;

COMMENT ON COLUMN raw_documents.canonical_id IS
  'raw 层转载组的代表行;NULL = 自己是代表(默认)。「哪些渠道报道 + 各自 url」走本列'
  '(raw 层是全集,事件层是子集,issue #83 P8)。'
  '⚠️ 分组与回填由 cmd/cluster-raw-link 完成(P-4 落地),判据 = P-1 正文指纹 @0.85;'
  '已非 NULL 的行**冻结不重选**。读路径**必须** COALESCE(canonical_id, id) —— '
  '未回填时退化为「自己就是代表」(合法,等价于无转载合并),不得直接 JOIN 本列(会显示 0 个来源)。';
