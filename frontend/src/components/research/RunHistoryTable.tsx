"use client";

import { useNavigate } from "react-router-dom";
import { useUrlState } from "@/hooks/useUrlState";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { groupRunsByCode } from "@/lib/research";
import RunGroupRow from "@/components/research/RunGroupRow";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import Pagination from "@/components/ui/Pagination";
import type { ResearchRunSummary } from "@/lib/types";

/**
 * 历史报告表（独立分析师页）。列出全部 run（不限股票），点击进入报告页。
 * issue #7：跨股票列表会因同股多份而平铺重复 —— 按 code 分组，每行显该股最新一份，
 * 历史/失败折叠可展开；展开态入 URL（规则 7）。分页对**组**进行（不是对行）。
 * 三态 + 客户端分页（page/size 入 URL）。
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
  const [query, setParam] = useUrlState();
  const { page, size, setPage, setSize, paginate } = usePagedQuery();
  const open = query.rh === "1";

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

  const groups = groupRunsByCode(runs);
  const paged = paginate(groups);
  const hasCollapsed = groups.some((g) => g.history.length > 0 || g.unfinished.length > 0);
  const go = (runId: string) => navigate(`/research/${runId}`);

  return (
    <>
      {hasCollapsed && (
        <div className="mb-1 flex items-center justify-end">
          <button
            onClick={() => setParam("rh", open ? "" : "1")}
            className="inline-flex items-center gap-1 text-[12px] text-muted hover:text-accent"
          >
            {open ? "收起历史版本" : "展开历史版本"}
          </button>
        </div>
      )}
      <div className="panel overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th className="text-left">标的</th>
              <th className="text-left">类型</th>
              <th className="text-left">数据截止</th>
              <th className="text-left">状态</th>
              <th className="text-left">机检</th>
              <th className="text-right">报告</th>
            </tr>
          </thead>
          <tbody>
            {paged.map((g) => (
              <RunGroupRow key={g.key} group={g} open={open} onOpen={go} />
            ))}
          </tbody>
        </table>
      </div>
      <Pagination
        page={page}
        pageSize={size}
        total={groups.length}
        onPage={setPage}
        onPageSize={setSize}
      />
    </>
  );
}
