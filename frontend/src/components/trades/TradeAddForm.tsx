"use client";

import { useState } from "react";
import { apiPost, ENDPOINTS } from "@/lib/api";

const INPUT =
  "h-9 w-full rounded-sm border border-line bg-card px-3 text-sm outline-none focus:border-accent";

/** 手动录入一笔交易（POST /api/v1/trades，成功后回调刷新列表） */
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
      await apiPost<{ ok: boolean }>(ENDPOINTS.trades, {
        name: f.name.trim(),
        code: f.code.trim(),
        side: f.side,
        price: Number(f.price),
        qty: Number(f.qty),
        trade_date: f.trade_date || undefined,
        note: f.note.trim(),
      });
      setF((v) => ({ ...v, name: "", code: "", price: "", qty: "", note: "" }));
      setMsg("已录入");
      onDone();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col gap-3 rounded border border-line bg-card p-4 shadow-card">
      <div className="flex items-center gap-2">
        <h2 className="card-title mb-0">手动录入</h2>
        {msg && <span className={`text-xs ${msg === "已录入" ? "text-down" : "text-up"}`}>{msg}</span>}
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
          className="h-9 rounded-sm bg-accent px-4 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          {saving ? "录入中…" : "录入"}
        </button>
      </div>
    </div>
  );
}
