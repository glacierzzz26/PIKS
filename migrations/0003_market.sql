-- PIKS 迭代2 市场情报 schema
-- 追加式应用,不破坏 0001/0002(迭代0/1 已冻结)。

-- 2.1 每日市场状态快照(设计 §2.1,架构 §9.7 Market + §9.8 Emotion)
-- 一日一行;emotion 并入(score/state/detail),不拆 emotions 表(定稿 D14)。
CREATE TABLE market_snapshots (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  trade_date          DATE NOT NULL UNIQUE,       -- 交易日(东财 qdate 确认,非交易日不建行)
  index_json          JSONB,          -- {"sh":{"close":3912.52,"pct":0.59},"sz":{...},"cyb":{...}}
  turnover_amt        NUMERIC(16,2),  -- 两市成交额(亿),源待核验,可 NULL
  breadth             JSONB,          -- {"advance":n,"decline":n,"flat":n}
  limit_up_count      INT,
  limit_down_count    INT,
  broken_limit_count  INT,            -- 炸板
  max_board           INT,            -- 最高连板
  zt_pool             JSONB,          -- 涨停池精简快照 [{code,name,lbc,hybk,fund}]
  strong_yesterday    JSONB,          -- {"avg_ret":x,"count":n} 昨日涨停今日表现
  industry_dist       JSONB,          -- {"家居用品":5,"其他电源":2,...}
  hot_topics          JSONB,          -- [{name:"...", event_ids:[...]}] 从 events 派生
  top_events          JSONB,          -- [event_id,...] 当日重要事件
  capital_flow        JSONB,          -- 资金(源待定),可 NULL
  emotion_score       NUMERIC,        -- 规则加权得分(设计 §2.2)
  emotion_state       TEXT,           -- Extreme_Fear/Fear/Weak/Neutral/Warm/Strong/Extreme_Greed
  emotion_detail      JSONB,          -- 各组件分值明细,可解释(设计 §2.2)
  my_judgment         TEXT,           -- 预留(不自动写;个人判断在 09-Personal)
  evidence            JSONB,          -- 数据源清单 [{endpoint,fetched_at,count}](可信度/血缘)
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_market_snapshots_date ON market_snapshots (trade_date DESC);

-- observations 已有 0001 建表;补 market+indicator+observed_at 索引(填充契约 §2.3 查询)
CREATE INDEX idx_observations_market_indicator ON observations (market, indicator, observed_at);
