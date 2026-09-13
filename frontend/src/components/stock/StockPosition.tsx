"use client";

import { Wallet } from "lucide-react";
import { Num } from "@/components/ui/Num";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import type { PositionRow, TradeRow } from "@/lib/types";

/**
 * 个股中心「我的持仓与交易」区块。
 * position 为该股最近一次持仓快照（无则未持仓）；trades 为该股全部成交。
 * 二者皆空 → 如实空态 + 引导去交易页录入（截图/手动）。
 */
export default function StockPosition({
  position,
  trades,
}: {
  position: PositionRow | null;
  trades: TradeRow[];
}) {
  if (!position && trades.length === 0) {
    return (
      <StockSectionEmpty
        tip="无持仓、无成交记录"
        action={{ to: "/trades", label: "去交易页录入", icon: Wallet }}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      {position ? (
        <div className="overflow-x-auto">
          <table className="table">
            <thead>
              <tr>
                <th style={{ textAlign: "left" }}>标的</th>
                <th>数量</th>
                <th>成本</th>
                <th>现价</th>
                <th>盈亏</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <td>
                  <span className="chip">{position.code}</span>
                  <span className="ml-2 font-semibold">{position.name}</span>
                </td>
                <td className="num-t">{position.qty}</td>
                <td className="num-t">{position.cost.toFixed(2)}</td>
                <td className="num-t">{position.last.toFixed(2)}</td>
                <td className="num-t font-bold">
                  <Num value={position.pnl_pct} suffix="%" colored />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      ) : (
        <p className="text-[13px] text-faint">当前未持仓（无持仓快照记录）</p>
      )}

      {trades.length > 0 && (
        <div className="overflow-x-auto">
          <table className="table">
            <thead>
              <tr>
                <th style={{ textAlign: "left" }}>日期</th>
                <th style={{ textAlign: "left" }}>方向</th>
                <th>价格</th>
                <th>数量</th>
                <th>金额</th>
                <th style={{ textAlign: "left" }}>来源</th>
              </tr>
            </thead>
            <tbody>
              {trades.map((t) => (
                <tr key={t.id}>
                  <td className="num-t">{t.date}</td>
                  <td>
                    <span className={`st ${t.side === "buy" ? "st-up" : "st-down"}`}>
                      {t.side === "buy" ? "买入" : "卖出"}
                    </span>
                  </td>
                  <td className="num-t">{t.price.toFixed(2)}</td>
                  <td className="num-t">{t.qty}</td>
                  <td className="num-t">{t.amount.toLocaleString()}</td>
                  <td className="text-[12.5px] text-faint" style={{ textAlign: "left" }}>
                    {t.source === "screenshot" ? "截图识别" : "手动"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
