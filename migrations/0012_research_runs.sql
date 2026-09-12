-- 个股深研报告归档(research 并入,design research-merge.md D-5/D-9)。
-- 一个 run = 一次深研快照;同 code 多 run = 时间序列(支撑后续 delta 复研)。
-- metrics 是数字唯一源(Fact);synthesis 是 LLM 定性(Opinion);两者严格分域。
-- status 状态机:pending → gathering → synthesizing → verifying → done / failed。
CREATE TABLE IF NOT EXISTS research_runs (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id     TEXT NOT NULL UNIQUE,          -- research 侧 run_id(幂等键,来自 run_meta.json)
  code       TEXT NOT NULL,                 -- 归一 6 位(join entities.detail->>'code')
  symbol     TEXT NOT NULL,                 -- research full_code(如 sh600519)
  profile    TEXT NOT NULL,                 -- complete-stock / short-term
  as_of      DATE NOT NULL,                 -- 数据截止交易日(防未来函数基准)
  status     TEXT NOT NULL DEFAULT 'pending',
  metrics    JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {code}_metrics.json(数字唯一源 = Fact)
  synthesis  JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {summary,trend,conclusion}(LLM 定性 = Opinion)
  markdown   TEXT,                                -- 最终报告(机检通过)或骨架报告(未通过)
  lint       JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {scanned,matched,ignored,passed,issues[]}
  gate       JSONB NOT NULL DEFAULT '{}'::jsonb,  -- 六项机检结果
  evidence   JSONB NOT NULL DEFAULT '[]'::jsonb,  -- research Evidence 链(迭代 3 再并入 evidences 表)
  error      TEXT,                                -- 失败原因(如实,不掩盖)
  model      TEXT NOT NULL DEFAULT '',            -- 合成所用模型(如实标注)
  tokens     BIGINT NOT NULL DEFAULT 0,           -- 本次合成 token(task_runs 同记,双份留痕)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 同股按时间倒序取(实体页/持仓行「查看报告」列表)
CREATE INDEX IF NOT EXISTS idx_research_runs_code ON research_runs(code, as_of DESC);
-- 状态筛选(编排命令查 pending/failed 重试;前端轮询)
CREATE INDEX IF NOT EXISTS idx_research_runs_status ON research_runs(status);
