"use client";

import { Link } from "react-router-dom";
import { Chip } from "@/components/ui/Num";
import { useEventTypes } from "@/lib/eventTypes";
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
      <table className="table">
        <thead>
          <tr>
            <th className="text-left">日期</th>
            <th className="text-left">情绪</th>
            <th>涨停</th>
            <th>跌停</th>
            <th>成交</th>
            <th className="text-left">我的判断</th>
          </tr>
        </thead>
        <tbody>
          {snaps.map((s) => (
            <tr key={s.date}>
              <td className="num-t">{s.date}</td>
              <td>
                <span className="flex items-center gap-1.5">
                  <span className="text-[13px]">{s.emotion}</span>
                  <span className="text-xs text-faint">{s.weekday}</span>
                </span>
              </td>
              <td className="num-t" style={{ color: "var(--red)" }}>
                {s.limit_up}
              </td>
              <td className="num-t" style={{ color: "var(--green)" }}>
                {s.limit_down}
              </td>
              <td className="num-t">{s.turnover}</td>
              <td className="max-w-[260px] truncate text-[12.5px] txt-faint">
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
  // 类型 label 取后端枚举（issue #61）——此处此前裸渲染 `e.event_type`，
  // 绕过所有映射层，直接把英文 key 显示给用户。
  const { labelOf } = useEventTypes();
  return (
    <ul className="flex flex-col divide-y divide-line">
      {events.map((e) => (
        <li key={e.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
          <span className="num w-10 shrink-0 text-xs text-faint">{e.date}</span>
          <span className="flex-1 truncate">{e.title}</span>
          <Chip tone="dim">{labelOf(e.event_type) ?? e.event_type}</Chip>
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
          <span className="num text-xs text-faint">{n.updated}</span>
        </li>
      ))}
    </ul>
  );
}

/** 本周交易表 */
export function TradeSection({ trades }: { trades: WeeklyTrade[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th className="text-left">日期</th>
            <th className="text-left">标的</th>
            <th className="text-left">方向</th>
            <th>价格</th>
            <th>数量</th>
            <th>金额</th>
          </tr>
        </thead>
        <tbody>
          {trades.map((t, i) => (
            <tr key={i}>
              <td className="num-t">{t.date}</td>
              <td>
                <span className="chip">{t.code}</span>
                <span className="ml-2 font-semibold">{t.name}</span>
              </td>
              <td>
                <Chip tone={t.side === "buy" ? "up" : "down"}>{t.side_label}</Chip>
              </td>
              <td className="num-t">{t.price.toFixed(3)}</td>
              <td className="num-t">{t.qty}</td>
              <td className="num-t">{t.amount.toLocaleString("zh-CN")}</td>
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
      <table className="table">
        <thead>
          <tr>
            <th className="text-left">标的</th>
            <th>数量</th>
            <th>成本</th>
            <th>现价</th>
            <th>市值</th>
            <th>盈亏</th>
          </tr>
        </thead>
        <tbody>
          {positions.map((p) => (
            <tr key={p.code}>
              <td>
                <span className="chip">{p.code}</span>
                <span className="ml-2 font-semibold">{p.name}</span>
              </td>
              <td className="num-t">{p.qty}</td>
              <td className="num-t">{p.cost}</td>
              <td className="num-t">{p.price}</td>
              <td className="num-t">{p.mv}</td>
              <td
                className={`num-t font-bold ${
                  p.pl.startsWith("+") ? "text-up" : p.pl.startsWith("-") ? "text-down" : "txt-faint"
                }`}
              >
                {p.pl}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
