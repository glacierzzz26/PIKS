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

/**
 * 可触发的研究类型（profile）—— 触发选择器用（P9-4 / issue #11 功能拆分）。
 * key 与 research/profiles/*.yaml 的 name 一致。
 *
 * `company` = 公司研报（季频文档，回答「这家公司什么质地」）；
 * `stock`   = 个股分析（日频速览，回答「这票现在能不能动」）。
 * 两者即 §1.1 的功能拆分，此前合体在 `complete-stock` 里。
 *
 * ⚠️ `complete-stock`（旧的 11 节合体档案）**不在选择器里** —— 功能已被上面
 * 两者取代，但它保留为**兼容档案**：生产有历史 run 用它（多份 run = 时间序列，
 * 决策记录 based_on 边指向它们），删掉会破坏历史。故它仍留在
 * RESEARCH_PROFILE_LABEL 里供历史回显，只是不再能被新触发。
 */
export const RESEARCH_PROFILES = [
  { key: "stock", label: "个股分析" },
  { key: "company", label: "公司研报" },
  { key: "short-term", label: "短线视角" },
];

/**
 * 宏观维度（P9-5 / #13）→ 中文标签。
 *
 * ⚠️ 主体码是 `macro:<key>`,key **只**能是后端已支持的维度(`cn_cpi` / `cn_ppi` /
 * `cn_m2` / `cn_gdp`,真源在 Python 侧的维度表)—— 前端不给用户自由输入 key 的机会,
 * 只列这几个按钮。新增维度须后端先支持、再同步此处(前端不猜可用取值)。
 * 键序即展示序(CPI → PPI → M2 → GDP,先月频后季频)。
 */
export const MACRO_DIMENSIONS = [
  { key: "cn_cpi", label: "CPI" },
  { key: "cn_ppi", label: "PPI" },
  { key: "cn_m2", label: "M2" },
  { key: "cn_gdp", label: "GDP" },
];

/** 宏观维度选择产出的主体码(`macro:<key>`)。 */
export const macroSubject = (key: string) => `macro:${key}`;

/**
 * profile key → 中文标签（未知 key 原样回显）。
 * 含**不在选择器里**的 profile（`complete-stock` 兼容档案、`industry` 行业研报、
 * `macro` 宏观研报），否则历史 run 会显示英文 key。行业研报主体是 sw+6 位码，
 * 宏观主体是 macro:<key>，二者都不在本页 6 位代码输入的触发范围内，故不入
 * profile 选择器但需能回显（宏观维度选择器见 AnalystTrigger）。
 */
export const RESEARCH_PROFILE_LABEL: Record<string, string> = {
  ...Object.fromEntries(RESEARCH_PROFILES.map((p) => [p.key, p.label])),
  "complete-stock": "全面深研（旧）",
  "prebuy": "买入前速评",
  "industry": "行业研报",
  "macro": "宏观研报",
};

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

/**
 * 消息页排序选项（issue #37）—— key 与后端 store.EventSort* / store.FlashSort* 一致。
 * 空 key = 默认（时间倒序、最新在前），故默认态不写 URL query，深链向后兼容。
 */
export const EVENT_SORTS = [
  { key: "", label: "按时间" },
  { key: "confidence", label: "按置信度" },
];

/** 快讯排序：「重要优先」的「重要」= 已被抽取成事件（后端近似口径，见 toFlash）。 */
export const FLASH_SORTS = [
  { key: "", label: "按时间" },
  { key: "important", label: "重要优先" },
];
