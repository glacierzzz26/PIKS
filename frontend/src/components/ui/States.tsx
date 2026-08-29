"use client";

import { AlertCircle, Inbox } from "lucide-react";

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
      <p className="max-w-sm text-[13px] text-muted">{msg}</p>
    </div>
  );
}

export function EmptyState({ tip = "暂无数据" }: { tip?: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 text-muted">
      <Inbox size={22} strokeWidth={1.8} />
      <p className="text-[13px] italic">{tip}</p>
    </div>
  );
}
