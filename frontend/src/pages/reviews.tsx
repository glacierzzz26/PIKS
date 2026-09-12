"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { ReviewRow } from "@/lib/types";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";

const STATE: Record<string, { cls: string; label: string }> = {
  positive: { cls: "st-down", label: "偏多" },
  negative: { cls: "st-up", label: "偏空" },
  neutral: { cls: "st-dim", label: "中性" },
};

/** 复盘（只读）：AI 带引用诊断结果展示（诊断触发在交易页「组合诊断」） */
export default function Page() {
  const reviews = useData<ReviewRow[]>({ path: ENDPOINTS.reviews });
  const rows = reviews.data ?? [];

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>复盘</h1>
          <div className="psub">
            AI 持仓诊断 · 带知识库引用 · 诊断在交易页触发
          </div>
        </div>
        <div className="meta">
          <span className="st st-accent">共 {rows.length} 期</span>
        </div>
      </div>

      <div className="panel">
        {reviews.loading ? (
          <LoadingBlock rows={6} />
        ) : reviews.error ? (
          <ErrorState msg={reviews.error} />
        ) : rows.length === 0 ? (
          <EmptyState tip="暂无复盘记录" />
        ) : (
          rows.map((r, i) => (
            <div key={i} className="rvrow">
              <div className="rd">
                {r.date}
                <span className="scope">{r.scope} · 引用 {r.refs} 条</span>
              </div>
              <div className="rc">
                {r.summary}
                {r.risks && r.risks.length > 0 && (
                  <div className="mt-1.5 flex flex-col gap-1">
                    {r.risks.map((rk, j) => (
                      <span key={j} className="risk">
                        风险：{rk.title}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <div className="rs">
                <span className={`st ${STATE[r.state]?.cls ?? "st-dim"}`}>
                  {STATE[r.state]?.label ?? r.state}
                </span>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
