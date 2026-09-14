"use client";

import { Wand2 } from "lucide-react";
import MarkdownBody from "@/components/md/MarkdownBody";
import { Chip } from "@/components/ui/Num";
import type { WeeklyDetail } from "@/lib/types";

export type GenMsg = { tone: "dim" | "amber" | "up" | "down"; label: string };

/** 周报顶部「AI 综述」面板：生成/重生成 + 展示（无综述时给 data.summary_note 说明）。 */
export default function SummaryPanel({
  data,
  generating,
  genMsg,
  onGenerate,
}: {
  data: WeeklyDetail;
  generating: boolean;
  genMsg: GenMsg | null;
  onGenerate: () => void;
}) {
  return (
    <div className="panel">
      <div className="wk-head">
        <div>
          <h3>{data.week}</h3>
          <div className="rng">{data.range}</div>
        </div>
        <button onClick={onGenerate} disabled={generating} className="btn">
          <Wand2 size={13} className="mr-1 inline" />
          {generating ? "生成中…" : data.summary ? "重新生成" : "生成 AI 综述"}
        </button>
      </div>
      {data.summary && (
        <div className="ai-summary">
          <div className="ah">
            <Wand2 size={16} strokeWidth={2} />
            AI 综述 · 本周
            <span className="chip">
              {data.summary.model} · {data.summary.tokens.toLocaleString()} tokens
            </span>
          </div>
          <MarkdownBody content={data.summary.summary} />
          <p className="num mt-3 border-t border-line pt-2 text-[11px] text-faint">
            生成于 {data.summary.updated_at}
          </p>
        </div>
      )}
      {genMsg && (
        <div className="flex items-center gap-2 px-5 pb-4">
          <Chip tone={genMsg.tone}>{genMsg.label}</Chip>
        </div>
      )}
      {!data.summary && (
        <p className="px-5 py-4 text-[13px] text-faint">{data.summary_note}</p>
      )}
    </div>
  );
}
