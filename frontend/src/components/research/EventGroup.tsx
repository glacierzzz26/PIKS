"use client";

import type { ResearchEvidence, ResearchMetrics } from "@/lib/types";

type Events = {
  total_news?: number;
  total_events?: number;
  recent_news?: { title: string; source: string; published: string }[];
};

/** 事件节：标题列表（research 已降噪去重）+ 降噪前后计数。 */
export default function EventGroup({
  metrics,
  evidence,
}: {
  metrics: ResearchMetrics;
  evidence: ResearchEvidence[];
}) {
  const ev = metrics.events as Events | undefined;
  if (!ev) return null;
  const news = ev.recent_news ?? [];
  return (
    <div className="panel panel-pad">
      <div className="mb-3 flex items-center gap-2">
        <h3 className="m-0 text-[15px] font-bold">事件与新闻</h3>
        <span className="num text-[11px] text-faint">
          来源：确定性计算 + Evidence {evidence.length} 条
        </span>
      </div>
      <div className="num mb-2 text-[12px] text-muted">
        原始 {ev.total_news ?? "—"} 条 · 降噪后 {ev.total_events ?? "—"} 条
      </div>
      {news.length === 0 ? (
        <div className="text-[13px] text-faint italic">区间内无新闻记录</div>
      ) : (
        <ul className="m-0 list-none space-y-1.5 p-0">
          {news.map((n, i) => (
            <li key={i} className="text-[13px] leading-snug">
              {n.title}
              <span className="num ml-2 text-[11px] text-faint">
                {n.source} · {n.published?.slice(0, 10)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
