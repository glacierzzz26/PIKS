"use client";

import { Loader2 } from "lucide-react";
import { isActive, STATUS_LABEL } from "@/hooks/useResearchRun";
import type { ResearchRunSummary } from "@/lib/types";

/**
 * 研报状态/机检徽标（列表与个股页共用，issue #7 抽出以免两处各写一遍）。
 * 无骨架屏：进行中用文字徽标（规则：核心数字区禁 skeleton）。
 */

/** 状态徽标：进行中带转圈，终态用文字。 */
export function RunStatusBadge({ status }: { status: ResearchRunSummary["status"] }) {
  if (status === "done") return <span className="st st-dim">已完成</span>;
  if (status === "failed") return <span className="st st-up">失败</span>;
  if (isActive(status))
    return (
      <span className="st st-amber inline-flex items-center gap-1">
        <Loader2 size={11} className="animate-spin" />
        {STATUS_LABEL[status]}
      </span>
    );
  return <span className="st st-dim">{STATUS_LABEL[status]}</span>;
}

/** 机检徽标：仅 done 有意义（未完成显 —）。 */
export function LintBadge({ s }: { s: ResearchRunSummary }) {
  if (s.status !== "done") return <span className="text-faint">—</span>;
  return s.lint_ok && s.gate_ok ? (
    <span className="st st-accent">通过</span>
  ) : (
    <span className="st st-up">未过</span>
  );
}
