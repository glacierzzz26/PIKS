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
  type: string; // note/belief/case/mistake/daily-review/weekly（周报走 weekly）
  slug: string;
  title: string;
  updated_at: string;
  content: string; // Markdown
};

/** 笔记详情（GET /api/v1/notes/:id）：含状态/置信度/关联，编辑表单回填用。 */
export type NoteDetail = {
  id: string;
  type: string;
  slug: string;
  title: string;
  status: string;
  confidence?: number;
  content: string;
  updated_at: string;
  sel_events: string[];
  sel_entities: string[];
};

/** 笔记创建/更新请求体（POST/PUT /api/v1/notes）。 */
export type NoteInput = {
  type: string;
  slug?: string;
  title: string;
  status: string;
  confidence?: string;
  content: string;
  sel_events?: string[];
  sel_entities?: string[];
};

// ---- 只读投影 / 写 API 领域类型（对齐 internal/web/api_v1.go 响应） ----

export type ReviewPoint = {
  title: string;
  content: string;
};

/** 截图导入预览（POST /api/v1/trades/import 返回，确认时原样回传）。 */
export type PreviewTrade = {
  include: boolean;
  exists: boolean;
  date: string;
  code: string;
  name: string;
  side: string;
  price: string;
  qty: string;
  amount: string;
};

export type PreviewPosition = {
  include: boolean;
  code: string;
  name: string;
  qty: string;
  cost_price: string;
  price: string;
  market_value: string;
  pl: string;
};

export type ImportPreview = {
  kind: string;
  attachment_id: string;
  trades: PreviewTrade[];
  positions: PreviewPosition[];
};

export type TradeRow = {
  id: string;
  date: string;
  code: string;
  name: string;
  side: "buy" | "sell";
  price: number;
  qty: number;
  amount: number;
  source: string; // manual / screenshot
  note?: string;
  review?: string; // AI 复盘 Markdown（已生成时）
  mistakes?: ReviewPoint[]; // 复盘点（存为笔记用）
};

export type PositionRow = {
  code: string;
  name: string;
  qty: number;
  cost: number;
  last: number;
  pnl_pct: number;
};

export type ReviewRow = {
  date: string;
  scope: string;
  summary: string;
  refs: number;
  state: "positive" | "negative" | "neutral";
  risks?: ReviewPoint[];
};

export type SnapRow = {
  date: string;
  emotion_score: number;
  emotion_state: string;
  limit_up: number;
  limit_down: number;
  broken_limit: number;
  max_board: number;
};

export type TaskRun = {
  command: string;
  status: "ok" | "failed" | "running";
  time: string;
  note?: string;
};

export type ReconRow = {
  date: string;
  flashes: number;
  events: number;
  anomalies: number;
  status: "ok" | "warn" | "failed";
  note?: string;
};

export type ChatMsg = {
  role: "user" | "assistant";
  content: string;
  time: string;
  refs?: string[];
};

export type StatCard = { label: string; value: number };

export type TopEvent = { id: string; title: string; score: number };

export type DashboardData = {
  stats: StatCard[];
  market: MarketSnapshot;
  snap_history: SnapRow[];
  review: string;
  top_events: TopEvent[];
  task_runs: TaskRun[];
};

// ---- 周报交互页（weekly detail / generate） ----

export type WeeklySnap = {
  date: string;
  weekday: string;
  emotion: string;
  limit_up: number;
  limit_down: number;
  turnover: string;
  judgment: string;
};

export type WeeklyEvent = {
  id: string;
  title: string;
  date: string;
  event_type: string;
};

export type WeeklyNote = {
  id: string;
  title: string;
  type: string;
  type_label: string;
  updated: string;
};

export type WeeklyTrade = {
  date: string;
  name: string;
  code: string;
  side: string;
  side_label: string;
  qty: number;
  price: number;
  amount: number;
};

export type WeeklyPosition = {
  date: string;
  code: string;
  name: string;
  qty: number;
  cost: string;
  price: string;
  mv: string;
  pl: string;
};

export type WeeklySummary = {
  id: string;
  week: string;
  summary: string;
  model: string;
  tokens: number;
  created_at: string;
  updated_at: string;
};

export type WeeklyDetail = {
  week: string;
  range: string;
  offset: number;
  snaps: WeeklySnap[];
  events: WeeklyEvent[];
  notes: WeeklyNote[];
  trades: WeeklyTrade[];
  positions: WeeklyPosition[];
  summary: WeeklySummary | null;
  summary_note: string;
};

// ---- 设置交互页 ----

export type SettingsForm = {
  base_url: string;
  key_masked: string;
  model_extract: string;
  model_reasoning: string;
  model_vision: string;
  budget: string; // 日 token 预算(0=关闭)
  model_options: string[];
  model_note?: string;
};
