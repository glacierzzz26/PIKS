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

export const ENTITY_TYPE_LABEL: Record<string, string> = {
  company: "公司",
  industry: "行业",
  concept: "概念",
  person: "人物",
  region: "地区",
};

export const EVENT_TYPE_LABEL: Record<string, string> = {
  policy: "政策",
  earnings: "财报业绩",
  product_launch: "产品发布",
  supply_agreement: "供需协议",
  industry_event: "行业动态",
  investment: "投资动向",
  sales_data: "销售数据",
  rumor: "传闻待证",
};

export const DOC_TYPE_LABEL: Record<string, string> = {
  "daily-review": "每日复盘",
  note: "笔记",
  weekly: "周报",
  mistake: "错题",
};
