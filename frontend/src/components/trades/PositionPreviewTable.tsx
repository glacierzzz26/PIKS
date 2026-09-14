"use client";

import type { PreviewPosition } from "@/lib/types";
import { PREVIEW_INPUT } from "@/components/trades/previewInput";

/** 持仓截图预览：逐行可编辑 + 勾选（成本/现价/市值/盈亏可从截图修正）。 */
export default function PositionPreviewTable({
  rows,
  onPatch,
}: {
  rows: PreviewPosition[];
  onPatch: (i: number, p: Partial<PreviewPosition>) => void;
}) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            <th className="text-left">选</th>
            <th className="text-left">代码</th>
            <th className="text-left">名称</th>
            <th>数量</th>
            <th>成本</th>
            <th>现价</th>
            <th>市值</th>
            <th>盈亏</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr key={i}>
              <td className="px-2 py-1.5">
                <input type="checkbox" checked={r.include} onChange={(e) => onPatch(i, { include: e.target.checked })} />
              </td>
              <td className="px-1 py-1.5"><input value={r.code} onChange={(e) => onPatch(i, { code: e.target.value })} className={PREVIEW_INPUT} /></td>
              <td className="px-1 py-1.5"><input value={r.name} onChange={(e) => onPatch(i, { name: e.target.value })} className={PREVIEW_INPUT} /></td>
              <td className="px-1 py-1.5"><input value={r.qty} onChange={(e) => onPatch(i, { qty: e.target.value })} className={`${PREVIEW_INPUT} num text-right`} /></td>
              <td className="px-1 py-1.5"><input value={r.cost_price} onChange={(e) => onPatch(i, { cost_price: e.target.value })} className={`${PREVIEW_INPUT} num text-right`} /></td>
              <td className="px-1 py-1.5"><input value={r.price} onChange={(e) => onPatch(i, { price: e.target.value })} className={`${PREVIEW_INPUT} num text-right`} /></td>
              <td className="px-1 py-1.5"><input value={r.market_value} onChange={(e) => onPatch(i, { market_value: e.target.value })} className={`${PREVIEW_INPUT} num text-right`} /></td>
              <td className="px-1 py-1.5"><input value={r.pl} onChange={(e) => onPatch(i, { pl: e.target.value })} className={`${PREVIEW_INPUT} num text-right`} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
