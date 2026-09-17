import { DOMAIN_LABEL, RESEARCH_REPORT_TYPES } from "@/lib/constants";
import type {
  ResearchDomain,
  ResearchRun,
  ResearchSection,
  ResearchSubjectType,
} from "@/lib/types";

/**
 * 研报版面数据整形（P9-2，design report-layout.md §4）。
 *
 * 目录与三域标记的**数据源**是 `metrics.meta.section_manifest`（D-R8，零 schema），
 * 不是 markdown 标题 —— `MarkdownBody` 是裸 react-markdown，无 heading id、无锚点，
 * 解析标题不可靠。正文仍由后端产出的 markdown 承载，前端只做「按 `## ` 切章 + 套锚点/
 * 域标签」，不重排内容。
 *
 * 旧报告无 `section_manifest` → 降级为「解析 `## ` 标题、域留空」（§4.8，零回归）。
 */

/** 末章标题（与 research 侧章节清单的 DISCLAIMER_TITLE 同名 —— 靠标题识别免责章）。 */
export const DISCLAIMER_TITLE = "数据说明与免责声明";

/** 一「章」：标题 + 域标记 + 正文片段 + 页内锚点 id。 */
export type ReportChapter = {
  title: string;
  domains: ResearchDomain[];
  /** 该章的 markdown 片段（不含 `## ` 标题行本身 —— 标题由 React 渲染以挂锚点与域标签） */
  markdown: string;
  /** 页内锚点（`sec-1`…）：前端自生成，不依赖 markdown 插件（§4.7） */
  id: string;
};

/** 剥掉中文序号前缀（`一、执行摘要` → `执行摘要`），与 section_manifest 的标题对齐。 */
function stripOrdinal(title: string): string {
  return title.replace(/^[一二三四五六七八九十]+、\s*/, "").trim();
}

/** 按 `## ` 一级章节标题切分 markdown 正文（`###`/`####` 子标题不受影响）。 */
export function splitMarkdown(markdown: string): { title: string; body: string }[] {
  const out: { title: string; body: string[] }[] = [];
  for (const line of markdown.split("\n")) {
    const m = /^##\s+(.*)$/.exec(line);
    if (m) {
      out.push({ title: stripOrdinal(m[1]), body: [] });
      continue;
    }
    if (out.length > 0) out[out.length - 1].body.push(line);
  }
  return out
    .filter((s) => s.title !== "")
    .map((s) => ({ title: s.title, body: s.body.join("\n").trim() }));
}

/** 章节清单：优先用 metrics.meta.section_manifest；缺失则从 markdown 标题降级解析。 */
function manifestOf(run: ResearchRun, parsed: { title: string }[]): ResearchSection[] {
  const m = run.metrics?.meta?.section_manifest;
  if (Array.isArray(m) && m.length > 0) return m;
  return parsed.map((p) => ({ title: p.title, domains: [] as ResearchDomain[] }));
}

/**
 * 把一份报告拆成「正文各章 + 页脚章」。
 *
 * 免责声明章按**标题**（非位置）识别并移入页脚 `.report-foot`（§4.6）——
 * 旧报告的末章可能是 legacy 的「AI 综合研判」，按位置取会认错。
 */
export function reportChapters(run: ResearchRun): {
  chapters: ReportChapter[];
  foot: ReportChapter | null;
} {
  const parsed = splitMarkdown(run.markdown ?? "");
  const manifest = manifestOf(run, parsed);

  const chapters: ReportChapter[] = [];
  let foot: ReportChapter | null = null;
  parsed.forEach((part, i) => {
    const meta = manifest[i] ?? { title: part.title, domains: [] as ResearchDomain[] };
    // 清单标题优先（域与 TOC 同源）；清单缺失该项时用正文标题
    const title = meta.title || part.title;
    const ch: ReportChapter = {
      title,
      domains: meta.domains ?? [],
      markdown: part.body,
      id: `sec-${i + 1}`,
    };
    if (title === DISCLAIMER_TITLE) foot = ch;
    else chapters.push(ch);
  });
  // 清单里声明了正文没有的章（理论不该发生）：不臆造内容，TOC 也不列它。
  return { chapters, foot };
}

/** 目录条目 = 正文各章 + 页脚章（§4.1 示意：页脚也占一条）。 */
export function tocEntries(
  chapters: ReportChapter[],
  foot: ReportChapter | null
): ReportChapter[] {
  return foot ? [...chapters, foot] : chapters;
}

