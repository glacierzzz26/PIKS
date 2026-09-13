"use client";

import { useNavigate } from "react-router-dom";
import { Loader2, ArrowRight } from "lucide-react";
import { isActive, STATUS_LABEL } from "@/hooks/useResearchRun";
import { RESEARCH_PROFILE_LABEL } from "@/lib/constants";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import type { ResearchRunSummary } from "@/lib/types";

/** 个股中心「深研」区块：该股历史报告（已按 code 过滤）。 */
export default function StockResearch({ runs }: { runs: ResearchRunSummary[] }) {
  const navigate = useNavigate();

  if (runs.length === 0) {
    return <StockSectionEmpty tip="尚无深研报告 —— 点右上「深研」发起第一次分析" />;
  }

  return (
    <div className="flex flex-col divide-y divide-line">
      {runs.map((r) => (
        <div
          key={r.run_id}
          role="button"
          tabIndex={0}
          className="flex cursor-pointer items-center gap-3 px-1 py-2.5 hover:bg-hover"
          onClick={() => navigate(`/research/${r.run_id}`)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              navigate(`/research/${r.run_id}`);
            }
          }}
        >
          <span className="st st-dim">
            {RESEARCH_PROFILE_LABEL[r.profile] ?? r.profile}
          </span>
          <span className="num w-24 text-[12.5px] text-muted">{r.as_of}</span>
          <span className="flex-1">
            {r.status === "done" ? (
              r.lint_ok && r.gate_ok ? (
                <span className="st st-accent">机检通过</span>
              ) : (
                <span className="st st-up">机检未过</span>
              )
            ) : r.status === "failed" ? (
              <span className="st st-up">失败</span>
            ) : isActive(r.status) ? (
              <span className="st st-amber inline-flex items-center gap-1">
                <Loader2 size={11} className="animate-spin" />
                {STATUS_LABEL[r.status]}
              </span>
            ) : (
              <span className="st st-dim">{STATUS_LABEL[r.status]}</span>
            )}
          </span>
          <span className="inline-flex items-center gap-1 text-[12px] text-accent">
            查看 <ArrowRight size={12} />
          </span>
        </div>
      ))}
    </div>
  );
}
