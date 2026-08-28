-- 持仓 AI 诊断缓存(交易闭环,design trade-loop.md D-L1)。
-- 诊断按 snapshot_date 缓存(一天一份):避免每次 GET /trades 重复调 LLM;
-- snapshot_date UNIQUE 防同日重复行;新快照上传(snapshot_date 变化)或点按钮才重生成。
CREATE TABLE IF NOT EXISTS position_reviews (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  snapshot_date DATE NOT NULL UNIQUE,       -- 对应 positions.snapshot_date(快照日)
  review        JSONB NOT NULL DEFAULT '{}'::jsonb, -- {review, refs, risks, model, tokens, generated_at}
  model         TEXT NOT NULL DEFAULT '',   -- 生成所用模型(如实标注)
  tokens        BIGINT NOT NULL DEFAULT 0,  -- 本次生成 token(task_runs 同记,双份留痕)
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_position_reviews_date ON position_reviews(snapshot_date DESC);
