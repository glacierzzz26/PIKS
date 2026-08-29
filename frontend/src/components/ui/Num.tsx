"use client";

import type { ReactNode } from "react";

/**
 * 数字单元格：等宽 + tabular-nums + 右对齐（规范第 2 条）。
 * 涨跌自动按 A 股习惯着色（涨红跌绿）。
 */
export function Num({
  value,
  suffix,
  colored = false,
  className = "",
}: {
  value: number | null;
  suffix?: string;
  colored?: boolean;
  className?: string;
}) {
  if (value === null || value === undefined) {
    return <span className="num block text-muted">—</span>;
  }
  const color = colored
    ? value > 0
      ? "text-up"
      : value < 0
        ? "text-down"
        : "text-muted"
    : "";
  const sign = colored && value > 0 ? "+" : "";
  return (
    <span className={`num block ${color} ${className}`}>
      {sign}
      {value.toLocaleString("zh-CN", { maximumFractionDigits: 2 })}
      {suffix && <span className="ml-0.5 text-muted">{suffix}</span>}
    </span>
  );
}

/** 语义胶囊徽章（对齐 base.css .chip） */
export function Chip({
  children,
  tone = "dim",
}: {
  children: ReactNode;
  tone?: "dim" | "accent" | "up" | "down" | "amber";
}) {
  return <span className={`chip chip-${tone}`}>{children}</span>;
}

/** 置信度条（0-1） */
export function ConfidenceBar({ v }: { v: number }) {
  const color = v >= 0.8 ? "bg-down" : v >= 0.6 ? "bg-accent" : "bg-up";
  return (
    <span className="flex items-center justify-end gap-2">
      <span className="num w-9 text-xs">{(v * 100).toFixed(0)}%</span>
      <span className="h-1.5 w-14 overflow-hidden rounded-full bg-bg-soft">
        <span
          className={`block h-full rounded-full ${color}`}
          style={{ width: `${v * 100}%` }}
        />
      </span>
    </span>
  );
}
