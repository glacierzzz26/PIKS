"use client";

import { useCallback, useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiPost, ENDPOINTS } from "@/lib/api";
import type { ResearchTrigger } from "@/lib/types";

/**
 * 触发一次深研（分析师独立页入口）。
 *
 * D-10：POST 只建 pending 行并返回 run_id，编排在服务端后台跑（10~60s）。
 * 与 useResearchRun 不同：这里不在落地页轮询，只负责「提交 → 跳报告页」，
 * 报告页自带 in-progress 轮询与三态。POST 返回后立即 render 期外导航。
 */
export function useResearchTrigger() {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const trigger = useCallback(
    async (code: string, profile: string) => {
      setBusy(true);
      setError(null);
      try {
        const res = await apiPost<ResearchTrigger>(ENDPOINTS.researchRuns, {
          code,
          profile,
        });
        navigate(`/research/${res.run_id}`);
      } catch (e) {
        // 失败留在本页，如实显示 error 原文（不降级、不跳转）。
        setError(e instanceof Error ? e.message : String(e));
      } finally {
        setBusy(false);
      }
    },
    [navigate]
  );

  return { trigger, busy, error };
}
