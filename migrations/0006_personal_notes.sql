-- 0006 personal_notes:个人认知沉淀(Web 编辑,权威源=PG)。
-- 依据 web-app.md §5.1(改造 iter4:去 source_path/content_hash/harvested_at;
-- 加 author/updated_by;slug 保留为稳定键)。type 新增 note = 事件卡「我的理解」。
-- 关联复用 relationships(from_type='personal_note', rel_type='references'/'supports'/'contradicts'/'updates')。

CREATE TABLE IF NOT EXISTS personal_notes (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type        TEXT NOT NULL,                -- belief / case / mistake / note
  slug        TEXT NOT NULL,                -- 稳定键(用户起短名,如 "低价股并不代表便宜")
  title       TEXT,                         -- 标题
  status      TEXT NOT NULL DEFAULT 'hypothesis',  -- belief:hypothesis/active/confirmed/questioned/rejected;case/mistake/note:active/archived
  confidence  NUMERIC,                      -- belief 自评(可选,0~1)
  content     TEXT,                         -- 全文(正文)
  detail      JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {"sections":[{"section":"陈述","text":"..."}]}(预留结构化)
  author      TEXT NOT NULL DEFAULT 'me',   -- 单人系统,预留
  updated_by  TEXT,                         -- 手动编辑记录
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (type, slug)
);
CREATE INDEX IF NOT EXISTS idx_personal_notes_type   ON personal_notes(type);
CREATE INDEX IF NOT EXISTS idx_personal_notes_status ON personal_notes(status);
CREATE INDEX IF NOT EXISTS idx_personal_notes_updated ON personal_notes(updated_at);
