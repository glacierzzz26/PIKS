"use client";

import { Suspense } from "react";
import { Link } from "react-router-dom";
import { Newspaper, ScrollText, Zap } from "lucide-react";
import { useUrlState } from "@/hooks/useUrlState";
import { LoadingBlock } from "@/components/ui/States";
import EventsTab from "@/pages/events";
import FlashesTab from "@/pages/flashes";
import AnnouncementsTab from "@/pages/announcements";

const TABS = [
  { key: "important", label: "重要消息", icon: Newspaper, hint: "AI 抽取的结构化事实" },
  { key: "flash", label: "快讯", icon: Zap, hint: "东财 7×24 原始流" },
  // 公告（issue #50）：官方披露的原始流，不经 AI 抽取，故与「快讯」并列而**非**其子集。
  { key: "announcement", label: "公告", icon: ScrollText, hint: "交易所披露原文（巨潮）" },
] as const;

/**
 * 消息：重要消息（= 结构化事件，含置信度/影响实体）+ 快讯（原始流）
 * + 公告（官方披露原文）三 tab。
 * 原来分属 `/events` 与 `/flashes` 两页；两者同源（快讯 → 事件），合为一页后
 * 侧栏少一项。tab 状态写 URL query（规范第 7 条）。路由 `/flashes` 保留并落在此页。
 */
export default function Page() {
  const [query, setParam] = useUrlState();
  const tab = query.tab ?? "important";
  const active = TABS.some((t) => t.key === tab) ? tab : "important";

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>消息</h1>
          <div className="psub">
            市场消息都在这儿 —— 重要消息已解读，原始快讯一字不改，公告是披露原文
          </div>
        </div>
        <div className="meta">
          <Link
            to="/help"
            className="inline-flex h-8 items-center rounded-[10px] border border-line bg-card px-3 text-xs text-muted no-underline hover:text-accent"
          >
            名词解释
          </Link>
        </div>
      </div>

      <div className="filter-bar">
        {TABS.map((t) => {
          const Icon = t.icon;
          const on = active === t.key;
          return (
            <button
              key={t.key}
              onClick={() => setParam("tab", t.key)}
              className={`chip-btn inline-flex items-center gap-1.5 ${on ? "on" : ""}`}
              title={t.hint}
            >
              <Icon size={14} />
              {t.label}
            </button>
          );
        })}
      </div>

      <Suspense fallback={<div className="panel"><LoadingBlock rows={8} /></div>}>
        {active === "flash" ? (
          <FlashesTab />
        ) : active === "announcement" ? (
          <AnnouncementsTab />
        ) : (
          <EventsTab />
        )}
      </Suspense>
    </div>
  );
}
