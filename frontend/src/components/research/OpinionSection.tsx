"use client";

import { Sparkles } from "lucide-react";
import type { ResearchSynthesis } from "@/lib/types";

const SLOTS: { key: keyof ResearchSynthesis; label: string }[] = [
  { key: "summary", label: "执行摘要" },
  { key: "trend", label: "趋势解读" },
  { key: "conclusion", label: "综合结论" },
];

/** Opinion 区：AI 综合研判（三槽位）—— 显著标注「非事实」+ 模型名。 */
export default function OpinionSection({
  synthesis,
  model,
  tokens,
  heading = "二、AI 研判（仅供参考）",
}: {
  synthesis: ResearchSynthesis;
  model: string;
  tokens: number;
  heading?: string;
}) {
  const hasAny = SLOTS.some((s) => (synthesis[s.key] ?? "").trim() !== "");
  return (
    <section className="mt-5">
      <div className="mb-2 flex items-baseline gap-2">
        <h2 className="m-0 text-[15px] font-bold tracking-wide">{heading}</h2>
        <span className="inline-flex items-center gap-1 rounded-sm bg-bg-soft px-1.5 py-0.5 text-[11px] text-muted">
          <Sparkles size={10} />
          AI 定性研判，非事实
        </span>
      </div>
      <div className="panel panel-pad border-l-2 border-l-accent">
        <div className="num mb-3 flex flex-wrap gap-x-4 text-[11px] text-faint">
          <span>模型 {model || "—"}</span>
          <span>tokens {tokens.toLocaleString("zh-CN")}</span>
        </div>
        {!hasAny ? (
          <div className="py-4 text-center text-[13px] text-faint italic">
            本轮没有 AI 研判，请看上面「事实」区的确定性结论
          </div>
        ) : (
          <div className="space-y-4">
            {SLOTS.map((s) => {
              const text = (synthesis[s.key] ?? "").trim();
              if (!text) return null;
              return (
                <div key={s.key}>
                  <div className="mb-1 text-[13px] font-semibold text-muted">
                    {s.label}
                  </div>
                  <p className="m-0 whitespace-pre-wrap text-sm leading-relaxed">
                    {text}
                  </p>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </section>
  );
}
