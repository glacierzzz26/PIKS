"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { RunStatusBadge } from "@/components/research/StatusBadges";
import { RESEARCH_PROFILE_LABEL } from "@/lib/constants";
import type { ResearchRunSummary } from "@/lib/types";

/**
 * 折叠行（issue #7）：把一组研报的「历史版本」与「未完成/失败」收成可展开的一行，
 * 避免同一只票的多份记录平铺。默认收起——主列表只显最新一份 done。
 *
 * 折叠 UI 沿用既有模式（WatchGroups 的 ChevronDown + 文字切换）。
 * `kind` 决定文案：history = 较早的已完成版本；unfinished = 进行中/失败的记录。
 */
export default function CollapsedRuns({
  runs,
  kind,
  onOpen,
  forceOpen = false,
}: {
  runs: ResearchRunSummary[];
  kind: "history" | "unfinished";
  onOpen: (runId: string) => void;
  /** 初始展开且不可收起（用于该票无 done、失败记录应直接可见的场景）。 */
  forceOpen?: boolean;
}) {
  const [openState, setOpen] = useState(false);
  const open = forceOpen || openState;
  if (runs.length === 0) return null;

  const failed = runs.filter((r) => r.status === "failed").length;
  const label =
    kind === "history"
      ? `历史 ${runs.length} 份`
      : `另有 ${runs.length} 次未产出${failed > 0 ? `（失败 ${failed}）` : ""}`;

  return (
    <div className="divide-y divide-line border-t border-line">
      {!forceOpen && (
        <button
          onClick={() => setOpen((v) => !v)}
          className="flex w-full items-center gap-1.5 px-1 py-2 text-left text-[12px] text-muted hover:text-accent"
        >
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
          {label}
        </button>
      )}
      {open && (
        <div className="divide-y divide-line">
          {runs.map((r) => (
            <div
              key={r.run_id}
              role="button"
              tabIndex={0}
              className="flex cursor-pointer items-center gap-3 px-1 py-2 pl-5 hover:bg-hover"
              onClick={() => onOpen(r.run_id)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  onOpen(r.run_id);
                }
              }}
            >
              <span className="st st-dim">{RESEARCH_PROFILE_LABEL[r.profile] ?? r.profile}</span>
              <span className="num w-24 text-[12.5px] text-muted">{r.as_of}</span>
              <span className="flex-1">
                <RunStatusBadge status={r.status} />
              </span>
              {r.status === "failed" && r.error && (
                <span className="max-w-[38ch] truncate text-[11.5px] text-faint" title={r.error}>
                  {r.error}
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
