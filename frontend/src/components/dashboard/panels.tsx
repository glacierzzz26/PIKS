"use client";

import { fmtYi } from "@/lib/format";
import type { MarketSnapshot } from "@/lib/types";

/** 市场快照面板（指数表 + 涨停/跌停/炸板/最高板小卡 + 成交/情绪汇总） */
export function MarketPanel({ market }: { market: MarketSnapshot }) {
  const up = market.indices.filter((i) => i.change_pct > 0).length;
  return (
    <div className="panel">
      <table className="table">
        <thead>
          <tr>
            <th>指数</th>
            <th>收盘</th>
            <th>涨跌幅</th>
          </tr>
        </thead>
        <tbody>
          {market.indices.map((i) => (
            <tr key={i.name}>
              <td>
                <div className="idx-name">
                  <b>{i.name}</b>
                </div>
              </td>
              <td className="num-t">
                {i.close.toLocaleString("zh-CN", { minimumFractionDigits: 2 })}
              </td>
              <td>
                <span className={`chg ${i.change_pct >= 0 ? "up" : "down"}`}>
                  {i.change_pct >= 0 ? "+" : ""}
                  {i.change_pct.toFixed(2)}%
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="grid grid-cols-4 gap-2.5 px-4">
        <div className="snap">
          <b style={{ color: "var(--red)" }}>{market.limit_up}</b>
          <span>涨停</span>
        </div>
        <div className="snap">
          <b style={{ color: "var(--green)" }}>{market.limit_down}</b>
          <span>跌停</span>
        </div>
        <div className="snap">
          <b style={{ color: "var(--warn)" }}>{market.broken_limit}</b>
          <span>炸板</span>
        </div>
        <div className="snap">
          <b>{market.max_board}</b>
          <span>最高板</span>
        </div>
      </div>
      <div className="snap-foot mx-4 px-1 pb-4">
        <span>
          两市成交 <b>{fmtYi(market.turnover_yi)}</b>
        </span>
        <span>
          情绪分 <b className="hl">{market.emotion_score}</b> ·{" "}
          <b className="hl">{market.emotion_state}</b>
        </span>
        <span>
          {up}/{market.indices.length} 指数收涨
        </span>
      </div>
    </div>
  );
}

/** 英雄区右侧情绪环（大号） */
export function HeroScore({ score, state, limitUp, limitDown }: {
  score: number;
  state: string;
  limitUp: number;
  limitDown: number;
}) {
  const dashoffset = 207.3 * (1 - Math.max(0, Math.min(100, score)) / 100);
  return (
    <div className="score-badge">
      <div className="score-ring">
        <svg width="78" height="78" viewBox="0 0 78 78">
          <circle cx="39" cy="39" r="33" fill="none" stroke="rgba(255,255,255,.22)" strokeWidth="8" />
          <circle
            cx="39" cy="39" r="33" fill="none" stroke="#fff" strokeWidth="8" strokeLinecap="round"
            strokeDasharray="207.3"
            strokeDashoffset={dashoffset.toFixed(1)}
            transform="rotate(-90 39 39)"
          />
        </svg>
        <div className="num">
          <b>{score}</b>
          <span>/ 100</span>
        </div>
      </div>
      <div className="score-meta">
        <div className="lvl">市场情绪 {state}</div>
        <div className="desc">
          涨停 {limitUp} · 跌停 {limitDown}
        </div>
      </div>
    </div>
  );
}
