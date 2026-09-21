/** 涨跌色与金额格式化（A 股习惯：涨红跌绿） */

export function pctClass(v: number): string {
  if (v > 0) return "text-up";
  if (v < 0) return "text-down";
  return "text-muted";
}

export function pct(v: number): string {
  const s = v > 0 ? "+" : "";
  return `${s}${v.toFixed(2)}%`;
}

export function fmtYi(v: number): string {
  if (v >= 10000) return `${(v / 10000).toFixed(2)} 万亿`;
  return `${v.toLocaleString("zh-CN", { maximumFractionDigits: 1 })} 亿`;
}

export function fmtWan(v: number): string {
  return `${(v / 1e4).toFixed(1)} 万`;
}

export function fmtYuan(v: number): string {
  return `¥${v.toLocaleString("zh-CN")}`;
}

/** 从 YYYY-MM-DD 到今天的天数差；无法解析返回 null（如实留空，不猜）。 */
export function daysAgo(date: string): number | null {
  if (!date) return null;
  const t = Date.parse(`${date}T00:00:00`);
  if (Number.isNaN(t)) return null;
  return Math.floor((Date.now() - t) / 86_400_000);
}

/**
 * 是否为合法 A 股 6 位数字代码（与后端 store.IsStockCode 同一规则）。
 * 用于判断 code 能否安全进深研/个股聚合 —— 名称当作代码会产脏数据（issue #2）。
 */
export function isStockCode(code: string | undefined | null): boolean {
  return !!code && /^\d{6}$/.test(code);
}

/**
 * 是否为合法**宏观主体码** `macro:<key>`（与后端 store.NormalizeSubject 的
 * 宏观分支同一形态规则：前缀 + 非空 key）。
 *
 * ⚠️ 与 `isStockCode` 刻意**互斥**：宏观档案绝不接受 6 位数字，个股档案也绝不
 * 接受 `macro:` 形态（见 AnalystTrigger 的档案分派校验）。放开非 6 位输入**不等于**
 * 让任意字符串进后端 —— 那会把 issue #2 的脏 code 从入口放回来。
 */
export function isMacroCode(code: string | undefined | null): boolean {
  return !!code && /^macro:[a-z0-9_]+$/i.test(code);
}

export const ENTITY_TYPE_LABEL: Record<string, string> = {
  company: "公司",
  industry: "行业",
  concept: "概念",
  person: "人物",
  region: "地区",
};

// ⚠️ `EVENT_TYPE_LABEL` 已删除（issue #61）：类型 label 的唯一真源在后端，
// 经 `lib/eventTypes.tsx` 的 `useEventTypes().labelOf` 取。此处不得再放本地映射。

export const DOC_TYPE_LABEL: Record<string, string> = {
  "daily-review": "每日复盘",
  note: "笔记",
  belief: "信念",
  case: "案例",
  mistake: "错题",
  weekly: "周报",
};
