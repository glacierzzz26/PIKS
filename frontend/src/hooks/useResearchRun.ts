"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { apiGet, apiPost, ENDPOINTS } from "@/lib/api";
import type { ResearchRun, ResearchStatus, ResearchTrigger } from "@/lib/types";

/** 进行中的状态（轮询期间）；终态 done/failed 停轮询。 */
const ACTIVE: ResearchStatus[] = [
  "pending",
  "gathering",
  "synthesizing",
  "verifying",
];

export function isActive(s: ResearchStatus | undefined): boolean {
  return s !== undefined && ACTIVE.includes(s);
}

export const STATUS_LABEL: Record<ResearchStatus, string> = {
  pending: "排队中",
  gathering: "采集中",
  synthesizing: "合成中",
  verifying: "机检中",
  done: "已完成",
  failed: "失败",
};

type State = {
  /** 当前跟踪的 run_id（触发后或外部传入） */
  runId: string | null;
  status: ResearchStatus | null;
  /** 已完成的报告全量（done 时拉取；失败/进行中为 null） */
  report: ResearchRun | null;
  error: string | null;
  busy: boolean;
};

type Options = {
  code: string | null;
  profile?: string;
  /** 已有报告：直接跟踪它（用于「查看报告」/持仓行回填），不触发新跑 */
  runId?: string | null;
  /** 轮询间隔（ms）。默认 2s。 */
  intervalMs?: number;
};

/**
 * 深研触发 + 状态轮询（CLAUDE.md 规则 6：>50 行逻辑抽 hook）。
 *
 * D-10：POST 只建 pending 行并返回 run_id，编排在服务端后台跑（10~60s），
 * 故前端轮询 GET 取状态。到 done 拉单份全量给报告页；failed 取 error 原文。
 * 组件卸载 / 换股票即 abort，不留悬挂 interval。
 */
export function useResearchRun(opts: Options) {
  const { code, profile = "complete-stock", runId: givenRunId, intervalMs = 2000 } = opts;
  const [state, setState] = useState<State>({
    runId: givenRunId ?? null,
    status: givenRunId ? "pending" : null,
    report: null,
    error: null,
    busy: false,
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

  // 命中 done 拉全量；failed 原样保留 error；进行中继续排下一次轮询。
  const poll = useCallback(
    async (id: string) => {
      try {
        const r = await apiGet<ResearchRun>(
          ENDPOINTS.researchRun.replace(":runId", id)
        );
        if (!alive.current) return;
        setState((s) => ({
          ...s,
          status: r.status,
          report: r.status === "done" ? r : null,
          error: r.status === "failed" ? r.error || "深研失败" : null,
        }));
        if (isActive(r.status)) {
          timer.current = window.setTimeout(() => poll(id), intervalMs);
        }
      } catch (e) {
        if (!alive.current) return;
        setState((s) => ({
          ...s,
          error: e instanceof Error ? e.message : String(e),
        }));
      }
    },
    [intervalMs]
  );

  // 外部传入 runId（查看已有报告）→ 直接跟它，不触发。
  useEffect(() => {
    if (!givenRunId) return;
    setState({ runId: givenRunId, status: "pending", report: null, error: null, busy: false });
    poll(givenRunId);
  }, [givenRunId, poll]);

  /** 触发一次新深研：POST → 拿 run_id → 开始轮询。 */
  const trigger = useCallback(async () => {
    if (!code) return;
    if (timer.current !== null) window.clearTimeout(timer.current);
    setState({ runId: null, status: "pending", report: null, error: null, busy: true });
    try {
      const res = await apiPost<ResearchTrigger>(ENDPOINTS.researchRuns, {
        code,
        profile,
      });
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
  }, [code, profile, poll]);

  return { ...state, trigger, active: isActive(state.status ?? undefined) };
}
