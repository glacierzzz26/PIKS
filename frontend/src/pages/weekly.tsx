"use client";

import { Link } from "react-router-dom";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { getDocs } from "@/lib/mockService";
import type { Doc } from "@/lib/types";
import { Chip } from "@/components/ui/Num";
import { EmptyState } from "@/components/ui/States";

/** 周报（只读；对齐 dev /weekly）：规则聚合 + AI 综述，阅读跳转笔记页 */
export default function Page() {
  const weekly = useData<Doc[]>({
    path: ENDPOINTS.weekly,
    fallback: () => getDocs().filter((d) => d.type === "weekly"),
  });
  const weeklies = weekly.data ?? [];

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">周报</h1>
          <span className="num text-[13px] text-muted">
            {weeklies.length} 期
          </span>
          <span className="ml-auto text-xs text-muted">
            AI 综述触发在 Go 端 /weekly（只读约束）
          </span>
        </div>
      </div>

      {weeklies.length === 0 ? (
        <div className="mt-4 rounded border border-line bg-card shadow-card">
          <EmptyState tip="暂无周报" />
        </div>
      ) : (
        <div className="mt-4 flex flex-col gap-2.5">
          {weeklies.map((w) => (
            <Link
              key={w.id}
              to={`/notes/${w.id}`}
              className="block rounded border border-line bg-card p-4 shadow-card no-underline hover:border-accent"
            >
              <div className="flex items-center gap-2.5">
                <span className="text-base font-semibold text-ink">
                  {w.title}
                </span>
                <Chip tone="accent">周报</Chip>
                <span className="num ml-auto text-xs text-muted">
                  {w.updated_at}
                </span>
              </div>
              <p className="mb-0 mt-1.5 text-[13px] text-muted">
                {w.content.replace(/[#*>|-]/g, "").slice(0, 130)}…
              </p>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
