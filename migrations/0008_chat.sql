-- 迭代 5-3:AI 对话 + 截图。设计 §4.4(对话历史)+ §5.2(新表)。
-- 单人系统:单个默认会话即可,不做会话管理 UI(设计标 可选)。
-- 附件文件本体存 data/uploads/{date}/,本表只存元数据(路径引用)。
CREATE TABLE IF NOT EXISTS chat_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL DEFAULT '对话',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS chat_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
  role TEXT NOT NULL,                                   -- user / assistant
  content TEXT NOT NULL DEFAULT '',
  refs JSONB NOT NULL DEFAULT '{}'::jsonb,              -- {"events":[...],"entities":[...]} 引用(答案可点跳转)
  attachments JSONB NOT NULL DEFAULT '[]'::jsonb,       -- 关联截图附件 id 列表
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages(session_id, created_at);

CREATE TABLE IF NOT EXISTS attachments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  filename TEXT NOT NULL,                               -- 原始文件名
  mime TEXT NOT NULL,
  size BIGINT NOT NULL DEFAULT 0,
  path TEXT NOT NULL,                                   -- data/uploads/{date}/{uuid}.{ext}
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
