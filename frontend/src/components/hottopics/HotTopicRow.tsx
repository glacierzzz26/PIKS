"use client";

import { ExternalLink } from "lucide-react";
import type { HotTopicItem } from "@/lib/types";

/**
 * 热榜一行：名次 + 标题 + 热度（**仅源内可比**）。
 *
 * ⚠️ 热度数值**不做千分位以外的加工**（不换算成百分比、不跨源比较）；
 * 上游未给（null）时如实留空，**不填 0** —— 0 与「没有这个数」是两回事。
 */
export function HotTopicRow({ item }: { item: HotTopicItem }) {
  return (
    <div className="hottopic-row">
      <span className="rank num">{item.rank}</span>
      {item.url ? (
        <a
          href={item.url}
          target="_blank"
          rel="noreferrer noopener"
          className="title inline-flex items-center gap-1"
          title={item.title}
        >
          {item.title}
          <ExternalLink size={12} className="shrink-0 txt-faint" />
        </a>
      ) : (
        <span className="title" title={item.title}>
          {item.title}
        </span>
      )}
      <span className="hot num-t">
        {item.hot_value === null ? "—" : item.hot_value.toLocaleString("zh-CN")}
      </span>
    </div>
  );
}
