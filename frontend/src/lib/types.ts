/** 领域类型 —— 与 internal/model 及 migrations 保持一致（只读投影） */

export type EventItem = {
  id: string;
  title: string;
  event_type: string;
  summary: string;
  facts: string[];
  affected: { word: string; entity_id?: string; entity_name?: string; code?: string }[];
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
  /** 6 位股票代码（仅公司实体带 detail.code 时）；缺省 = 不提供深研入口 */
  code?: string;
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

/** 自选镜像预览行：change=add 将加入 / remove 将移出 / keep 已在（不变）。 */
export type PreviewWatch = {
  include: boolean;
  change: "add" | "remove" | "keep";
  code: string;
  name: string;
};

export type ImportPreview = {
  kind: string;
  attachment_id: string;
  trades: PreviewTrade[];
  positions: PreviewPosition[];
  watch: PreviewWatch[];
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
  based_on?: DecisionRef[]; // 决策关联：买入时在看什么（P6-4）
};

/** 决策关联目标（P6-4）：kind 决定图标与跳转。 */
export type DecisionRef = {
  kind: "research" | "event" | "note";
  id: string;
  title: string;
  date?: string; // research as_of / event 发生日
  url?: string; // 深链
};

/** POST /api/v1/trades 的决策关联入参（to_id 用各表 UUID）。 */
export type TradeBasedOn = {
  run_ids: string[]; // research_runs.id（UUID，绝不用 run_id）
  event_ids: string[];
  note_ids: string[];
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
  mistakes?: ReviewPoint[]; // 复盘点（诊断时识别出的操作失误）
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

// ---- 个股深研（research 并入，对齐 internal/web/api_research.go DTO）----

/** 编排状态机：pending → gathering → synthesizing → verifying → done / failed */
export type ResearchStatus =
  | "pending"
  | "gathering"
  | "synthesizing"
  | "verifying"
  | "done"
  | "failed";

/**
 * 研报主体类型（P9-2 版面）。公司/行业/宏观共用一套版面，差异由此驱动
 * （design report-layout.md D-R2）。与 internal/store/research_runs.go 的
 * SubjectCompany/SubjectIndustry/SubjectMacro 一致。
 */
export type ResearchSubjectType = "company" | "industry" | "macro";

/** 三域标记一条（D-R5）：域 key → 展示映射见 constants.DOMAIN_LABEL */
export type ResearchDomain = "fact" | "calc" | "opinion";

/** 章节清单一项（metrics.meta.section_manifest，D-R8）—— TOC 与域标签的数据源 */
export type ResearchSection = {
  title: string;
  domains: ResearchDomain[];
};

/** Evidence 一条：Fact 的溯源凭证（research 的 Evidence 链，逐条落库） */
export type ResearchEvidence = {
  id: string;
  type: string; // fact / …
  tier: string; // structured / …
  section: string; // price / volume / events / risk …
  statement: string;
  period?: string | null;
  source?: { provider?: string; uri?: string | null; title?: string | null };
};

/** 机检问题一条（Number Lint 或 Quality Gate） */
export type ResearchIssue = {
  kind?: string;
  detail?: string;
  [k: string]: unknown;
};

/** Number Lint 结果（数字机检：扫描/匹配/忽略/问题） */
export type ResearchLint = {
  scanned?: number;
  matched?: number;
  ignored?: number;
  passed?: boolean;
  issues?: ResearchIssue[];
};

/** Quality Gate 单项 */
export type ResearchGateCheck = {
  name: string;
  passed: boolean;
  detail: string;
};

/** 六项 Quality Gate 结果 */
export type ResearchGate = {
  passed?: boolean;
  checks?: ResearchGateCheck[];
  issues?: ResearchIssue[];
};

/** AI 定性研判三槽位（Opinion，非事实） */
export type ResearchSynthesis = {
  summary?: string;
  trend?: string;
  conclusion?: string;
};

/** 量价形态逐日一点（双轴图数据源） */
export type PatternPoint = {
  date: string;
  close: number;
  turnover: number;
  volume: number;
};

/**
 * 放量异常日逐日明细（issue #3）—— `metrics.volume.abnormal_volume_detail`。
 *
 * 数值全部由引擎算出后落库（Fact），前端只展示不重算。
 * - `threshold`：该日**自身**的判定阈值 `max(前20日均量×2, 前5日峰值)`
 * - `ratio`：`volume / threshold`，>1 即为异常（放大倍数）
 *
 * ⚠️ 窗口外（bars cap 之前）的交易日不参与扫描，故本明细与量价形态 60 日窗口一致；
 * 更早的异常日不在列 —— 如实不补。
 */
export type AbnormalVolumeDay = {
  date: string;
  volume: number;
  amount: number;
  threshold: number;
  ratio: number | null;
};

/** 量价形态标签（规则判定 = Inference，非遗事实；每条带 evidence 可核对） */
export type PatternLabel = {
  code: string;
  label: string;
  date: string;
  evidence: string;
};

/** 换手率峰值与股价高点的对齐关系（辅助事实） */
export type PatternDivergence = {
  peak_date?: string;
  peak_turnover?: number;
  high_date?: string;
  high_close?: number;
  peak_high_gap_days?: number;
  peak_high_coincide?: boolean;
  turnover_price_corr?: number | null;
  after_peak_return_pct?: number;
};

/** 量价形态指标卡（metrics.patterns）—— 换手率为 A股流通股本口径（与同花顺一致），形态是规则判定 */
export type ResearchPatterns = {
  symbol?: string;
  as_of?: string;
  window_days?: number;
  series?: PatternPoint[];
  labels?: PatternLabel[];
  divergence?: PatternDivergence;
  note?: string;
};

/** 单项风险（规则判定，metrics.risk.items[]） */
export type ResearchRiskItem = {
  category: string;
  level: string; // low / medium / high
  evidence: string;
  description: string;
};

/** 风险指标卡（metrics.risk）—— overall_level + veto_buy（一票否决）为确定性结论 */
export type ResearchRisk = {
  symbol?: string;
  as_of?: string;
  overall_level?: string;
  items?: ResearchRiskItem[];
  veto_buy?: boolean;
};

/** metrics = Fact 区（确定性计算结果，按 section 组织） */
export type ResearchMetrics = {
  /** meta.section_manifest = 章节清单（D-R8）；旧报告无此字段 → 前端走降级路径 */
  meta?: { section_manifest?: ResearchSection[]; [k: string]: unknown };
  price?: Record<string, number | null>;
  volume?: Record<string, unknown>;
  financial?: Record<string, unknown>;
  events?: Record<string, unknown>;
  capital?: Record<string, unknown>;
  risk?: ResearchRisk;
  patterns?: ResearchPatterns;
  /** 行业**本体**指标卡（P9 #12；主体=申万行业指数，非「个股所处行业」） */
  industry_index?: Record<string, unknown>;
  scorecard?: {
    overall?: number | null;
    overall_label?: string;
    dimensions?: {
      dimension: string;
      score: number | null;
      reason: string;
      unavailable: boolean;
    }[];
  };
  [k: string]: unknown;
};

/** GET /api/v1/research-runs/:runId 单份报告全量 */
export type ResearchRun = {
  run_id: string;
  code: string;
  symbol: string;
  /** 公司名（后端经 entities.detail.code 富化；空 = 未建实体） */
  name: string;
  profile: string;
  as_of: string;
  status: ResearchStatus;
  metrics: ResearchMetrics;
  synthesis: ResearchSynthesis;
  markdown: string;
  lint: ResearchLint;
  gate: ResearchGate;
  evidence: ResearchEvidence[];
  error: string;
  model: string;
  tokens: number;
  created_at: string;
  updated_at: string;
  /** 主体类型 + 展示名（P9-2 版面；后端按 code 形态判别，前端不必猜） */
  subject_type: ResearchSubjectType;
  display_name: string;
};

/** 列表项（GET /api/v1/research-runs，不含 markdown/metrics 宽字段） */
export type ResearchRunSummary = {
  id: string; // research_runs.id（UUID，决策关联用）
  run_id: string; // 业务键（深链 /research/:runId、轮询用）
  code: string;
  symbol: string;
  /** 公司名（后端经 entities.detail.code 富化；空 = 未建实体，如实不臆测） */
  name: string;
  profile: string;
  as_of: string;
  status: ResearchStatus;
  lint_ok: boolean;
  gate_ok: boolean;
  error: string;
  model: string;
  tokens: number;
  subject_type: ResearchSubjectType;
  display_name: string;
};

export type ResearchRunList = { runs: ResearchRunSummary[] };

/** POST /api/v1/research-runs 触发响应（reused = 复用了已在跑的同 code+profile，未新建行） */
export type ResearchTrigger = { run_id: string; status: ResearchStatus; reused?: boolean };

/** POST /api/v1/research-runs 请求体（quick = 快速模式：合成可选，无 AI 也出确定性结论） */
export type ResearchTriggerBody = {
  code: string;
  profile?: string;
  days?: number;
  quick?: boolean;
};

// ---- 个股中心（GET /api/v1/stock/:code，设计 frontend-ia §2.4）----

/** 个股中心页头实体（可为 null：未建公司实体时，持仓/深研/涨停照常，事件/笔记/行业空） */
export type StockEntity = {
  id: string;
  name: string;
  status: string;
  description: string;
};

export type StockIndustry = { id: string; name: string };

export type StockEvent = {
  id: string;
  title: string;
  event_type: string;
  occurred_at: string;
  confidence: number;
  source: string;
};

export type StockNote = {
  id: string;
  type: string;
  title: string;
  status: string;
  updated_at: string;
};

/** GET /api/v1/stock/:code 聚合响应 —— 一站看全一只票。 */
export type StockHub = {
  code: string;
  symbol: string;
  entity: StockEntity | null;
  industry: StockIndustry | null;
  position: PositionRow | null;
  trades: TradeRow[];
  research: ResearchRunSummary[];
  events: StockEvent[];
  notes: StockNote[];
  limit_ups: string[];
  /** 每笔交易的决策关联（P6-4「当时在看什么」）：trade_id → 引用列表。 */
  decisions: Record<string, DecisionRef[]>;
};

// ---- 自选（GET /api/v1/watchlist，设计 frontend-ia §2.3 / phase6 ux-ia §2）----

/** 自选项最近一条 affects 事件（P6-3 富化）。 */
export type WatchEvent = {
  title: string;
  date: string; // 事件发生日 YYYY-MM-DD
  count: number; // affects 到该实体的累计事件数
  latest_id: string; // 最近事件 id（深链 /events/:id）
};

/** 自选行：自选实体 + 是否持有（持仓快照命中时带现价/盈亏）+ 富化徽标。 */
export type WatchItem = {
  code: string;
  entity_id: string;
  name: string;
  description: string;
  held: boolean;
  position: PositionRow | null;
  // 富化（P6-3）
  latest_event: WatchEvent | null;
  position_date: string; // 持仓快照日（空 = 无持仓）
  has_research: boolean;
  latest_research_asof: string; // YYYY-MM-DD（空 = 未深研）
};

export type Watchlist = {
  items: WatchItem[];
  position_date: string; // 全部持仓共用的最新快照日（空 = 无持仓）
  researched: number; // 自选中做过深研的数量（覆盖率）
};
