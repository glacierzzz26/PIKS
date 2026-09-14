"use client";

import { Loader2, ArrowRight } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { isActive, STATUS_LABEL } from "@/hooks/useResearchRun";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { RESEARCH_PROFILE_LABEL } from "@/lib/constants";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import Pagination from "@/components/ui/Pagination";
import type { ResearchRunSummary } from "@/lib/types";

/** 状态徽标（无骨架屏：进行中用文字徽标） */
function StatusBadge({ s }: { s: ResearchRunSummary }) {
  if (s.status === "done") return <span className="st st-dim">已完成</span>;
  if (s.status === "failed") return <span className="st st-up">失败</span>;
  if (isActive(s.status))
    return (
      <span className="st st-amber inline-flex items-center gap-1">
        <Loader2 size={11} className="animate-spin" />
        {STATUS_LABEL[s.status]}
      </span>
    );
  return <span className="st st-dim">{STATUS_LABEL[s.status]}</span>;
}

/** 机检徽标：仅 done 有意义 */
function LintBadge({ s }: { s: ResearchRunSummary }) {
  if (s.status !== "done") return <span className="text-faint">—</span>;
  return s.lint_ok && s.gate_ok ? (
    <span className="st st-accent">通过</span>
  ) : (
    <span className="st st-up">未过</span>
  );
}

/**
 * 历史报告表（独立分析师页）。列出全部 run（不限股票），点击进入报告页。
 * 三态 + 客户端分页（page/size 入 URL，规则 7）。
 */
export default function RunHistoryTable({
  runs,
  loading,
  error,
}: {
  runs: ResearchRunSummary[];
  loading: boolean;
  error: string | null;
}) {
  const navigate = useNavigate();
  const { page, size, setPage, setSize, paginate } = usePagedQuery();

  if (loading) {
    return (
      <div className="panel">
        <LoadingBlock rows={6} />
      </div>
    );
  }
  if (error) {
    return (
      <div className="panel">
        <ErrorState msg={error} />
      </div>
    );
  }
  if (runs.length === 0) {
    return (
      <div className="panel">
        <EmptyState tip="还没有深研报告 —— 在上方输入代码开始第一次分析" />
      </div>
    );
  }

  const paged = paginate(runs);
  return (
    <>
      <div className="panel overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th className="text-left">标的</th>
              <th className="text-left">类型</th>
              <th className="text-left">数据截止</th>
              <th className="text-left">状态</th>
              <th className="text-left">机检</th>
              <th className="text-right" >报告</th>
            </tr>
          </thead>
          <tbody>
            {paged.map((r) => (
              <tr
                key={r.run_id}
                role="button"
                tabIndex={0}
                className="cursor-pointer"
                onClick={() => navigate(`/research/${r.run_id}`)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    navigate(`/research/${r.run_id}`);
                  }
                }}
              >
                <td>
                  <span className="chip">{r.code}</span>
                  <span className="ml-2 font-semibold">{r.symbol}</span>
                </td>
                <td className="text-muted">
                  {RESEARCH_PROFILE_LABEL[r.profile] ?? r.profile}
                </td>
                <td className="num text-muted">{r.as_of}</td>
                <td>
                  <StatusBadge s={r} />
                </td>
                <td>
                  <LintBadge s={r} />
                </td>
                <td className="text-right" >
                  <span className="inline-flex items-center gap-1 text-[12px] text-accent">
                    查看 <ArrowRight size={12} />
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Pagination
        page={page}
        pageSize={size}
        total={runs.length}
        onPage={setPage}
        onPageSize={setSize}
      />
    </>
  );
}
