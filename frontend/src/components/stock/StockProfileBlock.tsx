"use client";

import { useNavigate } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import DeepResearchButton from "@/components/research/DeepResearchButton";
import CollapsedRuns from "@/components/research/CollapsedRuns";
import { RunStatusBadge, LintBadge } from "@/components/research/StatusBadges";
import type { ResearchRunGroup } from "@/lib/research";

const isDoneTip = "尚无完成的报告 —— 点上面「发起」跑一次";

/**
 * 一个 profile 块：标题 + 说明 + 独立触发按钮 + 最新一份 + 可折叠历史。
 * P9-4 / issue #11：个股页的「深研」区按功能拆成「公司质地」与「个股分析」两块，
 * 各自独立触发 —— 这是功能拆分在界面上的落点，不是一个下拉选择器。
 */
export default function StockProfileBlock({
  code,
  group,
  title,
  hint,
  profile,
  open,
}: {
  code: string;
  group: ResearchRunGroup | undefined;
  title: string;
  hint: string;
  profile: string;
  open: boolean;
}) {
  const navigate = useNavigate();
  const go = (runId: string) => navigate(`/research/${runId}`);

  return (
    <div className="flex flex-col divide-y divide-line">
      <div className="flex items-baseline gap-2 px-1 pb-1.5">
        <span className="text-[13px] font-semibold">{title}</span>
        <span className="text-[11.5px] text-faint">{hint}</span>
        <span className="ml-auto">
          <DeepResearchButton code={code} profile={profile} label="发起" />
        </span>
      </div>

      {group?.latest ? (
        <div
          role="button"
          tabIndex={0}
          className="flex cursor-pointer items-center gap-3 px-1 py-2.5 hover:bg-hover"
          onClick={() => go(group.latest!.run_id)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              go(group.latest!.run_id);
            }
          }}
        >
          <span className="num w-24 text-[12.5px] text-muted">{group.latest.as_of}</span>
          <span className="flex items-center gap-1.5">
            <RunStatusBadge status={group.latest.status} />
            <LintBadge s={group.latest} />
          </span>
          <span className="ml-auto inline-flex items-center gap-1 text-[12px] text-accent">
            查看 <ArrowRight size={12} />
          </span>
        </div>
      ) : (
        <div className="px-1 py-2.5 text-[12.5px] text-faint">{isDoneTip}</div>
      )}

      {group && open && (
        <CollapsedRuns runs={group.history} kind="history" onOpen={go} />
      )}
      {/* 无 done 时把失败/进行中直接铺开（否则该块会显得「什么都没有」） */}
      {group && (open || !group.latest) && (
        <CollapsedRuns runs={group.unfinished} kind="unfinished" onOpen={go} forceOpen={!group.latest} />
      )}
    </div>
  );
}
