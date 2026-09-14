"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { apiGet, apiPost, ENDPOINTS } from "@/lib/api";
import { isActive } from "@/hooks/useResearchRun";
import type {
  ResearchRun,
  ResearchRunList,
  ResearchStatus,
  ResearchTrigger,
  ResearchTriggerBody,
} from "@/lib/types";

const PROFILE = "prebuy";

type State = {
  runId: string | null;
  status: ResearchStatus | null;
  report: ResearchRun | null;
  error: string | null;
  busy: boolean;
  /** 是否已有一次速评记录（决定首次渲染空态还是卡片） */
  hasExisting: boolean;
};

/**
 * 买入前速评 hook（CLAUDE.md 规则 6：>50 行逻辑抽 hook）。
 *
 * 与 useResearchRun 的差别：
 * - profile 固定 prebuy、quick=true（合成可选 → 无 AI 也出确定性结论）；
 * - 挂载时先查该股最近的 prebuy done 报告（列表 → 取全量），有则直接渲染；
 * - 「快速分析」原地重跑，不跳页。
 */
export function usePrebuy(code: string | null, intervalMs = 2000) {
  const [state, setState] = useState<State>({
    runId: null,
    status: null,
    report: null,
    error: null,
    busy: false,
    hasExisting: false,
  });
  const timer = useRef<number | null>(null);
  const alive = useRef(true);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      if (timer.current !== null) window.clearTimeout(timer.current);
    };
  }, []);

  const poll = useCallback(
    async (id: string) => {
      try {
        const r = await apiGet<ResearchRun>(ENDPOINTS.researchRun.replace(":runId", id));
        if (!alive.current) return;
        setState((s) => ({
          ...s,
          status: r.status,
          report: r.status === "done" ? r : null,
          error: r.status === "failed" ? r.error || "速评失败" : null,
        }));
        if (isActive(r.status)) {
          timer.current = window.setTimeout(() => poll(id), intervalMs);
        }
      } catch (e) {
        if (!alive.current) return;
        setState((s) => ({ ...s, error: e instanceof Error ? e.message : String(e) }));
      }
    },
    [intervalMs]
  );

  // 挂载 / 换股：查最近一条 prebuy 报告（按 as_of DESC），done 才拉全量。
  useEffect(() => {
    if (!code) return;
    let ok = true;
    setState((s) => ({ ...s, error: null, report: null, hasExisting: false }));
    apiGet<ResearchRunList>(ENDPOINTS.researchRuns, { code, limit: "20" })
      .then((list) => {
        if (!ok) return;
        const hit = list.runs.find((r) => r.profile === PROFILE && r.status === "done");
        if (!hit) return;
        setState((s) => ({ ...s, hasExisting: true, runId: hit.run_id, status: "done" }));
        return apiGet<ResearchRun>(ENDPOINTS.researchRun.replace(":runId", hit.run_id)).then(
          (full) => {
            if (ok) setState((s) => ({ ...s, report: full }));
          }
        );
      })
      .catch(() => {
        /* 列表不可达：不阻塞「快速分析」按钮，静默降级为空态 */
      });
    return () => {
      ok = false;
    };
  }, [code]);

  /** 触发一次现场速评：POST(quick) → 拿 run_id → 轮询。 */
  const trigger = useCallback(async () => {
    if (!code) return;
    if (timer.current !== null) window.clearTimeout(timer.current);
    setState((s) => ({ ...s, runId: null, status: "pending", report: null, error: null, busy: true }));
    try {
      const body: ResearchTriggerBody = { code, profile: PROFILE, quick: true };
      const res = await apiPost<ResearchTrigger>(ENDPOINTS.researchRuns, body);
      if (!alive.current) return;
      setState((s) => ({ ...s, runId: res.run_id, status: res.status, busy: false }));
      poll(res.run_id);
    } catch (e) {
      if (!alive.current) return;
      setState((s) => ({
        ...s,
        busy: false,
        status: "failed",
        error: e instanceof Error ? e.message : String(e),
      }));
    }
  }, [code, poll]);

  return { ...state, trigger, active: isActive(state.status ?? undefined) };
}
