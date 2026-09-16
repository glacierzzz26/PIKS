"use client";

import { useState } from "react";
import { Link } from "react-router-dom";
import { ArrowLeft, ShieldCheck, ShieldAlert, Loader2 } from "lucide-react";
import { Chip } from "@/components/ui/Num";
import { reportTypeLabel, coverSub, traceability } from "@/lib/report";
import { STATUS_LABEL } from "@/hooks/useResearchRun";
import ReportChecks from "@/components/report/ReportChecks";
import type { ResearchRun } from "@/lib/types";

/**
 * 研报封面头（design report-layout.md §4.2）。三行：
 *   1. 主体名 + 主体码（等宽）+ 报告类型 chip
 *   2. 副信息（层级 · 成分 · 数据截至）
 *   3. 结论 chip + 可溯源计数 + 机检徽标（可展开）
 *
 * ⚠️ 与个股分析页的 `ReportHeader` 是两套：那里是运维仪表盘（含 tokens/模型），
 * 这里是文档体裁（不含管线信息，D-R9）。个股分析页不动。
 */
export default function ReportCover({ run }: { run: ResearchRun }) {
  // 行业主体的展示名来自指标卡；公司主体为实体富化名。两者都缺 → 退回 code（不臆测）
  const name = run.display_name || run.name || run.code;
  const sub = coverSub(run);
  const ok = (run.lint?.passed ?? false) && (run.gate?.passed ?? false);
  const trace = traceability(run.lint);

  return (
    <header className="report-cover">
      <Link
        to="/reports"
        className="mb-3 inline-flex items-center gap-1.5 text-[12px] text-muted no-underline hover:text-accent"
      >
        <ArrowLeft size={12} />
        返回研报列表
      </Link>

      <h1>
        <span>{name}</span>
        <span className="code">{run.code}</span>
        <Chip tone="accent">{reportTypeLabel(run.subject_type)}</Chip>
        {run.status !== "done" && (
          <Chip tone="amber">
            {run.status === "failed" ? (
              STATUS_LABEL[run.status]
            ) : (
              <span className="inline-flex items-center gap-1">
                <Loader2 size={11} className="animate-spin" />
                {STATUS_LABEL[run.status]}
              </span>
            )}
          </Chip>
        )}
      </h1>

      {sub.length > 0 && <div className="sub">{sub.join(" · ")}</div>}

      <div className="badges">
        {/* 机检徽标 —— 兑现「机检未过必须仍可见」（§8 诚实） */}
        {ok ? (
          <span className="st st-accent inline-flex items-center gap-1">
            <ShieldCheck size={11} />
            机检通过
          </span>
        ) : (
          <span className="st st-up inline-flex items-center gap-1">
            <ShieldAlert size={11} />
            AI 研判未过机检
          </span>
        )}
        {trace && (
          <span className="st st-dim inline-flex items-center gap-1" title="Number Lint：正文数字的溯源比例">
            <ShieldCheck size={11} />
            {trace}
          </span>
        )}
      </div>

      <ReportChecks lint={run.lint} gate={run.gate} />
    </header>
  );
}
