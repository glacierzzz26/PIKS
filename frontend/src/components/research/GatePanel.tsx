"use client";

import { useState } from "react";
import { ChevronRight, Check, X } from "lucide-react";
import type { ResearchGate, ResearchIssue, ResearchLint } from "@/lib/types";

function IssueList({ issues }: { issues: ResearchIssue[] }) {
  if (issues.length === 0) {
    return <div className="text-[12px] text-faint italic">无问题</div>;
  }
  return (
    <ul className="m-0 list-none space-y-1 p-0">
      {issues.map((it, i) => (
        <li key={i} className="text-[12px] leading-snug text-up">
          · {it.detail ?? JSON.stringify(it)}
        </li>
      ))}
    </ul>
  );
}

/** 机检详情（折叠）：Number Lint 扫描统计 + 六项 Quality Gate 逐项。 */
export default function GatePanel({
  lint,
  gate,
}: {
  lint: ResearchLint;
  gate: ResearchGate;
}) {
  const [open, setOpen] = useState(false);
  const checks = gate.checks ?? [];
  const passed = (lint.passed ?? false) && (gate.passed ?? false);

  return (
    <section className="mt-5">
      <div className="mb-2 flex items-baseline gap-2">
        <h2 className="m-0 text-[15px] font-bold tracking-wide">三、数字机检详情</h2>
      </div>
      <div className="panel">
        <button
          onClick={() => setOpen((o) => !o)}
          className="flex w-full items-center gap-2 px-4 py-3 text-left"
        >
          <ChevronRight
            size={14}
            className={`text-faint transition-transform ${open ? "rotate-90" : ""}`}
          />
          <span className="text-[13px] font-semibold">Number Lint + Quality Gate</span>
          <span
            className={`num ml-auto text-[12px] ${passed ? "text-accent" : "text-up"}`}
          >
            {passed ? "全部通过" : "有未过项"}
          </span>
        </button>

        {open && (
          <div className="border-t border-line px-4 py-3">
            <div className="num mb-3 text-[12px] text-muted">
              Number Lint：扫描 {lint.scanned ?? "—"} · 匹配 {lint.matched ?? "—"} · 忽略{" "}
              {lint.ignored ?? "—"}
            </div>
            <IssueList issues={lint.issues ?? []} />

            <div className="mb-2 mt-4 text-[12px] font-semibold text-muted">
              Quality Gate（六项）
            </div>
            <table className="table">
              <tbody>
                {checks.map((c) => (
                  <tr key={c.name} className="h-[36px]">
                    <td style={{ width: 20 }}>
                      {c.passed ? (
                        <Check size={13} className="text-accent" />
                      ) : (
                        <X size={13} className="text-up" />
                      )}
                    </td>
                    <td className="text-left text-muted">
                      {c.name}
                    </td>
                    <td className="text-left text-[12px] text-faint">
                      {c.detail}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="mt-3">
              <IssueList issues={gate.issues ?? []} />
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
