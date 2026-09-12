"use client";

import { Link } from "react-router-dom";
import { Num } from "@/components/ui/Num";
import type { SnapRow } from "@/lib/types";

/** 历史情绪快照卡（对齐 HTML .snap） */
export function SnapCard({ s, latest }: { s: SnapRow; latest?: boolean }) {
  const tone =
    s.emotion_score >= 60 ? "st-up" : s.emotion_score >= 45 ? "st-amber" : "st-down";
  return (
    <div className="rounded-[12px] border border-line bg-soft p-3">
      <div className="mb-2.5 flex items-center gap-2">
        <span className="num text-[15px] font-bold">{s.date}</span>
        {latest && (
          <span className="rounded-full bg-accent px-2 py-px text-[10px] font-bold text-accent-ink">
            最新
          </span>
        )}
        <span className={`st ${tone} ml-auto`}>{s.emotion_state}</span>
      </div>
      <div className="grid grid-cols-4 gap-1.5">
        {[
          { label: "涨停", v: s.limit_up, cls: "text-up" },
          { label: "跌停", v: s.limit_down, cls: "text-down" },
          { label: "炸板", v: s.broken_limit, cls: "text-amber" },
          { label: "最高板", v: s.max_board, cls: "" },
        ].map((m) => (
          <div key={m.label} className="text-center">
            <b className={`num block text-[18px] font-extrabold ${m.cls}`}>{m.v}</b>
            <span className="text-[11px] text-faint">{m.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

/** 知识库规模统计（对齐 HTML .kpi） */
export function StatCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="kpi">
      <div className="label">{label}</div>
      <div className="val">{value.toLocaleString("zh-CN")}</div>
    </div>
  );
}

/** 横向分布条（对齐 HTML .expo-*） */
export function Bars({ data }: { data: { name: string; count: number }[] }) {
  const max = Math.max(1, ...data.map((d) => d.count));
  return (
    <div>
      {data.map((d, i) => (
        <div key={d.name} className="expo-row">
          <div className="expo-top">
            <span className="en">{d.name}</span>
            <span className="ev">{d.count} 家</span>
          </div>
          <div className="expo-track">
            <i
              className={i === 0 ? "lead" : "ok"}
              style={{ width: `${(d.count / max) * 100}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

/** 高置信事件排行（对齐 HTML .table 事件行 + .rank） */
export function EventRank({
  items,
}: {
  items: { title: string; score: number; id: string }[];
}) {
  return (
    <table className="table">
      <thead>
        <tr>
          <th style={{ width: 40 }}>#</th>
          <th style={{ textAlign: "left" }}>事件标题</th>
          <th>置信度</th>
        </tr>
      </thead>
      <tbody>
        {items.map((e, i) => (
          <tr key={e.id}>
            <td>
              <span
                className={`inline-flex h-5 w-5 items-center justify-center rounded-[6px] text-[11.5px] font-bold ${
                  i < 2 ? "bg-accent-soft text-accent" : "bg-bg-soft text-faint"
                }`}
              >
                {i + 1}
              </span>
            </td>
            <td style={{ textAlign: "left" }}>
              <Link
                to={`/events?q=${encodeURIComponent(e.title.slice(0, 8))}`}
                className="ev-title no-underline hover:text-accent"
              >
                <b>{e.title}</b>
              </Link>
            </td>
            <td>
              <div className="pct-wrap">
                <div className="pct-bar">
                  <i style={{ width: `${e.score * 100}%` }} />
                </div>
                <span className="pct-num">{e.score.toFixed(2)}</span>
              </div>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
