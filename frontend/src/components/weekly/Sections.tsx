"use client";

import { Link } from "react-router-dom";
import { Chip } from "@/components/ui/Num";
import type {
  WeeklySnap,
  WeeklyEvent,
  WeeklyNote,
  WeeklyTrade,
  WeeklyPosition,
} from "@/lib/types";

/** 行情快照表 */
export function SnapSection({ snaps }: { snaps: WeeklySnap[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13.5px]">
        <thead>
          <tr className="border-b border-line-strong text-xs font-semibold text-muted">
            <th className="px-4 py-2.5 text-left">日期</th>
            <th className="px-4 py-2.5 text-left">情绪</th>
            <th className="px-4 py-2.5 text-right">涨停</th>
            <th className="px-4 py-2.5 text-right">跌停</th>
            <th className="px-4 py-2.5 text-right">成交</th>
            <th className="px-4 py-2.5 text-left">我的判断</th>
          </tr>
        </thead>
        <tbody>
          {snaps.map((s) => (
            <tr key={s.date} className="h-[46px] border-b border-line last:border-0">
              <td className="num px-4">{s.date}</td>
              <td className="px-4">
                <span className="flex items-center gap-1.5">
                  <span className="text-[13px]">{s.emotion}</span>
                  <span className="text-xs text-muted">{s.weekday}</span>
                </span>
              </td>
              <td className="num px-4 text-up">{s.limit_up}</td>
              <td className="num px-4 text-down">{s.limit_down}</td>
              <td className="num px-4">{s.turnover}</td>
              <td className="max-w-[260px] truncate px-4 text-muted">
                {s.judgment || "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** 本周事件列表 */
export function EventSection({ events }: { events: WeeklyEvent[] }) {
  return (
    <ul className="flex flex-col divide-y divide-line">
      {events.map((e) => (
        <li key={e.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
          <span className="num w-10 shrink-0 text-xs text-muted">{e.date}</span>
          <span className="flex-1 truncate">{e.title}</span>
          <Chip tone="dim">{e.event_type}</Chip>
        </li>
      ))}
    </ul>
  );
}

/** 本周沉淀笔记列表 */
export function NoteSection({ notes }: { notes: WeeklyNote[] }) {
  return (
    <ul className="flex flex-col divide-y divide-line">
      {notes.map((n) => (
        <li key={n.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
          <Link
            to={`/notes/${n.id}`}
            className="flex-1 truncate text-accent no-underline hover:underline"
          >
            {n.title}
          </Link>
          <Chip tone="dim">{n.type_label}</Chip>
          <span className="num text-xs text-muted">{n.updated}</span>
        </li>
      ))}
    </ul>
  );
}

/** 本周交易表 */
export function TradeSection({ trades }: { trades: WeeklyTrade[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13.5px]">
        <thead>
          <tr className="border-b border-line-strong text-xs font-semibold text-muted">
            <th className="px-4 py-2.5 text-left">日期</th>
            <th className="px-4 py-2.5 text-left">标的</th>
            <th className="px-4 py-2.5 text-left">方向</th>
            <th className="px-4 py-2.5 text-right">价格</th>
            <th className="px-4 py-2.5 text-right">数量</th>
            <th className="px-4 py-2.5 text-right">金额</th>
          </tr>
        </thead>
        <tbody>
          {trades.map((t, i) => (
            <tr key={i} className="h-[46px] border-b border-line last:border-0">
              <td className="num px-4">{t.date}</td>
              <td className="px-4">
                <span className="num chip-dim rounded px-1.5 py-0.5 text-xs text-muted">
                  {t.code}
                </span>
                <span className="ml-2 font-semibold">{t.name}</span>
              </td>
              <td className="px-4">
                <Chip tone={t.side === "buy" ? "up" : "down"}>{t.side_label}</Chip>
              </td>
              <td className="num px-4">{t.price.toFixed(3)}</td>
              <td className="num px-4">{t.qty}</td>
              <td className="num px-4">{t.amount.toLocaleString("zh-CN")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** 周末持仓快照表 */
export function PositionSection({ positions }: { positions: WeeklyPosition[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse text-[13.5px]">
        <thead>
          <tr className="border-b border-line-strong text-xs font-semibold text-muted">
            <th className="px-4 py-2.5 text-left">标的</th>
            <th className="px-4 py-2.5 text-right">数量</th>
            <th className="px-4 py-2.5 text-right">成本</th>
            <th className="px-4 py-2.5 text-right">现价</th>
            <th className="px-4 py-2.5 text-right">市值</th>
            <th className="px-4 py-2.5 text-right">盈亏</th>
          </tr>
        </thead>
        <tbody>
          {positions.map((p) => (
            <tr key={p.code} className="h-[46px] border-b border-line last:border-0">
              <td className="px-4">
                <span className="num chip-dim rounded px-1.5 py-0.5 text-xs text-muted">
                  {p.code}
                </span>
                <span className="ml-2 font-semibold">{p.name}</span>
              </td>
              <td className="num px-4">{p.qty}</td>
              <td className="num px-4">{p.cost}</td>
              <td className="num px-4">{p.price}</td>
              <td className="num px-4">{p.mv}</td>
              <td className="num px-4 font-bold">
                <span className={p.pl.startsWith("+") ? "text-up" : p.pl.startsWith("-") ? "text-down" : "text-muted"}>
                  {p.pl}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
