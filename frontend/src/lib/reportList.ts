import type { ResearchRunSummary, ResearchSubjectType } from "@/lib/types";

/**
 * 研报列表的主体分组（P9-2，design report-layout.md §6：`/reports` 列表「主体为轴」）。
 *
 * 与 `lib/research.ts` 的 `groupRunsByCode` 的区别：那个是**一只票**内的历史版本折叠
 * （issue #7，`/research` 历史表用）；这里是**跨主体**的一级分类（行业/公司/宏观），
 * 每个主体再取最新一份呈现。两者用途不同，故各自独立、不互相调用。
 *
 * 设计前提（与后端一致，勿假设其它顺序）：后端按 `as_of DESC, created_at DESC` 返回，
 * 故「最新一份」= 该主体 done 的第一项（与 `lib/research.ts` 同口径）。
 */

export type SubjectGroup = {
  /** 分组键：规范主体码（公司裸 6 位 / 行业 sw+6 位） */
  key: string;
  code: string;
  /** 展示名（行业来自指标卡、公司来自实体富化；缺则空，前端退回 code） */
  displayName: string;
  type: ResearchSubjectType;
  /** 最新一份 done（无 done 时为 null） */
  latest: ResearchRunSummary | null;
  /** 未完成/失败的（该主体无 done 时用于兜底显示，不让整组消失） */
  unfinished: ResearchRunSummary[];
  total: number;
};

export type SubjectSection = {
  type: ResearchSubjectType;
  label: string;
  groups: SubjectGroup[];
  count: number;
};

/** 主体类型的展示顺序与标题（§4.2：研报类型 chip 文案同源）。 */
const TYPE_ORDER: { type: ResearchSubjectType; label: string }[] = [
  { type: "industry", label: "行业研报" },
  { type: "company", label: "公司研报" },
  { type: "macro", label: "宏观研报" },
];

const isDone = (r: ResearchRunSummary) => r.status === "done";

/** 全部主体类型一节不落（无报告也提示空），避免用户以为某类不存在。 */
export function groupRunsBySubject(runs: ResearchRunSummary[]): SubjectSection[] {
  const buckets = new Map<string, ResearchRunSummary[]>();
  for (const r of runs) {
    (buckets.get(r.code) ?? buckets.set(r.code, []).get(r.code)!).push(r);
  }

  const groups: SubjectGroup[] = [...buckets.entries()].map(([code, list]) => {
    const done = list.filter(isDone);
    return {
      key: code,
      code,
      // 名称以组内**最新**一条为准（旧记录可能未富化出名称）
      displayName: list[0]?.display_name || list[0]?.name || "",
      type: list[0]?.subject_type ?? "company",
      latest: done[0] ?? null,
      unfinished: list.filter((r) => !isDone(r)),
      total: list.length,
    };
  });

  return TYPE_ORDER.map(({ type, label }) => {
    const inType = groups.filter((g) => g.type === type);
    return {
      type,
      label,
      groups: inType,
      count: inType.reduce((n, g) => n + g.total, 0),
    };
  }).filter((s) => s.groups.length > 0);
}
