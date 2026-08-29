"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { ReconRow } from "@/lib/mock/activity";
import { RECON_ROWS } from "@/lib/mock/activity";
import { Chip, Num } from "@/components/ui/Num";

const STATUS: Record<string, { tone: "down" | "amber" | "up"; label: string }> = {
  ok: { tone: "down", label: "通过" },
  warn: { tone: "amber", label: "有异常" },
  failed: { tone: "up", label: "失败" },
};

/** 对账（只读；对齐 dev /recon）：每日对账索引 + 异常明细 */
export default function Page() {
  const recon = useData<ReconRow[]>({
    path: ENDPOINTS.recon,
    fallback: () => RECON_ROWS,
  });
  const rows = recon.data ?? [];

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">对账</h1>
          <span className="text-[13px] text-muted">
            每日数据完整性核验 · 异常不掩盖，如实呈现
          </span>
        </div>
      </div>

      <div className="mt-4 flex flex-col gap-2.5">
        {rows.map((r) => (
          <div
            key={r.date}
            className="flex items-center gap-4 rounded border border-line bg-card px-4 py-3.5 shadow-card"
          >
            <span className="w-[92px] text-base font-bold">{r.date}</span>
            <Chip tone={STATUS[r.status].tone}>{STATUS[r.status].label}</Chip>
            <div className="flex flex-1 gap-5 text-[13px] text-muted">
              <span>
                快讯 <Num value={r.flashes} className="inline text-ink" />
              </span>
              <span>
                事件 <Num value={r.events} className="inline text-ink" />
              </span>
              <span>
                异常 <Num value={r.anomalies} className={`inline ${r.anomalies ? "text-up" : "text-ink"}`} />
              </span>
            </div>
            {r.note && (
              <span className="hidden max-w-[380px] truncate text-xs text-amber md:inline">
                {r.note}
              </span>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
