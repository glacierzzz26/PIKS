/** 领域类型 —— 与 internal/model 及 migrations 保持一致（只读投影） */

export type EventItem = {
  id: string;
  title: string;
  event_type: string;
  summary: string;
  facts: string[];
  affected: { word: string; entity_id?: string; entity_name?: string }[];
  occurred_at: string;
  confidence: number;
  status: "confirmed" | "pending" | "archived";
  source: string;
  source_url?: string;
};

export type Entity = {
  id: string;
  type: "company" | "industry" | "concept" | "person" | "region";
  name: string;
  aliases: string[];
  description: string;
  status: "active" | "watch" | "archived";
  updated_at: string;
};

export type Relationship = {
  id: string;
  from_id: string;
  to_id: string;
  rel_type: string;
  confidence: number;
};

export type LimitUpStock = {
  code: string;
  name: string;
  boards: number; // 连板数
  seal_amount: number; // 封单额（元）
  first_time: string; // 首次涨停时间
  industry: string;
  reason: string;
  turnover: number; // 换手率 %
  float_mv: number; // 流通市值（亿元）
};

export type MarketSnapshot = {
  trade_date: string;
  indices: { name: string; close: number; change_pct: number }[];
  limit_up: number;
  limit_down: number;
  broken_limit: number;
  max_board: number;
  turnover_yi: number; // 两市成交额（亿）
  emotion_score: number; // 0-100
  emotion_state: string;
  ladder: LimitUpStock[];
  industry_dist: { name: string; count: number }[];
};

export type Flash = {
  id: string;
  time: string;
  content: string;
  source: string;
  important: boolean;
  event_id?: string;
};

export type Doc = {
  id: string;
  type: "daily-review" | "note" | "weekly" | "mistake";
  slug: string;
  title: string;
  updated_at: string;
  content: string; // Markdown
};
