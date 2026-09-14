"use client";

import { AlertCircle, Inbox } from "lucide-react";
import { Link } from "react-router-dom";

/** 三态（规范第 9 条）：loading / error / empty */
export function LoadingBlock({ rows = 6 }: { rows?: number }) {
  return (
    <div className="divide-y divide-line">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex h-12 items-center gap-4 px-4">
          <div className="h-3 w-40 animate-pulse rounded bg-bg-soft" />
          <div className="ml-auto h-3 w-16 animate-pulse rounded bg-bg-soft" />
          <div className="h-3 w-12 animate-pulse rounded bg-bg-soft" />
        </div>
      ))}
    </div>
  );
}

export function ErrorState({ msg }: { msg: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 text-center">
      <AlertCircle size={22} className="text-up" strokeWidth={1.8} />
      <p className="text-sm">加载失败</p>
      <p className="max-w-sm text-[13px] txt-faint">{msg}</p>
    </div>
  );
}

/** 空态可带一个行动入口（to 内部路由 / label 按钮文案），引导新手下一步。 */
export function EmptyState({
  tip = "暂无数据",
  action,
}: {
  tip?: string;
  action?: { to: string; label: string };
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 txt-faint">
      <Inbox size={22} strokeWidth={1.8} />
      <p className="text-[13px] italic">{tip}</p>
      {action && (
        <Link
          to={action.to}
          className="mt-1 inline-flex h-8 items-center rounded-[10px] border border-line bg-card px-3 text-xs font-semibold text-muted no-underline hover:text-accent"
        >
          {action.label}
        </Link>
      )}
    </div>
  );
}
