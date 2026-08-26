-- PIKS 迭代1 可靠性 schema
-- 追加式应用,不破坏 0001(迭代0 已冻结)。

-- 2.1 事件簇:同一真实事件的报道集合(设计 D8)
CREATE TABLE event_clusters (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title      TEXT NOT NULL,                      -- 簇规范标题(LLM 生成或取 canonical 事件标题)
  status     TEXT NOT NULL DEFAULT 'active',     -- active/merged/archived
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2.2 events 增列
ALTER TABLE events ADD COLUMN cluster_id   UUID REFERENCES event_clusters(id);
ALTER TABLE events ADD COLUMN published_at TIMESTAMPTZ;  -- 增量发布锚点

CREATE INDEX idx_events_cluster_id    ON events(cluster_id);
CREATE INDEX idx_events_published_at  ON events(published_at);
