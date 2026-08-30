"use client";

import { Fragment, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Chip } from "@/components/ui/Num";
import TradeReview from "@/components/trades/TradeReview";
import type { TradeRow } from "@/lib/types";

/** 成交记录表：行展开 → AI 解读 / 复盘点存为笔记 */
export default function TradeTable({
  rows,
  refresh,
}: {
  rows: TradeRow[];
  refresh: () => void;
}) {
  const [openId, setOpenId] = useState<string | null>(null);

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
            <th className="px-4 py-2.5 text-left">来源</th>
            <th className="px-4 py-2.5 text-left">备注</th>
            <th className="px-4 py-2.5 text-right">解读</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((t) => (
            <Fragment key={t.id}>
              <tr className="h-[46px] border-b border-line">
                <td className="num px-4">{t.date}</td>
                <td className="px-4">
                  <span className="num chip-dim rounded px-1.5 py-0.5 text-xs text-muted">
                    {t.code}
                  </span>
                  <span className="ml-2 font-semibold">{t.name}</span>
                </td>
                <td className="px-4">
                  <Chip tone={t.side === "buy" ? "up" : "down"}>
                    {t.side === "buy" ? "买入" : "卖出"}
                  </Chip>
                </td>
                <td className="num px-4">{t.price.toFixed(2)}</td>
                <td className="num px-4">{t.qty}</td>
                <td className="num px-4">{t.amount.toLocaleString()}</td>
                <td className="px-4 text-muted">
                  {t.source === "screenshot" ? "截图识别" : "手动"}
                </td>
                <td className="max-w-[160px] truncate px-4 text-muted">
                  {t.note || "—"}
                </td>
                <td className="px-4 text-right">
                  <button
                    onClick={() => setOpenId(openId === t.id ? null : t.id)}
                    className="inline-flex h-7 w-7 items-center justify-center rounded-sm border border-line bg-card text-muted hover:text-accent"
                    title="AI 解读"
                  >
                    {openId === t.id ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                  </button>
                </td>
              </tr>
              {openId === t.id && (
                <tr className="border-b border-line bg-card-soft">
                  <td colSpan={9} className="px-4 py-3">
                    <TradeReview trade={t} refresh={refresh} />
                  </td>
                </tr>
              )}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}
