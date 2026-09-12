"use client";

import { Link } from "react-router-dom";
import { ArrowLeft, ShieldCheck, ShieldAlert, Loader2 } from "lucide-react";
import { Chip } from "@/components/ui/Num";
import type { ResearchStatus } from "@/lib/types";
import { STATUS_LABEL } from "@/hooks/useResearchRun";

const PROFILE_LABEL: Record<string, string> = {
  "complete-stock": "全面深研",
  "short-term": "短线视角",
};

/** 报告头：股票 + 代码 + as_of + profile + 机检徽标（§4.8）。 */
export default function ReportHeader({
  code,
  symbol,
  asOf,
  profile,
  status,
  lintOK,
  gateOK,
  model,
  tokens,
}: {
  code: string;
  symbol: string;
  asOf: string;
  profile: string;
  status: ResearchStatus;
  lintOK: boolean;
  gateOK: boolean;
  model: string;
  tokens: number;
}) {
  const passed = lintOK && gateOK;
  return (
    <div className="panel panel-pad">
      <Link
        to="/entities"
        className="mb-3 inline-flex items-center gap-1.5 text-[12px] text-muted no-underline hover:text-accent"
      >
        <ArrowLeft size={12} />
        返回实体库
      </Link>
      <div className="flex flex-wrap items-center gap-2.5">
        <h1 className="m-0 text-xl font-bold">{symbol}</h1>
        <span className="num text-sm text-muted">{code}</span>
        <Chip tone="dim">{PROFILE_LABEL[profile] ?? profile}</Chip>
        {status !== "done" && (
          <Chip tone="amber">
            {status === "failed" ? (
              STATUS_LABEL[status]
            ) : (
              <span className="inline-flex items-center gap-1">
                <Loader2 size={11} className="animate-spin" />
                {STATUS_LABEL[status]}
              </span>
            )}
          </Chip>
        )}
        {/* 机检徽标：核心结论可信度的唯一门（Number Lint + Quality Gate） */}
        {lintOK || gateOK ? (
          <span className="st st-accent inline-flex items-center gap-1">
            <ShieldCheck size={11} />
            数字已机检通过
          </span>
        ) : (
          <span className="st st-up inline-flex items-center gap-1">
            <ShieldAlert size={11} />
            AI 研判未通过数字机检
          </span>
        )}
      </div>
      <div className="num mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-faint">
        <span>数据截止 {asOf}</span>
        <span>模型 {model || "—"}</span>
        <span>tokens {tokens.toLocaleString("zh-CN")}</span>
      </div>
    </div>
  );
}
