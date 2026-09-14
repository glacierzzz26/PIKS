"use client";

import { useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, Microscope, FileText } from "lucide-react";
import { useResearchRun, STATUS_LABEL } from "@/hooks/useResearchRun";
import { apiGet, ENDPOINTS } from "@/lib/api";
import type { ResearchRunList } from "@/lib/types";
import { useState } from "react";

/**
 * 深研按钮（实体卡 / 持仓行共用）：
 *   无报告 → 「深研」触发；触发后原地显示状态徽标（文字，非骨架屏）。
 *   有报告 → 「查看报告」直达 /research/:runId。
 * 完成后自动跳报告页。
 */
export default function DeepResearchButton({
  code,
  profile = "complete-stock",
}: {
  code: string;
  profile?: string;
}) {
  const navigate = useNavigate();
  const { runId, status, active, error, trigger } = useResearchRun({ code, profile });
  // 该股是否已有完成的报告（决定首屏渲染「深研」还是「查看报告」）
  const [latest, setLatest] = useState<string | null>(null);

  useEffect(() => {
    let ok = true;
    apiGet<ResearchRunList>(ENDPOINTS.researchRuns, { code, limit: "1" })
      .then((r) => {
        if (ok && r.runs[0]?.status === "done") setLatest(r.runs[0].run_id);
      })
      .catch(() => {
        /* 列表不可达：不阻塞「深研」触发，静默降级为无报告 */
      });
    return () => {
      ok = false;
    };
  }, [code]);

  // 触发完成后跳转（状态从 active → done）
  useEffect(() => {
    if (runId && status === "done") navigate(`/research/${runId}`);
  }, [runId, status, navigate]);

  if (active) {
    return (
      <span className="st st-amber inline-flex items-center gap-1 text-[11px]">
        <Loader2 size={11} className="animate-spin" />
        {status ? STATUS_LABEL[status] : "提交中"}
      </span>
    );
  }

  return (
    <span className="inline-flex items-center gap-1.5">
      <button
        onClick={trigger}
        title={error ?? undefined}
        className="inline-flex h-7 items-center gap-1 rounded-sm border border-line bg-card px-2 text-[11px] text-muted hover:border-accent hover:text-accent"
      >
        <Microscope size={11} />
        深研
      </button>
      {latest && (
        <button
          onClick={() => navigate(`/research/${latest}`)}
          className="inline-flex h-7 items-center gap-1 rounded-sm border border-line bg-card px-2 text-[11px] text-muted hover:border-accent hover:text-accent"
        >
          <FileText size={11} />
          查看报告
        </button>
      )}
    </span>
  );
}
