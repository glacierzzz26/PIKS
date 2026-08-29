"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { ReviewRow } from "@/lib/mock/trading";
import { REVIEWS } from "@/lib/mock/trading";
import { Chip } from "@/components/ui/Num";
import { EmptyState } from "@/components/ui/States";

const STATE: Record<string, { tone: "down" | "up" | "amber"; label: string }> = {
  positive: { tone: "down", label: "逻辑自洽" },
  negative: { tone: "up", label: "存在风险" },
  neutral: { tone: "amber", label: "中性" },
};

/** 持仓复盘（只读；对齐 dev /reviews）：AI 带引用诊断结果展示 */
export default function Page() {
  const reviews = useData<ReviewRow[]>({
    path: ENDPOINTS.reviews,
    fallback: () => REVIEWS,
  });
  const rows = reviews.data ?? [];

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">复盘</h1>
          <span className="text-[13px] text-muted">
            AI 持仓诊断 · 带知识库引用 · 触发诊断在 Go 端 /reviews
          </span>
        </div>
      </div>

      {rows.length === 0 ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <EmptyState tip="暂无复盘记录" />
        </div>
      ) : (
        <div className="mt-4 flex flex-col gap-2.5">
          {rows.map((r, i) => (
            <div
              key={i}
              className="rounded border border-line bg-card p-4 shadow-card"
            >
              <div className="flex items-center gap-2.5">
                <span className="num text-base font-bold">{r.date}</span>
                <span className="text-[15px] font-semibold">{r.scope}</span>
                <Chip tone={STATE[r.state].tone}>{STATE[r.state].label}</Chip>
                <Chip tone="dim">{r.refs} 条引用</Chip>
              </div>
              <p className="mb-0 mt-2.5 text-sm leading-relaxed">{r.summary}</p>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
