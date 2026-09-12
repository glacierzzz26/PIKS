"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { ReconRow } from "@/lib/types";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";

const STATUS: Record<string, { cls: string; label: string }> = {
  ok: { cls: "st-down", label: "正常" },
  warn: { cls: "st-amber", label: "警告" },
  failed: { cls: "st-up", label: "失败" },
};

/** 对账：每日核对快讯 → 事件链路完整性 · 异常不掩盖 */
export default function Page() {
  const recon = useData<ReconRow[]>({ path: ENDPOINTS.recon });
  const rows = recon.data ?? [];
  const anomalies = rows.filter((r) => r.anomalies > 0).length;

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>对账</h1>
          <div className="psub">
            reconcile · 每日核对快讯 → 事件链路完整性 · 异常不掩盖
          </div>
        </div>
        <div className="meta">
          <span className="st st-accent">近 {rows.length} 日</span>
          {anomalies > 0 && <span className="st st-amber">异常 {anomalies} 天</span>}
        </div>
      </div>

      <div className="panel">
        {recon.loading ? (
          <LoadingBlock rows={6} />
        ) : recon.error ? (
          <ErrorState msg={recon.error} />
        ) : rows.length === 0 ? (
          <EmptyState tip="暂无对账记录" />
        ) : (
          <>
            <table className="table">
              <thead>
                <tr>
                  <th style={{ textAlign: "left" }}>日期</th>
                  <th>快讯数</th>
                  <th>生成事件</th>
                  <th>异常</th>
                  <th>状态</th>
                  <th style={{ textAlign: "left" }}>备注</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.date}>
                    <td style={{ textAlign: "left" }} className="num-t">
                      {r.date}
                    </td>
                    <td className="num-t">{r.flashes.toLocaleString("zh-CN")}</td>
                    <td className="num-t">{r.events}</td>
                    <td
                      className="num-t"
                      style={{ color: r.anomalies ? "var(--warn)" : undefined }}
                    >
                      {r.anomalies}
                    </td>
                    <td>
                      <span className={`st ${STATUS[r.status]?.cls ?? "st-dim"}`}>
                        {STATUS[r.status]?.label ?? r.status}
                      </span>
                    </td>
                    <td
                      style={{ textAlign: "left", color: "var(--ink-faint)" }}
                      className="text-[12.5px]"
                    >
                      {r.note ?? "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="rv-foot">
              <span className="chip">数据诚实</span>
              <span>缺失如实标空态，宁缺毋假 · 异常当日记录不阻断，次日重试</span>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
