"use client";

import { useEffect, useMemo, useState } from "react";
import { apiGet, ENDPOINTS } from "@/lib/api";
import type { ResearchRun, ResearchRunSummary } from "@/lib/types";

/** 一条对比项：新值 / 旧值 / 差值（任一缺失即 null，如实显 —）。 */
export type DeltaRow = {
  key: string;
  label: string;
  kind: "pct" | "price" | "score" | "level";
  prev: number | null;
  cur: number | null;
  delta: number | null;
  /** 文本型（综合评分档 / 风险等级）：旧 → 新原文。 */
  prevText?: string;
  curText?: string;
};

export type DeltaView = {
  prev: ResearchRunSummary; // 较早一份
  cur: ResearchRunSummary; // 最新一份
  rows: DeltaRow[];
};

const num = (v: unknown): number | null =>
  typeof v === "number" && Number.isFinite(v) ? v : null;

function priceVal(r: ResearchRun, key: string): number | null {
  return num((r.metrics?.price as Record<string, unknown> | undefined)?.[key]);
}

function levelText(r: ResearchRun): string {
  const risk = r.metrics?.risk as Record<string, unknown> | undefined;
  const v = risk?.overall_level;
  return typeof v === "string" ? v : "";
}

/** 构造对比行（纯函数，便于回归）：缺失项 prev/cur/delta 均 null → 前端显 —。 */
export function buildDelta(prev: ResearchRun, cur: ResearchRun): DeltaRow[] {
  const pct = (key: string, label: string, kind: DeltaRow["kind"] = "pct"): DeltaRow => {
    const p = priceVal(prev, key);
    const c = priceVal(cur, key);
    return {
      key,
      label,
      kind,
      prev: p,
      cur: c,
      delta: p !== null && c !== null ? c - p : null,
    };
  };
  const sPrev = num(prev.metrics?.scorecard?.overall);
  const sCur = num(cur.metrics?.scorecard?.overall);

  return [
    {
      key: "scorecard.overall",
      label: "综合评分",
      kind: "score",
      prev: sPrev,
      cur: sCur,
      delta: sPrev !== null && sCur !== null ? sCur - sPrev : null,
      prevText: prev.metrics?.scorecard?.overall_label ?? "",
      curText: cur.metrics?.scorecard?.overall_label ?? "",
    },
    pct("end_price", "期末价", "price"),
    pct("period_return_pct", "区间涨跌"),
    pct("return_pct_20d", "近 20 日"),
    pct("max_drawdown_pct", "最大回撤"),
    {
      key: "risk.overall_level",
      label: "风险等级",
      kind: "level",
      prev: null,
      cur: null,
      delta: null,
      prevText: levelText(prev),
      curText: levelText(cur),
    },
  ];
}

/**
 * 研究变化（设计 ux-ia §3.4）：取该股最近两份 **done** 报告的详情，diff 关键指标。
 * 纯前端、按需拉取（summary 无 metrics）；<2 份 done → hasPair=false，由组件如实空态。
 */
export function useResearchDelta(runs: ResearchRunSummary[]) {
  const pair = useMemo(() => {
    const done = runs.filter((r) => r.status === "done");
    // runs 已按 as_of 倒序：done[0] = 最新，done[1] = 前一份。
    return done.length >= 2 ? ([done[0], done[1]] as const) : null;
  }, [runs]);

  const [state, setState] = useState<{
    view: DeltaView | null;
    loading: boolean;
    error: string | null;
  }>({ view: null, loading: false, error: null });

  const curId = pair?.[0]?.run_id ?? null;
  const prevId = pair?.[1]?.run_id ?? null;

  useEffect(() => {
    if (!pair || !curId || !prevId) {
      setState({ view: null, loading: false, error: null });
      return;
    }
    const [cur, prev] = pair;
    const ac = new AbortController();
    setState({ view: null, loading: true, error: null });
    Promise.all([
      apiGet<ResearchRun>(ENDPOINTS.researchRun.replace(":runId", cur.run_id), undefined, ac.signal),
      apiGet<ResearchRun>(ENDPOINTS.researchRun.replace(":runId", prev.run_id), undefined, ac.signal),
    ])
      .then(([curRun, prevRun]) =>
        setState({
          view: { prev, cur, rows: buildDelta(prevRun, curRun) },
          loading: false,
          error: null,
        })
      )
      .catch((err) => {
        if (err?.name === "AbortError") return;
        setState({
          view: null,
          loading: false,
          error: err instanceof Error ? err.message : String(err),
        });
      });
    return () => ac.abort();
  }, [pair, curId, prevId]);

  return { ...state, hasPair: pair !== null };
}
