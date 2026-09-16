"use client";

import { useState } from "react";
import { ChevronRight, Check, X } from "lucide-react";
import type { ResearchGate, ResearchIssue, ResearchLint } from "@/lib/types";

function IssueList({ issues }: { issues: ResearchIssue[] }) {
  if (issues.length === 0) return null;
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

/**
 * 机检折叠区（D-R9，design report-layout.md §4.2）。
 *
 * 研报正文是给读者看的报告，不是管线状态面板 —— 故机检详情从正文移出，降为封面头
 * 默认收起的一栏。**保留而非删除**：诚实原则要求「机检未过」可查（§8）。
 */
export default function ReportChecks({
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
    <details
      className="report-checks"
      open={open}
      onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}
    >
      <summary>
        <span className="inline-flex items-center gap-1.5">
          <ChevronRight
            size={13}
            className={`inline transition-transform ${open ? "rotate-90" : ""}`}
          />
          数字机检详情（Number Lint + Quality Gate）
        </span>
        <span className={`num ml-2 ${passed ? "text-accent" : "text-up"}`}>
          {passed ? "全部通过" : "有未过项"}
        </span>
      </summary>

      <div className="mt-2.5">
        <div className="num text-[12px] text-muted">
          Number Lint：扫描 {lint.scanned ?? "—"} · 匹配 {lint.matched ?? "—"} · 忽略{" "}
          {lint.ignored ?? "—"}
        </div>
        <IssueList issues={lint.issues ?? []} />

        <div className="mb-1 mt-3 text-[12px] font-semibold text-muted">
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
                <td className="text-left text-muted">{c.name}</td>
                <td className="text-left text-[12px] text-faint">{c.detail}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <IssueList issues={gate.issues ?? []} />
      </div>
    </details>
  );
}
