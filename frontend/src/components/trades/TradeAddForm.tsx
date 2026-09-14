"use client";

import { useState } from "react";
import { apiPost, ENDPOINTS } from "@/lib/api";
import TradeBasedOnPicker, { EMPTY_BASED_ON } from "@/components/trades/TradeBasedOnPicker";
import type { TradeBasedOn } from "@/lib/types";

const INPUT = "input";

/** 手动录入一笔交易（POST /api/v1/trades，成功后回调刷新列表）。
 *  P6-4：可选「当时在看什么」——把买入决策关联到研报/消息/笔记，闭环「我为什么买它」。 */
export default function TradeAddForm({ onDone }: { onDone: () => void }) {
  const [f, setF] = useState({
    name: "",
    code: "",
    side: "buy",
    price: "",
    qty: "",
    trade_date: new Date().toISOString().slice(0, 10),
    note: "",
  });
  const [basedOn, setBasedOn] = useState<TradeBasedOn>(EMPTY_BASED_ON);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  const set =
    (k: keyof typeof f) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
      setF((v) => ({ ...v, [k]: e.target.value }));

  const submit = async () => {
    setSaving(true);
    setMsg(null);
    try {
      await apiPost<{ ok: boolean; linked?: number }>(ENDPOINTS.trades, {
        name: f.name.trim(),
        code: f.code.trim(),
        side: f.side,
        price: Number(f.price),
        qty: Number(f.qty),
        trade_date: f.trade_date || undefined,
        note: f.note.trim(),
        based_on: basedOn,
      });
      setF((v) => ({ ...v, name: "", code: "", price: "", qty: "", note: "" }));
      setBasedOn(EMPTY_BASED_ON);
      setMsg("已录入");
      onDone();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="panel panel-pad flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <h2 className="mb-0 text-[15px] font-bold tracking-wide">手动录入</h2>
        {msg && (
          <span
            className="text-xs"
            style={{ color: msg === "已录入" ? "var(--green)" : "var(--red)" }}
          >
            {msg}
          </span>
        )}
      </div>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <input value={f.name} onChange={set("name")} placeholder="证券名称 *" className={INPUT} />
        <input value={f.code} onChange={set("code")} placeholder="代码 *" className={INPUT} />
        <select value={f.side} onChange={set("side")} className={INPUT}>
          <option value="buy">买入</option>
          <option value="sell">卖出</option>
        </select>
        <input value={f.price} onChange={set("price")} type="number" step="0.001" placeholder="价格 *" className={INPUT} />
        <input value={f.qty} onChange={set("qty")} type="number" placeholder="数量 *" className={INPUT} />
        <input value={f.trade_date} onChange={set("trade_date")} type="date" className={INPUT} />
        <input value={f.note} onChange={set("note")} placeholder="备注" className={`${INPUT} col-span-2`} />
        <button
          onClick={submit}
          disabled={saving}
          className="h-9 rounded-[10px] bg-accent px-4 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
        >
          {saving ? "录入中…" : "录入"}
        </button>
      </div>
      <TradeBasedOnPicker code={f.code.trim()} value={basedOn} onChange={setBasedOn} />
    </div>
  );
}
