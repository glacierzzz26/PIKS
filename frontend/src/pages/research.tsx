"use client";

import { useEffect } from "react";
import { useParams } from "react-router-dom";
import { Loader2, RefreshCw, AlertTriangle } from "lucide-react";
import { useData } from "@/hooks/useData";
import { isActive } from "@/hooks/useResearchRun";
import { ENDPOINTS } from "@/lib/api";
import { ErrorState, EmptyState } from "@/components/ui/States";
import ReportHeader from "@/components/research/ReportHeader";
import FactSection from "@/components/research/FactSection";
import OpinionSection from "@/components/research/OpinionSection";
import GatePanel from "@/components/research/GatePanel";
import MarkdownBody from "@/components/md/MarkdownBody";
import type { ResearchRun } from "@/lib/types";

const POLL_MS = 1500;

/**
 * 个股深研报告页（只读）。Fact / Opinion / 机检 三分区。
 * 状态非 done 时轮询（文字徽标，非骨架屏 —— 核心数字区禁 skeleton）：
 * 进行中 → 提示等待；failed → error 原文 + 重试入口。
 */
export default function Page() {
  const { runId = "" } = useParams();
  const run = useData<ResearchRun>({
    path: runId ? ENDPOINTS.researchRun.replace(":runId", runId) : null,
  });

  // 非终态轮询：done/failed 即停（刷新经 refresh，不在渲染期排定时器）。
  const status = run.data?.status;
  const polling = isActive(status);
  useEffect(() => {
    if (!polling) return;
    const t = window.setTimeout(run.refresh, POLL_MS);
    return () => window.clearTimeout(t);
    // refresh 每次渲染都是新引用，故意只按 polling 变化重排
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [polling, run.data?.updated_at]);

  if (run.loading && !run.data) {
    return (
      <div className="panel mt-6 py-20 text-center text-[13px] text-faint">
        <Loader2 size={18} className="mx-auto mb-2 animate-spin" />
        加载报告中…
      </div>
    );
  }
  if (run.error) {
    return (
      <div className="panel mt-6">
        <ErrorState msg={run.error} />
      </div>
    );
  }
  const d = run.data;
  if (!d) {
    return (
      <div className="panel mt-6">
        <EmptyState tip="报告不存在或已被清理" />
      </div>
    );
  }

  const lintOK = d.lint?.passed ?? false;
  const gateOK = d.gate?.passed ?? false;

  return (
    <div className="mx-auto max-w-[1100px]">
      <div className="mt-5">
        <ReportHeader
          code={d.code}
          symbol={d.symbol}
          name={d.name}
          asOf={d.as_of}
          profile={d.profile}
          status={d.status}
          lintOK={lintOK}
          gateOK={gateOK}
          model={d.model}
          tokens={d.tokens}
        />
      </div>

      {d.status === "failed" ? (
        <div className="panel panel-pad mt-4">
          <div className="flex items-center gap-2 text-up">
            <AlertTriangle size={15} />
            <span className="text-sm font-semibold">本轮深研未完成</span>
          </div>
          <p className="mt-2 text-[13px] leading-relaxed text-muted">{d.error}</p>
          <button
            onClick={run.refresh}
            className="mt-3 inline-flex h-8 items-center gap-1.5 rounded-[10px] border border-line bg-card px-3 text-xs text-muted hover:text-accent"
          >
            <RefreshCw size={12} />
            重新查询
          </button>
        </div>
      ) : d.status !== "done" ? (
        <div className="panel panel-pad mt-4 text-center">
          <Loader2 size={16} className="mx-auto mb-2 animate-spin text-accent" />
          <div className="text-[13px] text-muted">
            正在采集合成，通常 10~60 秒，可留在本页等待…
          </div>
        </div>
      ) : (
        <>
          <div className="mt-4">
            <FactSection metrics={d.metrics} evidence={d.evidence ?? []} />
          </div>
          <OpinionSection synthesis={d.synthesis} model={d.model} tokens={d.tokens} />
          <GatePanel lint={d.lint} gate={d.gate} />

          <section className="mt-5">
            <div className="mb-2 flex items-baseline gap-2">
              <h2 className="m-0 text-[15px] font-bold tracking-wide">四、报告正文</h2>
              <span className="text-[12px] text-faint">
                {lintOK && gateOK ? "AI 研判已过机检" : "AI 研判未过机检，正文为确定性骨架"}
              </span>
            </div>
            <article className="panel panel-pad px-6 py-5">
              <MarkdownBody content={d.markdown} />
            </article>
          </section>
        </>
      )}
    </div>
  );
}
