-- 周报 AI 综述缓存(iter4 D26,Web 适配,design weekly-ai-summary.md)。
-- 综述每周一次、高智档生成:缓存避免每次 GET /weekly 重复调 LLM;week UNIQUE 防同周重复行。
CREATE TABLE IF NOT EXISTS weekly_summaries (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  week       TEXT NOT NULL UNIQUE,          -- ISO 周标签,如 2026-W35
  summary    TEXT NOT NULL,                 -- 综述正文(高智档生成)
  model      TEXT NOT NULL,                 -- 生成所用模型(如实标注)
  tokens     BIGINT NOT NULL DEFAULT 0,     -- 本次生成 token(task_runs 同记,双份留痕)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
