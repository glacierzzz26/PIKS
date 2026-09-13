"use client";

import { TrendingUp } from "lucide-react";
import { StockSectionEmpty } from "@/components/stock/StockHeader";

/** 个股中心「涨停记录」区块：该股在涨停池出现的交易日期。 */
export default function StockLimitUps({ dates }: { dates: string[] }) {
  if (dates.length === 0) {
    return <StockSectionEmpty tip="暂无涨停记录" />;
  }
  return (
    <div className="flex flex-wrap gap-2 py-1">
      {dates.map((d) => (
        <span key={d} className="st st-up inline-flex items-center gap-1">
          <TrendingUp size={12} />
          <span className="num">{d}</span>
        </span>
      ))}
    </div>
  );
}
