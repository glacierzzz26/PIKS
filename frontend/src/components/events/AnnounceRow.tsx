"use client";

import { useNavigate } from "react-router-dom";
import { Chip } from "@/components/ui/Num";
import { ANNOUNCE_GRADE_FALLBACK, announceGradeLabel } from "@/lib/constants";
import type { Announcement } from "@/lib/types";

/**
 * 公告行（issue #50 / #68）。时间 + 代码/名称 + 标题 + 查询原文。
 *
 * ⚠️ 只在 must/important 上显示级别 chip（issue #68）——「常规/噪音」是默认档，
 * 给每条都挂一个标签只会制造噪音；低级别靠**分组标题**体现，不逐行标。
 */
export function AnnouncementRow({ ann: a }: { ann: Announcement }) {
  const nav = useNavigate();
  const g = gradeOf(a.grade);
  const showChip = g === "must" || g === "important";
  return (
    <div className="flex gap-4 border-b border-line px-4 py-3 last:border-b-0">
      <span className="num w-10 shrink-0 pt-0.5 text-xs txt-faint">
        {a.time.slice(11, 16)}
      </span>
      <span className="w-[13rem] shrink-0 text-xs">
        {a.sec_code ? (
          <button
            className="num mr-1.5 text-accent hover:underline"
            title={`打开个股中心 ${a.sec_code}`}
            onClick={() => nav(`/stock/${a.sec_code}`)}
          >
            {a.sec_code}
          </button>
        ) : null}
        <span className="text-muted">{a.sec_name}</span>
      </span>
      <p className="flex-1 text-sm leading-relaxed">
        {showChip ? (
          <span
            className={`st mr-2 ${g === "must" ? "st-down" : "st-amber"}`}
            title="按标题规则自动判断，不是官方认定"
          >
            {announceGradeLabel(g)}
          </span>
        ) : null}
        {a.title}
      </p>
      <Chip tone="dim" href={a.url}>
        {a.url ? "查看原文" : a.source}
      </Chip>
    </div>
  );
}

/** 级别归一：空/未知一律按「常规」（与后端 gradeMatch 口径一致，历史行不消失）。 */
export function gradeOf(grade?: string): string {
  const g = (grade ?? "").trim();
  return g === "must" || g === "important" || g === "routine" || g === "noise"
    ? g
    : ANNOUNCE_GRADE_FALLBACK;
}
