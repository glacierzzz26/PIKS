"use client";

import type { PreviewPosition, PreviewAccount } from "@/lib/types";
import { PREVIEW_INPUT } from "@/components/trades/previewInput";

/** 账户汇总预览行（issue #19）：可改。留空 = 截图没这项，确认时落 NULL 而非 0。 */
function AccountPreview({
  account,
  onPatch,
}: {
  account: PreviewAccount;
  onPatch: (p: Partial<PreviewAccount>) => void;
}) {
  const fields: { key: keyof PreviewAccount; label: string }[] = [
    { key: "total_asset", label: "总资产" },
    { key: "total_mv", label: "总市值" },
    { key: "float_pl", label: "浮动盈亏" },
    { key: "daily_pl", label: "当日参考盈亏" },
  ];
  return (
    <div className="border-b border-line bg-card px-4 py-3">
      <div className="mb-2 text-xs text-muted">
        账户汇总（截图顶部原值；留空 = 截图没有，不填 0）
      </div>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        {fields.map((f) => (
          <label key={f.key} className="block">
            <span className="mb-1 block text-xs text-faint">{f.label}</span>
            <input
              value={account[f.key]}
              onChange={(e) => onPatch({ [f.key]: e.target.value })}
              className={`${PREVIEW_INPUT} num text-right`}
              inputMode="decimal"
            />
          </label>
        ))}
      </div>
    </div>
  );
}

/** 持仓截图预览：账户汇总（可改）+ 逐行可编辑 + 勾选（成本/现价/市值/盈亏可从截图修正）。 */
export default function PositionPreviewTable({
  rows,
  account,
  onPatch,
  onPatchAccount,
}: {
  rows: PreviewPosition[];
  account: PreviewAccount;
  onPatch: (i: number, p: Partial<PreviewPosition>) => void;
  onPatchAccount: (p: Partial<PreviewAccount>) => void;
}) {
  return (
    <div className="overflow-x-auto">
      <AccountPreview account={account} onPatch={onPatchAccount} />
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
