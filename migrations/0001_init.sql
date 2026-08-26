-- PIKS 迭代0 最小闭环 schema
-- PostgreSQL 16+ (gen_random_uuid() 为内置函数,无需 pgcrypto)

-- 3.1 数据源
CREATE TABLE sources (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL UNIQUE,
  source_type TEXT NOT NULL,                    -- news/policy/market/report/announcement/macro/history
  config      JSONB NOT NULL DEFAULT '{}'::jsonb,
  status      TEXT NOT NULL DEFAULT 'active',   -- active/paused/dead
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3.2 原始文档(去重与血缘锚点)
CREATE TABLE raw_documents (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id        UUID NOT NULL REFERENCES sources(id),
  external_id      TEXT,
  url              TEXT,
  title            TEXT,
  content          TEXT NOT NULL,
  content_hash     TEXT NOT NULL,                -- sha256(归一化 content)
  published_at     TIMESTAMPTZ,
  retrieved_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  status           TEXT NOT NULL DEFAULT 'raw',  -- raw/processed/failed
  pipeline_version TEXT,
  error            TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (source_id, content_hash)
);
CREATE INDEX idx_raw_docs_status ON raw_documents(status);

-- 3.3 事件(Fact 层)
CREATE TABLE events (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  raw_document_id  UUID REFERENCES raw_documents(id),
  title            TEXT NOT NULL,
  event_type       TEXT NOT NULL,                -- policy/earnings/industry/accident/international/tech/macro/company/other
  summary          TEXT,
  facts            JSONB NOT NULL DEFAULT '[]'::jsonb,
  affected         JSONB NOT NULL DEFAULT '[]'::jsonb,
  occurred_at      TIMESTAMPTZ,
  confidence       NUMERIC(3,2) NOT NULL DEFAULT 0,
  status           TEXT NOT NULL DEFAULT 'extracted', -- discovered/processed/extracted/verified/published/archived
  pipeline_version TEXT,
  source_id        UUID REFERENCES sources(id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  valid_from       TIMESTAMPTZ,
  valid_to         TIMESTAMPTZ
);
CREATE INDEX idx_events_raw_doc     ON events(raw_document_id);
CREATE INDEX idx_events_occurred_at ON events(occurred_at);
CREATE INDEX idx_events_status      ON events(status);

-- 3.4 证据(V1: 事件即主张, raw doc 即证据)
CREATE TABLE evidences (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id     UUID REFERENCES events(id),
  claim        TEXT NOT NULL,
  source_id    UUID REFERENCES sources(id),
  source_type  TEXT,
  url          TEXT,
  title        TEXT,
  content      TEXT,
  published_at TIMESTAMPTZ,
  retrieved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reliability  TEXT,                            -- high/medium/low
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3.5 观测(V1 建表预留,迭代2 开始填充)
CREATE TABLE observations (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id       UUID REFERENCES events(id),
  market         TEXT NOT NULL,
  indicator      TEXT NOT NULL,
  value          TEXT NOT NULL,
  previous_value TEXT,
  change         TEXT,
  observed_at    TIMESTAMPTZ NOT NULL,
  source         TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 3.6 关系(通用有向边;因果链=模板+视图,非约束)
CREATE TABLE relationships (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  from_type   TEXT NOT NULL,
  from_id     UUID NOT NULL,
  to_type     TEXT NOT NULL,
  to_id       UUID NOT NULL,
  rel_type    TEXT NOT NULL,
  properties  JSONB NOT NULL DEFAULT '{}'::jsonb,
  confidence  NUMERIC(3,2),
  source      TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  valid_from  TIMESTAMPTZ,
  valid_to    TIMESTAMPTZ,
  UNIQUE (from_type, from_id, to_type, to_id, rel_type)
);

-- 3.7 任务运行(可观测性)
CREATE TABLE task_runs (
  id         BIGSERIAL PRIMARY KEY,
  command    TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'running',   -- running/success/failed
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at   TIMESTAMPTZ,
  error      TEXT,
  meta       JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
