"use client";

import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import { useData } from "@/hooks/useData";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import { ENDPOINTS } from "@/lib/api";
import { LoadingBlock, ErrorState, EmptyState } from "@/components/ui/States";
import Pagination from "@/components/ui/Pagination";
import { RunStatusBadge, LintBadge } from "@/components/research/StatusBadges";
import {
  groupRunsBySubject,
  type SubjectGroup,
  type SubjectSection,
} from "@/lib/reportList";
import { reportTypeLabel } from "@/lib/report";
import type { ResearchRunList, ResearchRunSummary } from "@/lib/types";

/**
 * 研报列表（design report-layout.md §6「/reports（列表，主体为轴）」）。
 *
 * 与 `/research`（个股分析师）的区别：那是**触发入口**（输入代码让 AI 跑一次）+
 * 全库历史表；这里是**研报体裁的阅读入口**，按主体（行业/公司/宏观）分组呈现。
 * 触发仍走 `/research`（本页只读，不重复造轮子）。
 */
export default function Page() {
  const { data, loading, error } = useData<ResearchRunList>({
    path: ENDPOINTS.researchRuns,
  });
  const runs = data?.runs ?? [];
  const groups = useMemo(() => groupRunsBySubject(runs), [runs]);

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>研报</h1>
          <div className="psub">成篇的行业 / 公司研究报告 · 按主体分组</div>
        </div>
        <div className="meta">
          <span className="st st-accent">
            共 {loading ? "…" : runs.length} 份
          </span>
        </div>
      </div>

      {loading ? (
        <div className="panel">
          <LoadingBlock rows={6} />
        </div>
      ) : error ? (
        <div className="panel">
          <ErrorState msg={error} />
        </div>
      ) : groups.length === 0 ? (
        <div className="panel">
          <EmptyState
            tip="还没有研报 —— 到「个股分析」触发一次深研"
            action={{ to: "/research", label: "去个股分析" }}
          />
        </div>
      ) : (
        <GroupList groups={groups} />
      )}
    </div>
  );
}

/** 按主体渲染各分组（分页对**组**进行，不是对行）。 */
function GroupList({ groups }: { groups: SubjectSection[] }) {
  const { page, size, setPage, setSize, paginate } = usePagedQuery();
  const paged = paginate(groups);
  return (
    <>
      {paged.map((s) => (
        <Section key={s.type} section={s} />
      ))}
      <Pagination
        page={page}
        pageSize={size}
        total={groups.length}
        onPage={setPage}
        onPageSize={setSize}
      />
    </>
  );
}

/** 一个主体类型下的一节（标题 + 各组卡片）。 */
function Section({ section }: { section: SubjectSection }) {
  return (
    <section className="section">
      <div className="day-title">
        {section.label}
        <span className="text-[12px] font-normal text-faint">
          {section.groups.length} 个主体 · {section.count} 份
        </span>
      </div>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {section.groups.map((g) => (
          <SubjectCard key={g.key} group={g} />
        ))}
      </div>
    </section>
  );
}

/** 一个主体（一只票 / 一个行业）的卡片：最新一份为主，历史份数如实标注。 */
function SubjectCard({ group }: { group: SubjectGroup }) {
  const navigate = useNavigate();
  const latest: ResearchRunSummary | undefined = group.latest ?? group.unfinished[0];
  if (!latest) return null;
  const name = group.displayName || group.code;

  return (
    <button
      onClick={() => navigate(`/reports/${latest.run_id}`)}
      className="panel panel-pad w-full text-left transition-colors hover:border-accent"
    >
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="text-[15px] font-bold">{name}</span>
        <span className="num text-[12px] text-muted">{group.code}</span>
        <span className="st st-dim">{reportTypeLabel(group.type)}</span>
        {group.total > 1 && (
          <span className="text-[11.5px] text-faint">
            共 {group.total} 份 · 已呈现最新
          </span>
        )}
      </div>
      <div className="num mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-[12px] text-faint">
        <span>数据截至 {latest.as_of}</span>
        <span>{latest.model || "—"}</span>
      </div>
      <div className="mt-2.5 flex flex-wrap items-center gap-2">
        <RunStatusBadge status={latest.status} />
        <LintBadge s={latest} />
        <span className="ml-auto inline-flex items-center gap-1 text-[12px] text-accent">
          阅读 <ArrowRight size={12} />
        </span>
      </div>
    </button>
  );
}
