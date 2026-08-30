"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { DashboardData } from "@/lib/types";
import { fmtYi } from "@/lib/format";
import MarkdownBody from "@/components/md/MarkdownBody";
import { SnapCard, StatCard, Bars, EventRank } from "@/components/dashboard/cards";
import { LoadingBlock, ErrorState } from "@/components/ui/States";

/** 看板（首页）：知识库规模 + 最新市场快照 + 历史情绪 + 每日复盘 + 管线状态 + 热点事件 */
export default function Page() {
  const dash = useData<DashboardData>({
    path: ENDPOINTS.dashboard,
  });

  if (dash.loading) {
    return <div className="mt-6"><LoadingBlock rows={8} /></div>;
  }
  if (dash.error || !dash.data) {
    return (
      <div className="mt-6 rounded border border-line bg-card shadow-card">
        <ErrorState msg={dash.error ?? "暂无数据"} />

      </div>
    );
  }
  const data = dash.data;
  const market = data.market;
  const top = data.top_events;

  return (
    <div>
      <div className="mb-4 mt-5">
        <h1 className="mb-0.5 text-2xl font-bold tracking-wide">看板</h1>
        <p className="text-[13px] text-muted">
          知识库概览与每日市场状态 · 数据由管线自动生成
        </p>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-3.5 lg:grid-cols-4">
        {data.stats.map((s) => (
          <StatCard key={s.label} {...s} />
        ))}
      </div>

      <div className="mt-5 grid grid-cols-1 gap-3.5 xl:grid-cols-12">
        <div className="rounded border border-line bg-card p-[18px] shadow-card xl:col-span-7">
          <h2 className="card-title">
            最新市场快照
            <span className="num text-xs font-normal text-muted">
              {market.trade_date}
            </span>
          </h2>
          <div className="mb-3 flex flex-wrap items-baseline gap-x-5 gap-y-1">
            {market.indices.map((i) => (
              <span key={i.name} className="text-[13px]">
                <span className="text-muted">{i.name} </span>
                <span
                  className={`num font-bold ${i.change_pct > 0 ? "text-up" : "text-down"}`}
                >
                  {i.close.toLocaleString("zh-CN", { minimumFractionDigits: 2 })}
                  <span className="ml-1 text-xs">
                    {i.change_pct > 0 ? "+" : ""}
                    {i.change_pct.toFixed(2)}%
                  </span>
                </span>
              </span>
            ))}
          </div>
          <div className="grid grid-cols-4 gap-1.5">
            <SnapCard
              latest
              s={{
                date: market.trade_date,
                emotion_score: market.emotion_score,
                emotion_state: market.emotion_state,
                limit_up: market.limit_up,
                limit_down: market.limit_down,
                broken_limit: market.broken_limit,
                max_board: market.max_board,
              }}
            />
          </div>
          <div className="mt-3 flex gap-4 border-t border-line pt-3 text-[13px] text-muted">
            <span>
              两市成交{" "}
              <b className="num text-ink">{fmtYi(market.turnover_yi)}</b>
            </span>
            <span>
              情绪分{" "}
              <b className="num text-accent">{market.emotion_score}</b> ·{" "}
              {market.emotion_state}
            </span>
          </div>
        </div>

        <div className="rounded border border-line bg-card p-[18px] shadow-card xl:col-span-5">
          <h2 className="card-title">近 5 日情绪</h2>
          <div className="snapgrid flex flex-col gap-2.5">
            {data.snap_history.map((s, i) => (
              <SnapCard key={s.date} s={s} latest={i === 0} />
            ))}
          </div>
        </div>

        <div className="rounded border border-line bg-card p-[18px] shadow-card xl:col-span-7">
          <h2 className="card-title">每日复盘 · {market.trade_date}</h2>
          <MarkdownBody content={data.review} />
        </div>

        <div className="flex flex-col gap-3.5 xl:col-span-5">
          <div className="rounded border border-line bg-card p-[18px] shadow-card">
            <h2 className="card-title">行业涨停分布</h2>
            <Bars data={market.industry_dist.slice(0, 8)} />
          </div>
          <div className="rounded border border-line bg-card p-[18px] shadow-card">
            <h2 className="card-title">高置信事件</h2>
            <EventRank items={top} />
          </div>
          <div className="rounded border border-line bg-card p-[18px] shadow-card">
            <h2 className="card-title">管线状态</h2>
            <ul className="flex flex-col gap-1.5">
              {data.task_runs.slice(0, 6).map((t) => (
                <li
                  key={t.command}
                  className="flex items-center gap-2.5 text-[13px]"
                >
                  <span
                    className={`h-1.5 w-1.5 rounded-full ${
                      t.status === "ok"
                        ? "bg-down"
                        : t.status === "failed"
                          ? "bg-up"
                          : "bg-amber"
                    }`}
                  />
                  <span className="font-mono text-xs">{t.command}</span>
                  {t.note && (
                    <span className="flex-1 truncate text-xs text-muted">
                      {t.note}
                    </span>
                  )}
                  <span className="num ml-auto text-xs text-muted">
                    {t.time}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
