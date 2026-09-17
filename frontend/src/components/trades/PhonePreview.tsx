"use client";

import { Chip } from "@/components/ui/Num";
import type { PreviewPosition, PreviewTrade, PreviewWatch } from "@/lib/types";

/** 一格字段：标签 + 输入/只读值（手机竖排，数字右对齐等宽）。 */
function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="m-field">
      <span className="m-field-label">{label}</span>
      {children}
    </label>
  );
}

function Row({ r, i, onPatch }: {
  r: PreviewTrade;
  i: number;
  onPatch: (i: number, p: Partial<PreviewTrade>) => void;
}) {
  return (
    <div className="m-card">
      <header className="m-card-head">
        <input className="m-check" type="checkbox" checked={r.include}
          onChange={(e) => onPatch(i, { include: e.target.checked })} />
        <b>{r.code}</b>
        <span className="m-name">{r.name}</span>
        {r.exists && <Chip tone="amber">已存在</Chip>}
      </header>
      <div className="m-grid">
        <Field label="日期">
          <input className="m-input num" value={r.date}
            onChange={(e) => onPatch(i, { date: e.target.value })} />
        </Field>
        <Field label="方向">
          <select className="m-input" value={r.side}
            onChange={(e) => onPatch(i, { side: e.target.value })}>
            <option value="buy">买入</option>
            <option value="sell">卖出</option>
          </select>
        </Field>
        <Field label="价格">
          <input className="m-input num" inputMode="decimal" value={r.price}
            onChange={(e) => onPatch(i, { price: e.target.value })} />
        </Field>
        <Field label="数量">
          <input className="m-input num" inputMode="numeric" value={r.qty}
            onChange={(e) => onPatch(i, { qty: e.target.value })} />
        </Field>
      </div>
      <div className="m-readonly">
        金额 <b className="num">{r.amount}</b>
      </div>
    </div>
  );
}

function Pos({ r, i, onPatch }: {
  r: PreviewPosition;
  i: number;
  onPatch: (i: number, p: Partial<PreviewPosition>) => void;
}) {
  return (
    <div className="m-card">
      <header className="m-card-head">
        <input className="m-check" type="checkbox" checked={r.include}
          onChange={(e) => onPatch(i, { include: e.target.checked })} />
        <b>{r.code}</b>
        <span className="m-name">{r.name}</span>
      </header>
      <div className="m-grid">
        <Field label="数量">
          <input className="m-input num" inputMode="numeric" value={r.qty}
            onChange={(e) => onPatch(i, { qty: e.target.value })} />
        </Field>
        <Field label="成本">
          <input className="m-input num" inputMode="decimal" value={r.cost_price}
            onChange={(e) => onPatch(i, { cost_price: e.target.value })} />
        </Field>
        <Field label="现价">
          <input className="m-input num" inputMode="decimal" value={r.price}
            onChange={(e) => onPatch(i, { price: e.target.value })} />
        </Field>
        <Field label="市值">
          <input className="m-input num" inputMode="decimal" value={r.market_value}
            onChange={(e) => onPatch(i, { market_value: e.target.value })} />
        </Field>
        <Field label="盈亏">
          <input className="m-input num" inputMode="decimal" value={r.pl}
            onChange={(e) => onPatch(i, { pl: e.target.value })} />
        </Field>
      </div>
    </div>
  );
}

function Watch({ r, i, onPatch }: {
  r: PreviewWatch;
  i: number;
  onPatch: (i: number, p: Partial<PreviewWatch>) => void;
}) {
  return (
    <div className="m-card">
      <header className="m-card-head">
        {r.change === "keep" ? (
          <span className="m-check" aria-hidden="true" />
        ) : (
          <input className="m-check" type="checkbox" checked={r.include}
            onChange={(e) => onPatch(i, { include: e.target.checked })} />
        )}
        <b>{r.code}</b>
        <span className="m-name">{r.name}</span>
        {r.change === "add" && <Chip tone="up">加入自选</Chip>}
        {r.change === "keep" && <Chip tone="dim">已在自选</Chip>}
        {r.change === "remove" && <Chip tone="amber">移出自选</Chip>}
      </header>
    </div>
  );
}

/** 手机端卡片式预览：每个成交/持仓一张卡，逐行可编辑 + 勾选。
 *  `keep` 的自选行无可编辑项（后端对其不做任何事），如实只展示。 */
export default function PhonePreview({
  trades,
  positions,
  watch,
  onPatchTrade,
  onPatchPos,
  onPatchWatch,
}: {
  trades: PreviewTrade[];
  positions: PreviewPosition[];
  watch: PreviewWatch[];
  onPatchTrade: (i: number, p: Partial<PreviewTrade>) => void;
  onPatchPos: (i: number, p: Partial<PreviewPosition>) => void;
  onPatchWatch: (i: number, p: Partial<PreviewWatch>) => void;
}) {
  return (
    <div className="m-cards">
      {trades.map((r, i) => <Row key={`t${i}`} r={r} i={i} onPatch={onPatchTrade} />)}
      {positions.map((r, i) => <Pos key={`p${i}`} r={r} i={i} onPatch={onPatchPos} />)}
      {watch.map((r, i) => <Watch key={`w${i}`} r={r} i={i} onPatch={onPatchWatch} />)}
    </div>
  );
}
