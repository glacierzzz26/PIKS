"use client";

import { Fragment } from "react";
import { ArrowRight } from "lucide-react";
import { RESEARCH_PROFILE_LABEL } from "@/lib/constants";
import { RunStatusBadge, LintBadge } from "@/components/research/StatusBadges";
import CollapsedRuns from "@/components/research/CollapsedRuns";
import type { ResearchRunGroup } from "@/lib/research";

/**
 * 历史报告表的一「组」行（issue #7）：该股最新一份作主行，历史/失败折叠在下一行。
 * 抽成子组件以满足「组件单文件 ≤150 行」（规范第 5 条）。
 */
export default function RunGroupRow({
  group,
  open,
  onOpen,
}: {
  group: ResearchRunGroup;
  open: boolean;
  onOpen: (runId: string) => void;
}) {
  // 该股无 done 时退化显最近的未完成/失败记录，不让该股整行消失
  const latest = group.latest ?? group.unfinished[0];
  if (!latest) return null;
  // 折叠区要排掉已作为主行显示的那条，否则无 done 的票会把同一条渲染两遍
  const restUnfinished = group.unfinished.filter((r) => r.run_id !== latest.run_id);

  return (
    <Fragment>
      <tr
        role="button"
        tabIndex={0}
        className="cursor-pointer"
        onClick={() => onOpen(latest.run_id)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onOpen(latest.run_id);
          }
        }}
      >
        <td>
          {/* 有公司名 → 「名称(代码)」;未建实体则如实只显代码,不臆测名称 */}
          {group.label !== group.code ? (
            <>
              <span className="font-semibold">{group.label}</span>
              <span className="ml-2 chip">{group.code}</span>
            </>
          ) : (
            <span className="chip">{group.code}</span>
          )}
          {group.total > 1 && (
            <span className="ml-2 text-[11.5px] text-faint">共 {group.total} 份</span>
          )}
        </td>
        <td className="text-muted">{RESEARCH_PROFILE_LABEL[latest.profile] ?? latest.profile}</td>
        <td className="num text-muted">{latest.as_of}</td>
        <td>
          <RunStatusBadge status={latest.status} />
        </td>
        <td>
          <LintBadge s={latest} />
        </td>
        <td className="text-right">
          <span className="inline-flex items-center gap-1 text-[12px] text-accent">
            查看 <ArrowRight size={12} />
          </span>
        </td>
      </tr>
      {(open || !group.latest) && (group.history.length > 0 || restUnfinished.length > 0) && (
        <tr>
          <td colSpan={6} className="p-0">
            <CollapsedRuns runs={group.history} kind="history" onOpen={onOpen} />
            <CollapsedRuns
              runs={restUnfinished}
              kind="unfinished"
              onOpen={onOpen}
              forceOpen={!group.latest}
            />
          </td>
        </tr>
      )}
    </Fragment>
  );
}
