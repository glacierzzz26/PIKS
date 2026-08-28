-- 交易功能(design trades.md):「我做了什么」接入知识库。
-- trades=交易记录(同花顺今日交易截图抽取 / 手动录入,结构化事实);positions=持仓快照(只存只展示)。
-- review JSONB 存 AI 带引用复盘;attachment_id 弱引用 attachments(不建 FK,截图可清理)。
CREATE TABLE IF NOT EXISTS trades (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  trade_date  DATE NOT NULL,              -- 交易日期
  code        TEXT NOT NULL,              -- 证券代码(6 位)
  name        TEXT NOT NULL,              -- 证券名称
  side        TEXT NOT NULL,              -- buy / sell
  price       NUMERIC(12,3) NOT NULL,     -- 成交价
  qty         INT NOT NULL,               -- 数量(股)
  amount      NUMERIC(16,2) NOT NULL,     -- 成交金额
  source      TEXT NOT NULL DEFAULT 'manual',  -- manual / screenshot
  attachment_id UUID,                     -- 来源截图附件
  note        TEXT,                       -- 自评(可选)
  review      JSONB NOT NULL DEFAULT '{}'::jsonb, -- AI 复盘 {review,refs,model,tokens,generated_at}
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_trades_date ON trades(trade_date DESC, created_at);
CREATE INDEX IF NOT EXISTS idx_trades_code ON trades(code);

CREATE TABLE IF NOT EXISTS positions (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  snapshot_date DATE NOT NULL,            -- 持仓快照日
  code          TEXT NOT NULL,
  name          TEXT NOT NULL,
  qty           INT NOT NULL,             -- 持有数量
  cost_price    NUMERIC(12,3),            -- 成本价
  price         NUMERIC(12,3),            -- 现价
  market_value  NUMERIC(16,2),            -- 市值
  pl            NUMERIC(16,2),            -- 盈亏
  source        TEXT NOT NULL DEFAULT 'screenshot',
  attachment_id UUID,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_positions_date ON positions(snapshot_date DESC);
