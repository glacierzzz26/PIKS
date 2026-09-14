"use client";

import { Link } from "react-router-dom";
import { ShoppingCart } from "lucide-react";
import { DecisionRefList } from "@/components/trades/DecisionRefs";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import type { TradeRow } from "@/lib/types";

/**
 * 个股中心首屏「当时在看什么」（P6-4 决策记录闭环）。
 * 逐笔列出该股的买入决策及其关联的研报/消息/笔记 —— 回答「我为什么买它」。
 * 只展示有决策边的交易；一条都没有 → 如实空态，不伪造依据。
 */
export default function StockDecisions({ trades }: { trades: TradeRow[] }) {
  const withRefs = trades.filter((t) => (t.based_on?.length ?? 0) > 0);
  if (withRefs.length === 0) {
    return (
      <StockSectionEmpty
        tip="还没有记录过买入依据。下次录入交易时，可以顺手关联当时的研报 / 消息 / 笔记。"
        action={{ to: "/trades", label: "去录入交易并关联", icon: ShoppingCart }}
      />
    );
  }
  return (
    <div className="flex flex-col divide-y divide-line">
      {withRefs.map((t) => (
        <div key={t.id} className="flex flex-col gap-1.5 py-2.5 first:pt-0 last:pb-0">
          <div className="flex items-center gap-2 text-[12.5px]">
            <span className="num-t text-muted">{t.date}</span>
            <span className={`st ${t.side === "buy" ? "st-up" : "st-down"}`}>
              {t.side === "buy" ? "买入" : "卖出"}
            </span>
            {t.note && <span className="text-faint">「{t.note}」</span>}
            <Link
              to={`/trades`}
              className="ml-auto text-[12px] text-faint no-underline hover:text-accent"
            >
              交易页 →
            </Link>
          </div>
          <DecisionRefList refs={t.based_on ?? []} />
        </div>
      ))}
    </div>
  );
}
