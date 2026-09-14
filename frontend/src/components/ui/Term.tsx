"use client";

import { HelpCircle } from "lucide-react";
import { GLOSSARY } from "@/lib/glossary";

/**
 * 行内术语提示：hover / focus 弹出白话解释（纯 CSS，无 JS 状态）。
 * 术语来源统一走 lib/glossary.ts，与 /help 页共享。
 */
export default function Term({ k, children }: { k: string; children?: React.ReactNode }) {
  const entry = GLOSSARY[k];
  if (!entry) return <>{children ?? k}</>;
  return (
    <span className="term" tabIndex={0}>
      {children ?? entry.label}
      <HelpCircle size={11} strokeWidth={2.2} className="term-ico" aria-hidden="true" />
      <span className="term-pop" role="tooltip">
        {entry.def}
      </span>
    </span>
  );
}