/** 报告类型 chip 文案（由后端 subject_type 驱动，前端不猜 code 形态）。 */
export function reportTypeLabel(t: ResearchSubjectType | string): string {
  return RESEARCH_REPORT_TYPES[t] ?? "研报";
}

/** 域标签文案；未知域原样回显（不编）。 */
export function domainLabel(d: ResearchDomain | string): string {
  return DOMAIN_LABEL[d] ?? String(d);
}

/** 行业指标卡的窄类型（`metrics.industry_index` 是 Record<string, unknown>）。 */
type IndustryCard = {
  ref?: { name?: string; level_label?: string; parent?: string; code?: string };
  dispersion?: { count?: number };
  declared_count?: number | null;
};

function industryCard(run: ResearchRun): IndustryCard {
  const raw = run.metrics?.industry_index;
  return (raw ?? {}) as IndustryCard;
}

/** 宏观指标卡的窄类型（`metrics.macro` 是 Record<string, unknown>）。 */
type MacroCard = {
  ref?: { name?: string; level_label?: string };
  period?: { label?: string; period_end?: string; source_lag_days?: number };
};

function macroCard(run: ResearchRun): MacroCard {
  const raw = run.metrics?.macro;
  return (raw ?? {}) as MacroCard;
}

/**
 * 封面头副信息行（§4.2 行 2）：层级 · 成分数 · 上级 · 数据截至。
 * 只写指标卡里**确实有**的字段 —— 缺则整段省略，不填占位。
 */
export function coverSub(run: ResearchRun): string[] {
  const parts: string[] = [];
  if (run.subject_type === "company" && run.profile) parts.push(run.profile);

  const card = industryCard(run);
  if (card.ref?.level_label) parts.push(card.ref.level_label);
  if (card.ref?.parent) parts.push(`上级 ${card.ref.parent}`);
  const n = card.dispersion?.count ?? card.declared_count;
  if (typeof n === "number" && n > 0) parts.push(`成分 ${n} 只`);

  // 宏观（P9-5 / #13）：**数据截止 ≠ 报告生成日**，二者必须分开写（D-M3）。
  // 泛用的「数据截至 {as_of}」对宏观是**错的** —— as_of 是报告生成日，而宏观
  // 统计期总落后于它（实测 CPI 滞后 17 天、GDP 滞后 79 天）。写成一句会让读者
  // 以为「数据到今天」。故宏观走专门文案，用 period_end + 源站期号 + 滞后天数。
  const macro = macroCard(run);
  if (run.subject_type === "macro" || macro.period) {
    if (macro.ref?.level_label) parts.push(macro.ref.level_label);
    if (macro.period?.label) parts.push(`统计期 ${macro.period.label}`);
    if (macro.period?.period_end) {
      const lag = macro.period.source_lag_days;
      parts.push(
        typeof lag === "number"
          ? `数据截止 ${macro.period.period_end}（较报告生成日滞后 ${lag} 天）`
          : `数据截止 ${macro.period.period_end}`
      );
    }
    if (run.as_of) parts.push(`报告生成 ${run.as_of}`);
    return parts;
  }

  if (run.as_of) parts.push(`数据截至 ${run.as_of}`);
  return parts;
}

/** 可溯源计数（§4.2 行 3）：Number Lint 的 matched/scanned，正面呈现。 */
export function traceability(lint: ResearchRun["lint"]): string | null {
  const scanned = lint?.scanned;
  if (typeof scanned !== "number" || scanned === 0) return null;
  return `数字 ${lint.matched ?? 0}/${scanned} 可溯源`;
}

/** 结论 chip 文案：评分卡优先（个股），否则退到风险等级（行业无评分卡）。 */
export function conclusionChip(run: ResearchRun): string | null {
  const label = run.metrics?.scorecard?.overall_label;
  if (label) return label;
  const level = (run.metrics?.risk as { overall_level?: string } | undefined)?.overall_level;
  if (!level) return null;
  return `风险 ${({ low: "低", medium: "中", high: "高" } as Record<string, string>)[level] ?? level}`;
}

/** 后端若未替换槽位（合成未跑/未过机检），正文里会残留字面量 —— 不给读者看这个。 */
export function stripAiSlot(markdown: string): string {
  return markdown.replace(/\{\{?\s*ai_synthesis\s*\}\}?/g, "").trim();
}
