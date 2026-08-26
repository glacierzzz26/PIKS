-- PIKS 迭代3 实体补全 schema(设计 §2.1)
-- 单表 entities(type 判别 + detail JSONB),复用 relationships 多态(迭代0 已建,不新建关系表)。

-- 3.8 实体(Entity,架构 §9.1 继承模型落地)
CREATE TABLE entities (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type        TEXT NOT NULL,          -- company/industry/concept/topic/unknown
  name        TEXT NOT NULL,          -- 规范名(如"银行")
  aliases     JSONB NOT NULL DEFAULT '[]'::jsonb,  -- ["银行板块","银行股"] 措辞变体
  description TEXT,
  detail      JSONB NOT NULL DEFAULT '{}'::jsonb,  -- company:{code,exchange} industry:{source:eastmoney}
  status      TEXT NOT NULL DEFAULT 'active',       -- active/archived
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (type, name)
);
CREATE INDEX idx_entities_type ON entities(type);
CREATE INDEX idx_entities_name ON entities(name);
