/** UI 筛选常量（来自领域枚举，非演示数据） */

/**
 * ⚠️ 事件类型（`EVENT_TYPES`）**已移出本文件**（issue #61）。
 * 类型 key **与**中文 label 的唯一真源是后端 `model.EventTypes`，前端经
 * `GET /api/v1/event-types` 消费，见 `lib/eventTypes.tsx` 的 `useEventTypes()`。
 * 此处**不得**再放本地类型表 —— 曾经这里的 8 值枚举与后端 9 值权威枚举漂移，
 * 导致 92% 事件露英文原值、6/8 过滤项永远为空。
 */

export const EVENT_STATUS = [
  { key: "", label: "全部状态" },
  { key: "confirmed", label: "已确认" },
  { key: "pending", label: "待复核" },
];

/**
 * 来源维度标注（issue #49 T3）——与 EVENT_STATUS（抽取态）是**两条正交的轴**，
 * 不要混用：抽取态说的是「AI 抽得准不准」，来源数说的是「有几家机构在报」。
 *
 * ⚠️ 措辞取「单一来源」而非「待核」：仓库里 pending 已渲染为「待复核」，
 * 再上一个「待核」两个近义中文标签同屏会混。且「单一来源」是**客观计数**的陈述，
 * 不暗示消息可疑 —— 独家报道是正常且常见的（issue #49 红线）。
 */
export const SINGLE_SOURCE_LABEL = "单一来源";
export const SINGLE_SOURCE_HINT =
  "目前只有这一家机构在报。独家报道很常见，不代表消息不实，只是还没有旁证。";

/**
 * 公告分级（issue #68 A 层）。key 与后端 `internal/announce` 的常量严格一致；
 * 后端经 `GET /api/v1/announcements` 的 `grade` 字段下发。
 *
 * ⚠️ 这是**机器按标题关键词判的（Inference）不是事实** —— UI 必须如实标注，
 * 不得表述成「官方认定重要」（同 P7 量价形态的标注纪律）。
 *
 * ⚠️ 折叠≠隐藏（issue #68 §3.3 红线）：噪音只是默认收起，必须留「查看全部」入口。
 * 空 grade（未分级的历史行）按「常规」显示 —— 与后端 gradeMatch 的口径一致。
 */
export const ANNOUNCE_GRADES = [
  { key: "must", label: "必读", hint: "监管动作 / 退市风险 / 重大重组 / 控制权变更" },
  { key: "important", label: "重要", hint: "股权激励 / 回购 / 增减持 / 重大合同" },
  { key: "routine", label: "常规", hint: "定期报告 / 三会决议 / 权益分派" },
  { key: "noise", label: "噪音", hint: "工商变更 / 独董述职 / 券商律所衍生文件" },
] as const;

export type AnnounceGrade = (typeof ANNOUNCE_GRADES)[number]["key"];

/** 未分级（grade 空）按常规显示，与后端 API 口径一致。 */
export const ANNOUNCE_GRADE_FALLBACK: AnnounceGrade = "routine";

/** 级别 → 中文标签；未知/空值回落「常规」。 */
export function announceGradeLabel(grade?: string): string {
  const hit = ANNOUNCE_GRADES.find((g) => g.key === grade);
  return hit ? hit.label : "常规";
}

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

/**
 * 快讯来源筛选（issue #43 T1）—— key 必须**逐字**等于 `sources.name`（机构名）。
 * 多源采集后 sources 每机构一行：东方财富/金十数据/财联社/新浪财经/同花顺/富途资讯。
 * 后端按 source 精确匹配（internal/web/api_v1.go handleAPIFlashes），写错即筛不出。
 */
export const FLASH_SOURCES = [
  { key: "", label: "全部来源" },
  { key: "东方财富", label: "东方财富" },
  { key: "金十数据", label: "金十数据" },
  { key: "财联社", label: "财联社" },
  { key: "新浪财经", label: "新浪财经" },
  { key: "同花顺", label: "同花顺" },
  { key: "富途资讯", label: "富途资讯" },
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
