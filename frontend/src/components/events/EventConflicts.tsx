"use client";

import { AlertTriangle } from "lucide-react";
import type { EventItem } from "@/lib/types";

type Conflict = NonNullable<EventItem["event_conflicts"]>[number];

/**
 * 跨源说法不一致区（issue #49 T3）。
 *
 * 红线：**只指出分歧，不替人下结论**（三层模型 Fact ≠ Inference）。
 * 每条冲突都把**双方原话**并排摆出来，绝不显示「结论版」或只留一边 ——
 * 静默择一正是本功能要消灭的行为。
 *
 * 白话文案，不出现 cluster / facts 等实现黑话（P6-2 纪律）。
 */
export default function EventConflicts({ items }: { items: Conflict[] }) {
  if (items.length === 0) return null;
  return (
    <div className="mt-5 border-t border-line pt-3.5">
      <h3 className="mb-2 flex items-center gap-1.5 text-[15px] font-bold">
        <AlertTriangle size={15} className="text-[var(--warn)]" />
        来源说法不一致
      </h3>
      <p className="mb-3 text-xs leading-relaxed text-faint">
        不同机构报道同一件事时，下面这些数字对不上。PIKS 不替你挑一个版本 ——
        两边的原话都在这里，由你判断。
      </p>
      <ul className="m-0 list-none space-y-3 p-0">
        {items.map((c, i) => (
          <li key={`${c.unit}-${i}`} className="rounded-[10px] border border-line bg-card-soft p-3">
            <div className="mb-2 flex items-baseline gap-2">
              <span className="text-xs text-muted">对不上的数字</span>
              <span className="num text-sm font-bold">
                {c.values.map((v) => String(v)).join(" ／ ")}
              </span>
              {c.unit && <span className="text-xs text-faint">{c.unit}</span>}
            </div>
            <Saying text={c.sentence_a} />
            <Saying text={c.sentence_b} />
          </li>
        ))}
      </ul>
    </div>
  );
}

/** 一方原话：左侧竖线标明「这是原文」，不改写、不摘要。 */
function Saying({ text }: { text: string }) {
  return (
    <div className="mb-1.5 border-l-2 border-line pl-2.5 text-[13px] leading-relaxed last:mb-0">
      {text}
    </div>
  );
}
