/**
 * Fact 区渲染辅助：把 research 的确定性指标 JSON 映射成「标签 + 数值 + 语义」三元组。
 * 只做展示映射，不重算、不改写 —— 数字的唯一源是后端 metrics（Fact）。
 */

export type Kind = "pct" | "price" | "num" | "days" | "ratio";

export type Row = { key: string; label: string; kind: Kind };

/** 涨跌语义：仅对「收益率」类着色（涨红跌绿）；回撤/波动为中性（避免误读为涨）。 */
const COLORED = new Set([
  "return_pct_5d",
  "return_pct_10d",
  "return_pct_20d",
  "return_pct_60d",
  "period_return_pct",
  "max_rise_pct",
]);

export function isColored(key: string): boolean {
  return COLORED.has(key);
}

/** 股价表现（metrics.price） */
export const PRICE_ROWS: Row[] = [
  { key: "start_price", label: "期初价", kind: "price" },
  { key: "end_price", label: "期末价", kind: "price" },
  { key: "max_price", label: "区间最高", kind: "price" },
  { key: "min_price", label: "区间最低", kind: "price" },
  { key: "period_return_pct", label: "区间涨跌", kind: "pct" },
  { key: "return_pct_5d", label: "近 5 日", kind: "pct" },
  { key: "return_pct_10d", label: "近 10 日", kind: "pct" },
  { key: "return_pct_20d", label: "近 20 日", kind: "pct" },
  { key: "max_drawdown_pct", label: "最大回撤", kind: "pct" },
  { key: "volatility_annual", label: "年化波动", kind: "pct" },
  { key: "limit_up_days", label: "涨停天数", kind: "days" },
  { key: "limit_down_days", label: "跌停天数", kind: "days" },
];

/** 成交量价（metrics.volume） */
export const VOLUME_ROWS: Row[] = [
  { key: "avg_turnover_5d", label: "近 5 日换手", kind: "pct" },
  { key: "avg_turnover_20d", label: "近 20 日换手", kind: "pct" },
  { key: "max_turnover", label: "最高换手", kind: "pct" },
  { key: "min_turnover", label: "最低换手", kind: "pct" },
  { key: "avg_amount_5d", label: "近 5 日均额", kind: "num" },
  { key: "avg_amount_20d", label: "近 20 日均额", kind: "num" },
  { key: "price_volume_corr", label: "量价相关", kind: "ratio" },
  { key: "abnormal_volume_days", label: "放量异常日", kind: "days" },
];

/** 财务基本面（metrics.financial）—— 最近季度，用于买入前体检 */
export const FINANCIAL_ROWS: Row[] = [
  { key: "latest_revenue_yoy", label: "营收同比", kind: "pct" },
  { key: "latest_net_profit_yoy", label: "净利同比", kind: "pct" },
  { key: "latest_roe", label: "ROE", kind: "pct" },
  { key: "latest_gross_margin", label: "毛利率", kind: "pct" },
  { key: "latest_net_margin", label: "净利率", kind: "pct" },
  { key: "latest_debt_ratio", label: "资产负债率", kind: "pct" },
];

/** 估值（metrics.financial 同源）—— 买入前最关心的一项 */
export const VALUATION_ROWS: Row[] = [
  { key: "pe_ttm", label: "PE(TTM)", kind: "ratio" },
  { key: "pb", label: "PB", kind: "ratio" },
];

/** 取数值（null/undefined → null，保持「缺失如实空态」）。 */
export function numAt(
  obj: Record<string, unknown> | undefined,
  key: string
): number | null {
  if (!obj) return null;
  const v = obj[key];
  return typeof v === "number" && Number.isFinite(v) ? v : null;
}

/** 按 kind 格式化（等宽数字由调用方 .num 保证）。 */
export function fmt(value: number | null, kind: Kind): string {
  if (value === null) return "—";
  switch (kind) {
    case "pct":
      return `${value > 0 ? "+" : ""}${value.toFixed(2)}%`;
    case "price":
      return value.toFixed(2);
    case "days":
      return `${value}`;
    case "ratio":
      return value.toFixed(2);
    default:
      // 成交额等大数：亿/万分级，避免长串数字
      if (Math.abs(value) >= 1e8) return `${(value / 1e8).toFixed(2)} 亿`;
      if (Math.abs(value) >= 1e4) return `${(value / 1e4).toFixed(2)} 万`;
      return value.toLocaleString("zh-CN", { maximumFractionDigits: 2 });
  }
}

/** 评分卡维度中文名 */
export const DIM_LABEL: Record<string, string> = {
  business_quality: "业务质量",
  fundamental_trend: "基本面趋势",
  market_trend: "市场走势",
  valuation: "估值",
  recent_events: "近期事件",
  risk: "风险",
};
