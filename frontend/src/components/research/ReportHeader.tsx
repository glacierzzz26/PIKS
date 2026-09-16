"use client";

import { Link } from "react-router-dom";
import { ArrowLeft, ShieldCheck, ShieldAlert, Loader2 } from "lucide-react";
import { Chip } from "@/components/ui/Num";
import { RESEARCH_PROFILE_LABEL } from "@/lib/constants";
import type { ResearchStatus } from "@/lib/types";
import { STATUS_LABEL } from "@/hooks/useResearchRun";

/** 报告头：股票 + 代码 + as_of + profile + 机检徽标（§4.8）。 */
export default function ReportHeader({
  code,
  symbol,
  name,
  asOf,
  profile,
  status,
  lintOK,
  gateOK,
  model,
  tokens,
  backTo = "/research",
  backLabel = "返回个股分析师",
}: {
  code: string;
  symbol: string;
  /** 公司名；空 = 未建实体，标题退回代码（如实不臆测） */
  name: string;
  asOf: string;
  profile: string;
  status: ResearchStatus;
  lintOK: boolean;
  gateOK: boolean;
  model: string;
  tokens: number;
  backTo?: string;
  backLabel?: string;
}) {
  const passed = lintOK && gateOK;
  // 标题优先公司名，缺则退回 full_code(symbol) —— 后者仍比裸 6 位码可读。
  const title = name || symbol || code;
  return (
    <div className="panel panel-pad">
      <Link
        to={backTo}
        className="mb-3 inline-flex items-center gap-1.5 text-[12px] text-muted no-underline hover:text-accent"
      >
        <ArrowLeft size={12} />
        {backLabel}
      </Link>
      <div className="flex flex-wrap items-center gap-2.5">
        <h1 className="m-0 text-xl font-bold">{title}</h1>
        <Link
          to={`/stock/${code}`}
          className="num text-sm text-muted no-underline hover:text-accent"
        >
          {code}
        </Link>
        <Chip tone="dim">{RESEARCH_PROFILE_LABEL[profile] ?? profile}</Chip>
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
