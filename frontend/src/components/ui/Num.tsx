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
    return <span className="num block text-faint">—</span>;
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
      {suffix && <span className="ml-0.5 text-faint">{suffix}</span>}
    </span>
  );
}

/** 语义状态胶囊（对齐 HTML .st）。tone 沿用旧 API：accent/up/down/amber/dim */
export function Chip({
  children,
  tone = "dim",
}: {
  children: ReactNode;
  tone?: "dim" | "accent" | "up" | "down" | "amber";
}) {
  return <span className={`st st-${tone}`}>{children}</span>;
}

/** 置信度条（对齐 HTML .pct-wrap/.pct-bar/.pct-num） */
export function ConfidenceBar({ v }: { v: number }) {
  return (
    <span className="pct-wrap">
      <span className="pct-bar">
        <i style={{ width: `${Math.max(0, Math.min(1, v)) * 100}%` }} />
      </span>
      <span className="pct-num">{(v * 100).toFixed(0)}%</span>
    </span>
  );
}
