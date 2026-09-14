"use client";

import { Fragment, useState } from "react";
import { Link } from "react-router-dom";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Chip } from "@/components/ui/Num";
import TradeReview from "@/components/trades/TradeReview";
import { DecisionRefList } from "@/components/trades/DecisionRefs";
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
      <table className="table">
        <thead>
          <tr>
            <th className="text-left">日期</th>
            <th className="text-left">标的</th>
            <th className="text-left">方向</th>
            <th>价格</th>
            <th>数量</th>
            <th>金额</th>
            <th className="text-left">来源</th>
            <th className="text-left">备注</th>
            <th className="text-right" >解读</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((t) => (
            <Fragment key={t.id}>
              <tr>
                <td className="num-t">{t.date}</td>
                <td>
                  <Link to={`/stock/${t.code}`} className="chip no-underline hover:border-accent">
                    {t.code}
                  </Link>
                  <span className="ml-2 font-semibold">{t.name}</span>
                </td>
                <td>
                  <Chip tone={t.side === "buy" ? "up" : "down"}>
                    {t.side === "buy" ? "买入" : "卖出"}
                  </Chip>
                </td>
                <td className="num-t">{t.price.toFixed(2)}</td>
                <td className="num-t">{t.qty}</td>
                <td className="num-t">{t.amount.toLocaleString()}</td>
                <td className="text-[12.5px] txt-faint text-left">
                  {t.source === "screenshot" ? "截图识别" : "手动"}
                </td>
                <td className="max-w-[160px] truncate text-[12.5px] txt-faint text-left">
                  {t.note || "—"}
                </td>
                <td className="text-right">
                  <button
                    onClick={() => setOpenId(openId === t.id ? null : t.id)}
                    className="inline-flex h-7 w-7 items-center justify-center rounded-[10px] border border-line bg-card text-muted hover:text-accent"
                    title="AI 解读"
                  >
                    {openId === t.id ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                  </button>
                </td>
              </tr>
              {openId === t.id && (
                <tr>
                  <td colSpan={9} className="bg-card-soft px-4 py-3">
                    {(t.based_on?.length ?? 0) > 0 && (
                      <div className="mb-3 flex items-center gap-2">
                        <span className="shrink-0 text-[12.5px] font-semibold text-muted">
                          当时在看什么
                        </span>
                        <DecisionRefList refs={t.based_on ?? []} />
                      </div>
                    )}
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
