"use client";

import { Link } from "react-router-dom";
import { Newspaper, Microscope, Clock3, ChevronRight } from "lucide-react";
import type { WatchItem } from "@/lib/types";
import { daysAgo } from "@/lib/format";

/**
 * 「今天该看什么」—— 新手引导核心（设计 phase6/ux-ia.md §2 区块3）。
 * T1 纯前端概览句 + T2 每只票的待办徽标（有新消息 / 还没深研过 / 深研陈旧）。
 * 诚实分级：不伪造「买入后出了新报告」这类依赖决策边的提示（那是 P6-4+）。
 */
export default function TodayFocus({ items }: { items: WatchItem[] }) {
  const withNews = items.filter((i) => i.latest_event);
  const noResearch = items.filter((i) => i.code && !i.has_research);
  const stale = items.filter((i) => i.has_research && isStale(i.latest_research_asof));

  const todo = [
    ...withNews.map((i) => ({ kind: "news" as const, it: i })),
    ...noResearch.map((i) => ({ kind: "nores" as const, it: i })),
    ...stale.map((i) => ({ kind: "stale" as const, it: i })),
  ].slice(0, 5);

  if (items.length === 0) return null;

  return (
    <section className="section">
      <div className="section-head">
        <span className="bar" />
        <h2>今天该看什么</h2>
        <span className="hint">按新鲜度排序</span>
      </div>

      <p className="mb-3 text-[13px] leading-relaxed text-muted">
        你有 <b className="num text-ink">{items.length}</b> 只在自选，其中{" "}
        <b className="num text-ink">{items.filter((i) => i.held).length}</b> 只持仓。
        {withNews.length > 0 ? (
          <>
            今天有 <b className="num text-up">{withNews.length}</b> 只出了新消息。
          </>
        ) : (
          <>今天暂时没有新的相关消息。</>
        )}
        {noResearch.length > 0 && (
          <>
            还有 <b className="num text-ink">{noResearch.length}</b> 只你还没深研过。
          </>
        )}
      </p>

      {todo.length === 0 ? (
        <div className="panel panel-pad text-[13px] text-muted">
          自选都看过啦 —— 有新消息或新报告时会在这里提醒你。
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          {todo.map((t) => (
            <TodoRow key={`${t.kind}-${t.it.entity_id}`} kind={t.kind} it={t.it} />
          ))}
        </div>
      )}
    </section>
  );
}

const KIND = {
  news: { icon: Newspaper, tone: "text-up", text: (it: WatchItem) => `新消息：${it.latest_event?.title ?? ""}` },
  nores: { icon: Microscope, tone: "text-amber", text: () => "还没深研过 —— 去看看它值不值得研究" },
  stale: { icon: Clock3, tone: "text-muted", text: (it: WatchItem) => `上次深研在 ${it.latest_research_asof}` },
} as const;

function TodoRow({ kind, it }: { kind: keyof typeof KIND; it: WatchItem }) {
  const k = KIND[kind];
  const Icon = k.icon;
  const to = kind === "news" && it.latest_event ? `/events/${it.latest_event.latest_id}` : `/stock/${it.code}`;
  return (
    <Link
      to={to}
      className="flex items-center gap-3 rounded-[10px] border border-line bg-card px-4 py-3 no-underline hover:border-accent"
    >
      <Icon size={16} className={k.tone} strokeWidth={2} />
      <span className="font-semibold">
        {it.code ? <span className="num mr-2 text-[12px] txt-faint">{it.code}</span> : null}
        {it.name}
      </span>
      <span className="min-w-0 flex-1 truncate text-[12.5px] text-muted">{k.text(it)}</span>
      <ChevronRight size={15} className="shrink-0 txt-faint" />
    </Link>
  );
}

/** 深研超过 30 天视为陈旧（纯前端口径，不作为事实断言）。 */
function isStale(asOf: string): boolean {
  const d = daysAgo(asOf);
  return d !== null && d > 30;
}
