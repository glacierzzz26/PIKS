"use client";

import { useNavigate } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import { RESEARCH_PROFILE_LABEL } from "@/lib/constants";
import { groupRunsByProfile } from "@/lib/research";
import { RunStatusBadge, LintBadge } from "@/components/research/StatusBadges";
import CollapsedRuns from "@/components/research/CollapsedRuns";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import { useUrlState } from "@/hooks/useUrlState";
import type { ResearchRunSummary } from "@/lib/types";

/** 一行最新报告：日期 + 状态 + 机检 + 进入。 */
function LatestRow({ run, onOpen }: { run: ResearchRunSummary; onOpen: () => void }) {
  return (
    <div
      role="button"
      tabIndex={0}
      className="flex cursor-pointer items-center gap-3 px-1 py-2.5 hover:bg-hover"
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
    >
      <span className="num w-24 text-[12.5px] text-muted">{run.as_of}</span>
      <span className="flex items-center gap-1.5">
        <RunStatusBadge status={run.status} />
        <LintBadge s={run} />
      </span>
      <span className="ml-auto inline-flex items-center gap-1 text-[12px] text-accent">
        查看 <ArrowRight size={12} />
      </span>
    </div>
  );
}

/**
 * 个股中心「深研」区块：该股历史报告（已按 code 过滤）。
 * issue #7：同一只票会累积多份（时间序列，决策记录依赖），不该平铺。
 * 按 profile 分组，每组只显最新一份 done，历史/失败折叠可展开；展开态入 URL（规则 7）。
 *
 * ⚠️ 分组只在展示层：`StockResearchDelta` 仍收未分组的完整 runs 去算前后对比。
 */
export default function StockResearch({ runs }: { runs: ResearchRunSummary[] }) {
  const navigate = useNavigate();
  const [query, setParam] = useUrlState();
  const open = query.rh === "1"; // rh = research history 展开态

  if (runs.length === 0) {
    return <StockSectionEmpty tip="尚无深研报告 —— 点右上「深研」发起第一次分析" />;
  }

  const groups = groupRunsByProfile(runs);
  const hasCollapsed = groups.some((g) => g.history.length > 0 || g.unfinished.length > 0);
  const go = (runId: string) => navigate(`/research/${runId}`);

  return (
    <div>
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
      <div className="flex flex-col gap-3">
        {groups.map((g) => (
          <div key={g.key} className="flex flex-col divide-y divide-line">
            <div className="px-1 pb-1 text-[12px] text-faint">
              {RESEARCH_PROFILE_LABEL[g.key] ?? g.key}
              {g.total > 1 && <span className="ml-1">· 共 {g.total} 份</span>}
            </div>

            {g.latest ? (
              <LatestRow run={g.latest} onOpen={() => go(g.latest!.run_id)} />
            ) : (
              <div className="px-1 py-2 text-[12.5px] text-faint">
                尚无完成的报告 —— 点右上「深研」发起分析
              </div>
            )}

            {open && <CollapsedRuns runs={g.history} kind="history" onOpen={go} />}
            {/* 无 done 时把失败/进行中直接铺开(否则该票会显得「什么都没有」) */}
            {(open || !g.latest) && (
              <CollapsedRuns runs={g.unfinished} kind="unfinished" onOpen={go} forceOpen={!g.latest} />
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
