/** UI 筛选常量（来自领域枚举，非演示数据） */

export const EVENT_TYPES = [
  { key: "", label: "全部类型" },
  { key: "policy", label: "政策" },
  { key: "earnings", label: "财报业绩" },
  { key: "product_launch", label: "产品发布" },
  { key: "supply_agreement", label: "供需协议" },
  { key: "industry_event", label: "行业动态" },
  { key: "investment", label: "投资动向" },
  { key: "sales_data", label: "销售数据" },
  { key: "rumor", label: "传闻待证" },
];

export const EVENT_STATUS = [
  { key: "", label: "全部状态" },
  { key: "confirmed", label: "已确认" },
  { key: "pending", label: "待复核" },
];

export const ENTITY_TYPES = [
  { key: "", label: "全部" },
  { key: "company", label: "公司" },
  { key: "industry", label: "行业" },
  { key: "concept", label: "概念" },
  { key: "person", label: "人物" },
];

/** 个股深研报告类型（profile）。key 与 research/profiles/*.yaml 的 name 一致 */
export const RESEARCH_PROFILES = [
  { key: "complete-stock", label: "全面深研" },
  { key: "short-term", label: "短线视角" },
];

/** profile key → 中文标签（未知 key 原样回显） */
export const RESEARCH_PROFILE_LABEL: Record<string, string> = Object.fromEntries(
  RESEARCH_PROFILES.map((p) => [p.key, p.label])
);

/**
 * 研报类型（主体）→ 中文标签（P9-2 版面，design report-layout.md §4.2）。
 * 由后端 DTO 的 subject_type 驱动，前端不猜 code 形态。
 */
export const RESEARCH_REPORT_TYPES: Record<string, string> = {
  company: "公司研报",
  industry: "行业研报",
  macro: "宏观研报",
};

/**
 * 三域标记（D-R5）→ 中文标签。颜色复用既有 .st 色板（`.domain-tag` 由 CSS 决定），
 * 前端不新增颜色。键与后端 section_manifest.domains 一致。
 */
export const DOMAIN_LABEL: Record<string, string> = {
  fact: "数据",
  calc: "计算",
  opinion: "研判",
};

/** 域 key → .st 色板类（单系统 token；D-R5 明确不新增颜色） */
export const DOMAIN_TONE: Record<string, string> = {
  fact: "st-dim",
  calc: "st-accent",
  opinion: "st-amber",
};

/** 风险等级（research metrics.risk.overall_level）→ 中文；未知 key 原样回显。 */
export const RISK_LEVEL_LABEL: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
};

export const FLASH_SOURCES = [
  { key: "", label: "全部来源" },
  { key: "东财快讯", label: "东财快讯" },
  { key: "公司公告", label: "公司公告" },
  { key: "券商研报", label: "券商研报" },
  { key: "海外市场", label: "海外市场" },
  { key: "市场传闻", label: "市场传闻" },
];
