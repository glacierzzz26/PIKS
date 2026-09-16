"use client";

import { Suspense } from "react";
import { useData } from "@/hooks/useData";
import { useUrlState } from "@/hooks/useUrlState";
import { ENDPOINTS } from "@/lib/api";
import { RESEARCH_PROFILES } from "@/lib/constants";
import { LoadingBlock } from "@/components/ui/States";
import AnalystTrigger from "@/components/research/AnalystTrigger";
import RunHistoryTable from "@/components/research/RunHistoryTable";
import type { ResearchRunList } from "@/lib/types";

const DEFAULT_PROFILE = RESEARCH_PROFILES[0].key;

/**
 * 个股分析师（独立界面）。用户日常不查实体库 —— 这里直接输入代码触发深研，
 * 并列出全部历史报告，不依赖实体/持仓入口。
 * 触发成功后跳 /research/:runId（报告页自带轮询与三态）。
 * 选择状态入 URL（规则 7）：?profile=。
 */
export default function Page() {
  return (
    <Suspense fallback={<div className="panel mt-6"><LoadingBlock rows={6} /></div>}>
      <AnalystInner />
    </Suspense>
  );
}

function AnalystInner() {
  const [query, setParam] = useUrlState();
  const profile = query.profile ?? DEFAULT_PROFILE;

  // 不带 code/entity/limit = 全量列表（as_of DESC）。落地页重挂载即刷新。
  const { data, loading, error } = useData<ResearchRunList>({
    path: ENDPOINTS.researchRuns,
  });
  const runs = data?.runs ?? [];

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>个股分析</h1>
          <div className="psub">
            输入 6 位代码，选报告类型 —— 让 AI 做一次深度分析
          </div>
        </div>
        <div className="meta">
          <span className="st st-accent">
            共 {loading ? "…" : runs.length} 份报告
          </span>
        </div>
      </div>

      <AnalystTrigger
        profile={profile}
        onProfile={(p) => setParam("profile", p === DEFAULT_PROFILE ? "" : p)}
      />

      <div className="mb-2 mt-5 flex items-baseline gap-2">
        <h2 className="m-0 text-[15px] font-bold tracking-wide">历史报告</h2>
        <span className="text-[12px] text-faint">点击任意行查看完整报告</span>
      </div>
      <RunHistoryTable runs={runs} loading={loading} error={error} />
    </div>
  );
}
