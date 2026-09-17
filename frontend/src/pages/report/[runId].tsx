"use client";

import { useEffect } from "react";
import { useParams } from "react-router-dom";
import { Loader2, RefreshCw, AlertTriangle } from "lucide-react";
import { useData } from "@/hooks/useData";
import { isActive } from "@/hooks/useResearchRun";
import { ENDPOINTS } from "@/lib/api";
import { ErrorState, EmptyState } from "@/components/ui/States";
import { reportChapters, tocEntries } from "@/lib/report";
import ReportCover from "@/components/report/ReportCover";
import ReportToc from "@/components/report/ReportToc";
import ReportBody, { ReportFoot } from "@/components/report/ReportBody";
import MarkdownBody from "@/components/md/MarkdownBody";
import type { ResearchRun } from "@/lib/types";

const POLL_MS = 1500;

/**
 * 研报阅读器（design report-layout.md §4）。独立体裁，与个股分析页 `/research/:runId`
 * 的运维仪表盘**并存不互替**（D-R1）：这里是「单栏正文 + 左侧目录」的文档版面。
 *
 * 状态非终态时轮询（文字徽标，核心数字区禁 skeleton）。
 */
export default function Page() {
  const { runId = "" } = useParams();
  const run = useData<ResearchRun>({
    path: runId ? ENDPOINTS.researchRun.replace(":runId", runId) : null,
  });

  const status = run.data?.status;
  const polling = isActive(status);
  useEffect(() => {
    if (!polling) return;
    const t = window.setTimeout(run.refresh, POLL_MS);
    return () => window.clearTimeout(t);
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

  return (
    <div className="mx-auto mt-5 max-w-[1080px]">
      <ReportCover run={d} />

      {d.status === "failed" ? (
        <div className="panel panel-pad mt-4">
          <div className="flex items-center gap-2 text-up">
            <AlertTriangle size={15} />
            <span className="text-sm font-semibold">本轮研报未完成</span>
          </div>
          <p className="mt-2 text-[13px] leading-relaxed text-muted">{d.error}</p>
          <button
            onClick={run.refresh}
            className="mt-3 inline-flex h-8 items-center gap-1.5 rounded border border-line bg-card px-3 text-xs text-muted hover:text-accent"
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
        <Body d={d} />
      )}
    </div>
  );
}

/** 正文装配：左目录 + 右正文 + 页脚（§4.1 骨架）。 */
function Body({ d }: { d: ResearchRun }) {
  const { chapters, foot } = reportChapters(d);
  const priorRuns = Number(d.metrics?.meta?.prior_runs ?? 0);
  return (
    <>
      <div className="report-grid">
        <ReportToc items={tocEntries(chapters, foot)} />
        <div className="min-w-0">
          <ReportBody chapters={chapters} />
          {foot && (
            <ReportFoot>
              <MarkdownBody content={foot.markdown} />
            </ReportFoot>
          )}
          {priorRuns > 0 && (
            <div className="mt-3 text-[12px] text-faint">
              本份参考了 {priorRuns} 份既往研报（AI 研判中的「较上次」对比即源于此）
            </div>
          )}
        </div>
      </div>
    </>
  );
}
