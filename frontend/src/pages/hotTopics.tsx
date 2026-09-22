"use client";

import { Info } from "lucide-react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { HotTopics } from "@/lib/types";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import { HotTopicColumn } from "@/components/hottopics/HotTopicColumn";

/**
 * 热榜（issue #68 D 层）—— 两列**各出各的，不合并**。
 *
 * 数据源：`GET /api/v1/hot-topics`（独立表 hot_topic_items，与事件链路**零交集**）。
 *
 * 🔴 红线（设计 §6/§10，UI 须逐条兑现）：
 *   1. 热度**只作展示**，绝不作为「重要性」判定 —— 故页面显式标注「可被操纵、非重要性判定」；
 *   2. **不得与印证度合并展示** —— 故本页**不出现** source_count / 几家印证等任何事件字段；
 *   3. 两源**分列**，不许跨源合并/加权/排名（粒度不同，实测同题对=0）；
 *   4. 热度**仅源内可比**，故不提供跨源排序、不给「综合热度」。
 *
 * ⚠️ 数据可能很旧（常驻采集只在盘中跑）：如实显示 `snapshot_at`，
 * 落盘时刻旧就写旧，不假装是实时的。
 */
export default function Page() {
  const hot = useData<HotTopics>({ path: ENDPOINTS.hotTopics });
  const sources = hot.data?.sources ?? [];
  const hasAny = sources.some((s) => s.items.length > 0);

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>热榜</h1>
          <div className="psub">
            市场上大家在聊什么 —— 机器采集的话题/热文榜，按各自榜单的顺序原样呈现
          </div>
        </div>
        <div className="meta">
          {hot.data?.snapshot_at && (
            <span className="st st-dim num-t" title="最近一次采集时刻（盘中每 30 分钟）">
              数据到 {hot.data.snapshot_at}
            </span>
          )}
        </div>
      </div>

      <div className="notice-bar">
        <Info size={13} className="shrink-0" />
        <span>
          热榜**可被商业力量操纵**，只是「有多少人在看」的代理，**不是重要性判定、也不是推荐**。
          两个榜来源与口径不同、**热度只在各自榜内可比**，故分列呈现、不做合并排名。
        </span>
      </div>

      {hot.loading ? (
        <div className="panel">
          <LoadingBlock rows={8} />
        </div>
      ) : hot.error ? (
        <div className="panel">
          <ErrorState msg={hot.error} />
        </div>
      ) : !hasAny ? (
        <div className="panel">
          <EmptyState tip="暂无热榜数据 —— 采集在交易日盘中运行" />
        </div>
      ) : (
        <div className="hottopic-cols">
          {sources.map((s) => (
            <HotTopicColumn key={s.key} source={s} />
          ))}
        </div>
      )}
    </div>
  );
}
