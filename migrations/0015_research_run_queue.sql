-- 深研 DB 队列(2026-09-20 容器拆分 P2):web 只建 pending 行,常驻 worker 认领并驱动编排。
--
-- 动机:拆镜像后 web 容器不再含 Python 运行时(os/exec python3 会必挂,2026-09-12 已证),
-- 故触发的「执行方」必须从 web 进程内 goroutine 移出,改为入库排队 + 独立进程认领。
-- 零新表 —— 队列就是 research_runs.status 列(pending 即可认领),不引入队列框架
-- (与 docs/项目详解.md「V1 不上队列框架,任务用 cron + pg 任务表」一致)。
--
-- 本迁移只加两列 + 两个局部索引:
--   1. quick/days:web 知道、worker 需要,但 pending 行不携带 → 随行落库(否则认领后无从还原)。
--      与 task_runs.meta 的既有惯例同构(结构化参数入 JSONB/列,而非塞进 status 污染状态机)。
--   2. 局部索引:认领只扫 pending(按 created_at FIFO),reaper 只扫进行中(按 updated_at 心跳)。
--      带 WHERE 的局部索引让两者的计划都是 Index Scan,不与其它状态的行竞争。
ALTER TABLE research_runs ADD COLUMN IF NOT EXISTS quick BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE research_runs ADD COLUMN IF NOT EXISTS days  INT     NOT NULL DEFAULT 0;

COMMENT ON COLUMN research_runs.quick IS '快速模式:合成可选(RequireSynthesis=false),无 AI 也出确定性结论(买入前速评用)';
COMMENT ON COLUMN research_runs.days  IS '展示窗口覆盖(交易日);0 = 用 profile 默认';

-- 认领查询(pending,created_at ASC)的计划索引。
CREATE INDEX IF NOT EXISTS idx_research_runs_pending
  ON research_runs(created_at) WHERE status = 'pending';

-- 孤儿回收(reaper 按 updated_at 心跳判定:进行中但久未更新 = 推进它的进程没了)。
-- 刻意不加独立的 heartbeat/claimed_by 列:updated_at 已是每次状态/产物写入的 now(),
-- 单一真源更稳(claim 自身也写 updated_at,故认领即开始心跳)。
CREATE INDEX IF NOT EXISTS idx_research_runs_heartbeat
  ON research_runs(updated_at)
  WHERE status IN ('pending','gathering','synthesizing','verifying');
