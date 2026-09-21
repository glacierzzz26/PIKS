"use client";

import { ChevronDown, ChevronUp, Info } from "lucide-react";
import { AnnounceSearch } from "@/components/events/AnnounceSearch";
import { ANNOUNCE_GRADES } from "@/lib/constants";

/**
 * 公告分级筛选条（issue #68 A 层）。
 *
 * 默认只看**必读+重要**（实测 ~11~20%），其余折叠；「查看全部」一键放开。
 *
 * ⚠️ 红线（§3.3）：**折叠 ≠ 隐藏** —— 必须始终给出回到全量的入口，
 * 且显式告知被折叠了多少条（`hidden`）。否则规则误判一条 = 用户永远看不到。
 *
 * ⚠️ 分级是**机器判定（Inference）不是事实**，故此处如实标注「按标题规则自动判断」，
 * 不得表述为「官方认定重要」（同 P7 量价形态的标注纪律）。
 */
export function GradeFilterBar({
  q,
  setFilter,
  showAll,
  hidden,
  onToggle,
}: {
  q: string;
  setFilter: (k: string, v: string) => void;
  showAll: boolean;
  hidden: number;
  onToggle: () => void;
}) {
  return (
    <div className="filter-bar flex-wrap">
      <AnnounceSearch q={q} onSearch={(v) => setFilter("q", v)} />
      <button
        onClick={onToggle}
        className={`chip-btn inline-flex items-center gap-1.5 ${showAll ? "on" : ""}`}
        title={
          showAll
            ? "当前显示全部级别（含常规/噪音）"
            : "当前只显示必读+重要，其余已折叠"
        }
      >
        {showAll ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        {showAll ? "只看必读+重要" : `查看全部${hidden > 0 ? `（另有 ${hidden} 条）` : ""}`}
      </button>
      <span
        className="ml-1 inline-flex items-center gap-1 text-xs txt-faint"
        title={ANNOUNCE_GRADES.map((g) => `${g.label}：${g.hint}`).join("\n")}
      >
        <Info size={12} />
        级别按标题规则自动判断，非官方认定；标「噪音」的只收起、未删除
      </span>
    </div>
  );
}
