import type { ResearchRunSummary } from "@/lib/types";

/**
 * 研报列表分组（issue #7：同一只票多次研报会产生多条数据，不该平铺）。
 *
 * 设计前提（与后端一致，勿假设其它顺序）：
 * - 后端 `ListResearchRuns` / `ListResearchRunsByEntity` 均按 `as_of DESC, created_at DESC` 返回；
 * - 故「最新一份」= 过滤 done 后的第一项（与 `useResearchDelta.ts` 同口径）。
 *
 * ⚠️ 分组只影响**展示**：`StockResearchDelta` 依赖**未分组**的完整 runs 去算前后对比，
 * 调用方不要把分组结果回传给 delta（见 `pages/stock/[code].tsx`）。
 */

/** 一只票（或一个 profile）下的一组研报：最新一份 done + 折叠的历史 + 折叠的失败。 */
export type ResearchRunGroup = {
  /** 分组键（code 或 profile） */
  key: string;
  /** 展示名（公司名优先，缺则回退 code）；code 分组时用 */
  label: string;
  /** code（code 分组时 = key；profile 分组时取组内首个） */
  code: string;
  /** 最新一份 done（无 done 时为 null，前端如实显示空态/仅失败） */
  latest: ResearchRunSummary | null;
  /** 其余 done（较早的历史版本，可为空） */
  history: ResearchRunSummary[];
  /** 未完成/失败的（进行中 + 失败），一并折叠 */
  unfinished: ResearchRunSummary[];
  /** 组内总份数（含 latest） */
  total: number;
};

const isDone = (r: ResearchRunSummary) => r.status === "done";

/** 把一组 runs 折成 latest/history/unfinished（入参须已按 as_of DESC）。 */
function fold(key: string, label: string, code: string, runs: ResearchRunSummary[]): ResearchRunGroup {
  const done = runs.filter(isDone);
  return {
    key,
    label,
    code,
    latest: done[0] ?? null,
    history: done.slice(1),
    unfinished: runs.filter((r) => !isDone(r)),
    total: runs.length,
  };
}

/** 按公司名/代码分组（`/research` 历史表：全库跨股票列表）。保持首次出现顺序。 */
export function groupRunsByCode(runs: ResearchRunSummary[]): ResearchRunGroup[] {
  const buckets = new Map<string, ResearchRunSummary[]>();
  for (const r of runs) {
    (buckets.get(r.code) ?? buckets.set(r.code, []).get(r.code)!).push(r);
  }
  return [...buckets.entries()].map(([code, list]) =>
    // 名称以组内**最新**一条为准（旧记录可能未富化出名称）
    fold(code, list[0]?.name || code, code, list)
  );
}

/** 按 profile 分组（个股页：同一只票的不同报告类型各成一组）。保持首次出现顺序。 */
export function groupRunsByProfile(runs: ResearchRunSummary[]): ResearchRunGroup[] {
  const buckets = new Map<string, ResearchRunSummary[]>();
  for (const r of runs) {
    (buckets.get(r.profile) ?? buckets.set(r.profile, []).get(r.profile)!).push(r);
  }
  return [...buckets.entries()].map(([profile, list]) =>
    fold(profile, profile, list[0]?.code ?? "", list)
  );
}
