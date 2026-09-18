-- 账户级资金快照(issue #19,design trades.md):同花顺「持仓」页顶部的账户汇总。
-- 与 positions 同 snapshot_date 关联 —— positions 逐股,本表账户级。
--
-- ⚠️ 四项**全部可空**:截图没显示就 NULL,**禁止推断**(与 prompt 诚实规则一致)。
--    NULL ≠ 0:0 是"确实为零",NULL 是"截图没有这个数",展示层必须区分(缺则不显示,不填 0)。
--
-- 口径(issue #19 §期望 4,勿混):
--   total_asset 总资产    = 持仓市值 + 可用资金/余额(唯一含现金的口径;PIKS 只存截图值,不建资金流水)
--   total_mv    总市值    = 持仓市值合计(不含现金)
--   float_pl    浮动盈亏  = 持仓累计浮盈(不含现金;≠当日)
--   daily_pl    当日参考盈亏 = 按**当日**价格变动的浮盈(与累计浮盈是两个维度)
-- snapshot_date UNIQUE:一天一份账户快照,同日重传覆盖(upsert),与 position_reviews 同手法。
CREATE TABLE IF NOT EXISTS account_snapshots (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  snapshot_date DATE NOT NULL UNIQUE,        -- 对应 positions.snapshot_date(快照日)
  total_asset   NUMERIC(16,2),               -- 总资产(含现金);截图缺 → NULL
  total_mv      NUMERIC(16,2),               -- 总市值(不含现金);截图缺 → NULL
  float_pl      NUMERIC(16,2),               -- 浮动盈亏(累计);截图缺 → NULL
  daily_pl      NUMERIC(16,2),               -- 当日参考盈亏;截图缺 → NULL
  source        TEXT NOT NULL DEFAULT 'screenshot',  -- manual / screenshot
  attachment_id UUID,                        -- 来源截图附件
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_account_snapshots_date ON account_snapshots(snapshot_date DESC);
