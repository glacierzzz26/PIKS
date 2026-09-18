"use client";

import { useState } from "react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { ReviewRow } from "@/lib/types";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import ReviewPointList from "@/components/reviews/ReviewPointList";
import AccountCard from "@/components/trades/AccountCard";
import { ShieldAlert, Repeat } from "lucide-react";

const STATE: Record<string, { cls: string; label: string }> = {
  positive: { cls: "st-down", label: "偏多" },
  negative: { cls: "st-up", label: "偏空" },
  neutral: { cls: "st-dim", label: "中性" },
};

/** 复盘（只读）：AI 带引用诊断结果展示（诊断触发在交易页「组合诊断」）。
 *  每条风险/复盘点可一键存为个人笔记 —— 后端补 references 边，笔记回流个股页与笔记库（P6-5）。 */
export default function Page() {
  const reviews = useData<ReviewRow[]>({ path: ENDPOINTS.reviews });
  const [msg, setMsg] = useState<string | null>(null);
  const rows = reviews.data ?? [];

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>持仓诊断</h1>
          <div className="psub">
            给当前持仓逐条体检 · 列出风险点与复盘点 · 结论可存成笔记
          </div>
        </div>
        <div className="meta">
          <span className="st st-accent">共 {rows.length} 期</span>
        </div>
      </div>

      {msg && <p className="mb-3 text-xs text-faint">{msg}</p>}

      {/* 账户资金卡（issue #19）：复盘时第一眼「账户整体今天如何」 */}
      <AccountCard className="mb-3.5" />

      <div className="panel">
        {reviews.loading ? (
          <LoadingBlock rows={6} />
        ) : reviews.error ? (
          <ErrorState msg={reviews.error} />
        ) : rows.length === 0 ? (
          <EmptyState
            tip="暂无诊断记录 · 到「交易与持仓」上传截图后点「组合诊断」"
            action={{ to: "/trades", label: "去生成诊断" }}
          />
        ) : (
          rows.map((r, i) => (
            <div key={i} className="rvrow">
              <div className="rd">
                {r.date}
                <span className="scope">{r.scope} · 引用 {r.refs} 条</span>
              </div>
              <div className="rc">
                {r.summary}
                <div className="mt-2 flex flex-col gap-3">
                  <ReviewPointList
                    title="风险点"
                    icon={<ShieldAlert size={12} />}
                    points={r.risks ?? []}
                    pathFor={(j) =>
                      `/trades/positions/save-risk/${j}?snapshot=${r.date}`
                    }
                    onSaved={setMsg}
                  />
                  <ReviewPointList
                    title="复盘点"
                    icon={<Repeat size={12} />}
                    points={r.mistakes ?? []}
                    pathFor={(j) =>
                      `/trades/positions/save-risk/${j}?snapshot=${r.date}`
                    }
                    onSaved={setMsg}
                  />
                </div>
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
