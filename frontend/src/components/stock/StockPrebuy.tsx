"use client";

import { Loader2, RefreshCw } from "lucide-react";
import { usePrebuy } from "@/hooks/usePrebuy";
import { STATUS_LABEL } from "@/hooks/useResearchRun";
import MetricGroup from "@/components/research/MetricGroup";
import Scorecard from "@/components/research/Scorecard";
import OpinionSection from "@/components/research/OpinionSection";
import VolumePriceChart from "@/components/charts/VolumePriceChart";
import { FINANCIAL_ROWS, VALUATION_ROWS } from "@/components/research/metricRows";
import PrebuyVerdict from "./PrebuyVerdict";
import RiskList from "./RiskList";
import PatternTags from "./PatternTags";

/**
 * 买入前速评（个股页首屏置顶）。一键现场重跑，原地出卡片不跳页。
 * 结论来源 = 规则确定性（评分卡 + 风险规则），AI 综合研判有则锦上添花。
 */
export default function StockPrebuy({ code }: { code: string }) {
  const { report, status, active, busy, error, hasExisting, trigger } = usePrebuy(code);

  return (
    <div className="panel panel-pad">
      <div className="mb-4 flex flex-wrap items-center gap-3">
        {report?.as_of && (
          <span className="num text-[11.5px] text-faint">截至 {report.as_of}</span>
        )}
        <span className="text-[11.5px] text-faint">
          现场重跑约 10~30 秒 · 结论取确定性评分卡与风险规则，不依赖 AI
        </span>
        <button
          onClick={trigger}
          disabled={active || busy}
          className="ml-auto inline-flex h-7 items-center gap-1 rounded-sm border border-line bg-card px-2.5 text-[11.5px] text-muted hover:border-accent hover:text-accent disabled:opacity-60"
        >
          {active || busy ? <Loader2 size={12} className="animate-spin" /> : <RefreshCw size={12} />}
          {active ? (status ? STATUS_LABEL[status] : "提交中") : hasExisting ? "重新分析" : "快速分析"}
        </button>
      </div>

      {error && !active && (
        <div className="mb-3 rounded-[var(--radius-md)] border border-line bg-bg-soft px-3 py-2 text-[12.5px] text-down">
          速评失败：{error}
        </div>
      )}

      {active && (
        <div className="flex flex-col items-center justify-center gap-2 py-10 text-faint">
          <Loader2 size={20} className="animate-spin" />
          <p className="m-0 text-[13px]">
            {status ? STATUS_LABEL[status] : "提交中"}… 采集数据并做确定性分析
          </p>
        </div>
      )}

      {!active && !report && !error && (
        <div className="flex flex-col items-center justify-center gap-2 py-10 text-faint">
          <p className="m-0 text-[13px] italic">
            {hasExisting ? "速评记录读取中…" : "还没做过买入前速评 —— 点右上「快速分析」现场跑一份"}
          </p>
        </div>
      )}

      {!active && report && <ReportBody report={report} />}
    </div>
  );
}

/** 速评正文（仅 done 时渲染）：结论 → 风险 → 财务/估值 → 量价形态 → AI。 */
function ReportBody({ report }: { report: NonNullable<ReturnType<typeof usePrebuy>["report"]> }) {
  const { metrics, evidence, synthesis, model, tokens } = report;
  const risk = metrics.risk;
  const patterns = metrics.patterns;
  const bySection = (s: string) => evidence.filter((e) => e.section === s);

  return (
    <>
      <PrebuyVerdict metrics={metrics} />

      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <div className="panel panel-pad">
          <div className="mb-3 flex items-center gap-2">
            <h3 className="m-0 text-[15px] font-bold">风险红线</h3>
            <span className="text-[11px] text-faint">规则判定 · 买入前最该看</span>
          </div>
          <RiskList items={risk?.items ?? []} />
        </div>
        <Scorecard metrics={metrics} />
        <MetricGroup
          title="财务基本面"
          rows={FINANCIAL_ROWS}
          data={metrics.financial}
          evidence={bySection("financial")}
        />
        <MetricGroup
          title="估值"
          rows={VALUATION_ROWS}
          data={metrics.financial}
          evidence={bySection("financial")}
        />
      </div>

      {patterns && (patterns.series?.length ?? 0) > 0 && (
        <div className="mt-3 panel panel-pad">
          <h3 className="mb-3 text-[15px] font-bold">量与价的走法</h3>
          <VolumePriceChart series={patterns.series ?? []} />
          <PatternTags
            labels={patterns.labels ?? []}
            divergence={patterns.divergence}
            note={patterns.note}
          />
        </div>
      )}

      {synthesis && (synthesis.summary || synthesis.trend || synthesis.conclusion) && (
        <OpinionSection
          synthesis={synthesis}
          model={model}
          tokens={tokens}
          heading="AI 综合研判（仅供参考）"
        />
      )}
    </>
  );
}
