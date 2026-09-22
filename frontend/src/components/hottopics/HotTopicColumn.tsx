"use client";

import { Flame } from "lucide-react";
import { HotTopicRow } from "./HotTopicRow";
import type { HotTopicSource } from "@/lib/types";

/**
 * 热榜一列（**一个源**）。两源各自独立成列，**不合并、不加权、不排名**。
 *
 * 为什么必须分列（设计 §6 方案 A）：两源粒度不同（同花顺=题材/事件，
 * 财联社=文章/复盘），实测同题对=0。把「沪指缩量反弹」和「华字辈大涨」并排排名
 * 没有可比性，必然误导 —— 故结构上就不给混排的机会。
 */
export function HotTopicColumn({ source }: { source: HotTopicSource }) {
  return (
    <section className="panel hottopic-col">
      <header className="hottopic-head">
        <div className="inline-flex items-center gap-1.5">
          <Flame size={14} className="text-up" strokeWidth={1.8} />
          <h2>{source.name}</h2>
          <span className="st st-dim num">{source.items.length}</span>
        </div>
        <p className="note">{source.note}</p>
      </header>
      {source.items.length === 0 ? (
        <p className="empty">该榜暂无数据</p>
      ) : (
        <div className="hottopic-list">
          {source.items.map((it) => (
            <HotTopicRow key={`${source.key}-${it.rank}`} item={it} />
          ))}
        </div>
      )}
    </section>
  );
}
