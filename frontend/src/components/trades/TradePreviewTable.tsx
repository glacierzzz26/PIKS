"use client";

import { Chip } from "@/components/ui/Num";
import type { PreviewTrade } from "@/lib/types";
import { PREVIEW_INPUT } from "@/components/trades/previewInput";

/** 交易截图预览：逐行可编辑 + 勾选，重复行如实标注「已存在」。 */
export default function TradePreviewTable({
  rows,
  onPatch,
}: {
  rows: PreviewTrade[];
  onPatch: (i: number, p: Partial<PreviewTrade>) => void;
}) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th style={{ textAlign: "left" }}>选</th>
            <th style={{ textAlign: "left" }}>日期</th>
            <th style={{ textAlign: "left" }}>代码</th>
            <th style={{ textAlign: "left" }}>名称</th>
            <th style={{ textAlign: "left" }}>方向</th>
            <th>价格</th>
            <th>数量</th>
            <th>金额</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td className="px-2 py-1.5">
                <input type="checkbox" checked={r.include} onChange={(e) => onPatch(i, { include: e.target.checked })} />
              </td>
              <td className="px-1 py-1.5"><input value={r.date} onChange={(e) => onPatch(i, { date: e.target.value })} className={`${PREVIEW_INPUT} num`} /></td>
              <td className="px-1 py-1.5"><input value={r.code} onChange={(e) => onPatch(i, { code: e.target.value })} className={PREVIEW_INPUT} /></td>
              <td className="px-1 py-1.5">
                <span className="flex items-center gap-1">
                  <input value={r.name} onChange={(e) => onPatch(i, { name: e.target.value })} className={PREVIEW_INPUT} />
                  {r.exists && <Chip tone="amber">已存在</Chip>}
                </span>
              </td>
              <td className="px-1 py-1.5">
                <select value={r.side} onChange={(e) => onPatch(i, { side: e.target.value })} className={PREVIEW_INPUT}>
                  <option value="buy">买入</option>
                  <option value="sell">卖出</option>
                </select>
              </td>
              <td className="px-1 py-1.5"><input value={r.price} onChange={(e) => onPatch(i, { price: e.target.value })} className={`${PREVIEW_INPUT} num text-right`} /></td>
              <td className="px-1 py-1.5"><input value={r.qty} onChange={(e) => onPatch(i, { qty: e.target.value })} className={`${PREVIEW_INPUT} num text-right`} /></td>
              <td className="num-t" style={{ color: "var(--ink-faint)" }}>{r.amount}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
