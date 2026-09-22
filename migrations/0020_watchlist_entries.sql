-- 自选加入价 / 加入日(同花顺「我的自选」自动同步,issue #87)。
--
-- 动机:
--   自选原靠「截图 → 视觉识别 → 人工确认」导入(见 docs/phase2/design/trades.md §2.3)。
--   本迁移支撑服务器侧定时自动同步(命令 watch-sync,每日 3 次),把同花顺云自选镜像进
--   PIKS。设计见 docs/phase11/design/watchlist-sync.md。
--
-- 🔴 为什么必须新表,绝不能塞 entities.detail(本表存在的**唯一**理由):
--   entities.detail 每轮被 entity-build 的 UpsertEntity **无条件覆盖**
--   (internal/store/entities.go 的 `UPDATE … detail=$4`)。写进 detail 的加入价/日
--   当天就会被抹掉。⚠️ 任何把加入价挪回 entities.detail 的改动都是错的。
--   反过来:自选「成员资格」仍在 entities.status(见下),本表只补价/日。
--
-- 🔴 成员资格真源 = entities.status,不在本表:
--   'watch' = 在自选 / 'active' = 库中有不在自选 / 'archived' = 曾自选已移出(迁移 0004)。
--   /api/v1/watchlist、/stock/:code、/entities?status=watch 全以它为真源,故**不迁表**。
--   本表以 code 为键,只为把 selfstock_detail 给的「加入价 + 加入日」挂上去。
--
-- 🔴 主键 = code 而非 entity_id:
--   ① 上游给的就是 code,成员资格本就 per-code;
--   ② entity_id 可能为 NULL(名称未解析 → 本轮不建实体,下轮补,见 internal/watchsync);
--   ③ 读路径(/api/v1/watchlist)本就以 code 聚合。
--
-- 🔴 移出不删行(removed_at 置位),与 entities.status='archived' 同构:保留历史。
--   re-add = 同一行 UPSERT,清 removed_at,并用**上游新值**覆盖加入价/日(同花顺会重新给);
--   在选期间重复同步(keep)则保留原值,避免上游 detail 抖动改写历史。
--
-- ⚠️ NULL vs 0:added_price = NULL 表示「上游未给 / ≤0 / 不可解析」,**绝不填 0**
--   (0 不是有效 A 股价格)。added_on 同理。原文留在 extra 便于审计。
CREATE TABLE IF NOT EXISTS watchlist_entries (
  code           TEXT PRIMARY KEY,             -- 6 位规范化代码(store.NormalizeCode 后);A 股全局唯一
  entity_id      UUID,                         -- entities.id;NULL = 名称未解析、实体未建(下轮补)
  market         TEXT NOT NULL,                -- SH/SZ/KC/CYB/BJ(internal/ths.MarketAbbr)
  market_id      TEXT NOT NULL,                -- 上游原始 marketid 数字码(17/33/…),留档不解释
  added_price    NUMERIC(12,4),                -- 同花顺加入价;NULL = 上游未给(见文件头 NULL vs 0)
  added_on       DATE,                         -- 同花顺加入日(T 字段);NULL = 上游未给/不可解析
  removed_at     TIMESTAMPTZ,                  -- 移出「我的自选」的时刻;NULL = 当前在自选
  first_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_synced_at TIMESTAMPTZ NOT NULL,         -- 本行最后一次出现在上游快照的时刻
  extra          JSONB NOT NULL DEFAULT '{}'::jsonb,  -- 上游原文 {C,M,P,T} + readd_count
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 当前在自选的行(唯一热查询);移出行不进索引。
CREATE INDEX IF NOT EXISTS idx_watchlist_entries_live
  ON watchlist_entries(code) WHERE removed_at IS NULL;
-- 按实体回查(个股中心 / 自选页富化)。
CREATE INDEX IF NOT EXISTS idx_watchlist_entries_entity
  ON watchlist_entries(entity_id) WHERE entity_id IS NOT NULL;

-- 常驻命令 watch-sync 的重启去重:同一 slot 当日是否已**成功**跑过
-- (TaskRunSlotDone 查 command + meta->>'slot' + started_at + status='success')。
CREATE INDEX IF NOT EXISTS idx_task_runs_command_started
  ON task_runs(command, started_at DESC);

COMMENT ON TABLE watchlist_entries IS
  '同花顺「我的自选」加入价/加入日(code 主键)。⚠️ 成员资格真源仍是 entities.status,'
  '本表只补价/日 —— 因 entities.detail 每轮被 entity-build 无条件覆盖,价/日不能进 detail。'
  '移出置 removed_at、不删行;re-add 用上游新值覆盖。';
COMMENT ON COLUMN watchlist_entries.added_price IS
  '同花顺加入价。NULL = 上游未给或 ≤0(0 不是有效 A 股价格,不填 0 也不臆造);原文留在 extra。';
COMMENT ON COLUMN watchlist_entries.removed_at IS
  '移出「我的自选」的时刻;NULL = 当前在自选。与 entities.status=''archived'' 同精神:移出不删行。';
