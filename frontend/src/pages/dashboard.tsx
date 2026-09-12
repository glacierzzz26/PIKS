"use client";

import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { DashboardData } from "@/lib/types";
import MarkdownBody from "@/components/md/MarkdownBody";
import { SnapCard, StatCard, Bars, EventRank } from "@/components/dashboard/cards";
import { Gauge, EmotionSpark, Pipeline } from "@/components/dashboard/widgets";
import { MarketPanel, HeroScore } from "@/components/dashboard/panels";
import { LoadingBlock, ErrorState } from "@/components/ui/States";

/** 看板（首页）：知识库规模 + 市场快照 + 情绪走势 + 行业分布 + 高置信事件 + 管线状态 */
export default function Page() {
  const dash = useData<DashboardData>({ path: ENDPOINTS.dashboard });

  if (dash.loading) {
    return (
      <div className="panel mt-6">
        <LoadingBlock rows={8} />
      </div>
    );
  }
  if (dash.error || !dash.data) {
    return (
      <div className="panel mt-6">
        <ErrorState msg={dash.error ?? "暂无数据"} />
      </div>
    );
  }
  const { stats, market, snap_history, review, top_events, task_runs } = dash.data;

  return (
    <div>
      <header className="hero">
        <div className="hero-top">
          <div className="logo" aria-hidden="true">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M3 3v18h18" />
              <path d="M7 14l4-4 3 3 5-6" />
            </svg>
          </div>
          <div>
            <h1>个人投资知识系统 · 每日看板</h1>
            <div className="sub">
              Personal Investment Knowledge System · {market.trade_date}
            </div>
          </div>
        </div>
        <div className="hero-body">
          <div className="hero-tag">
            基于今日 <b>东财 7×24 快讯</b> 与 <b>涨停池</b> 数据，经 AI 事件抽取、语义去重聚类与实体构建，自动沉淀为结构化知识库。
            <b>Fact ≠ Inference ≠ Belief</b>，机器产出与个人判断严格分域。
          </div>
          <HeroScore
            score={market.emotion_score}
            state={market.emotion_state}
            limitUp={market.limit_up}
            limitDown={market.limit_down}
          />
        </div>
      </header>

      <section className="section">
        <div className="kpi-grid">
          {stats.map((s) => (
            <StatCard key={s.label} {...s} />
          ))}
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>今日市场快照</h2>
          <span className="hint">quote-collector · 仅交易日运行</span>
        </div>
        <div className="two-col">
          <MarketPanel market={market} />
          <div className="panel panel-pad">
            <Gauge score={market.emotion_score} state={market.emotion_state} />
            <EmotionSpark history={snap_history} />
          </div>
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>行业涨停分布 与 高置信事件</h2>
          <span className="hint">今日涨停 {market.limit_up} 家 · 去重后按行业聚合</span>
        </div>
        <div className="two-col">
          <div className="panel expo-card">
            <h3>行业 / 板块涨停分布</h3>
            <div className="note">Top 8 · 依据涨停池个股所属申万行业聚合</div>
            {market.industry_dist.length > 0 ? (
              <Bars data={market.industry_dist.slice(0, 8)} />
            ) : (
              <p className="text-[13px] text-faint">暂无行业分布数据</p>
            )}
          </div>
          <div className="panel">
            <EventRank items={top_events} />
          </div>
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>每日复盘 · {market.trade_date}</h2>
          <span className="hint">daily-review · 管线自动生成</span>
        </div>
        <div className="panel panel-pad">
          <MarkdownBody content={review} />
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>近 5 日情绪</h2>
          <span className="hint">market-state · 每日收盘后更新</span>
        </div>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
          {snap_history.slice(0, 5).map((s, i) => (
            <SnapCard key={s.date} s={s} latest={i === 0} />
          ))}
        </div>
      </section>

      <section className="section">
        <div className="section-head">
          <span className="bar" />
          <h2>管线状态</h2>
          <span className="hint">crontab 每 15 分钟自判 · 幂等可重跑</span>
        </div>
        <div className="panel">
          <Pipeline runs={task_runs.slice(0, 9)} />
        </div>
      </section>
    </div>
  );
}
