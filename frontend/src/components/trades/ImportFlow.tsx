"use client";

import { useRef, useState } from "react";
import { Upload, X } from "lucide-react";
import { apiPost, apiUpload, ENDPOINTS } from "@/lib/api";
import { Chip } from "@/components/ui/Num";
import type { ImportPreview, PreviewPosition, PreviewTrade } from "@/lib/types";

const INPUT =
  "h-8 w-full min-w-0 rounded-sm border border-line bg-card px-2 text-sm outline-none focus:border-accent";

/** 截图导入：选类型 → 上传识别 → 预览可编辑表格(勾选) → 确认入库 */
export default function ImportFlow({ onDone }: { onDone: () => void }) {
  const [kind, setKind] = useState<"" | "trade" | "position">("");
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const upload = async () => {
    if (!kind || !file) return;
    setBusy(true);
    setErr(null);
    setPreview(null);
    setMsg(null);
    try {
      const fd = new FormData();
      fd.append("type", kind);
      fd.append("file", file);
      setPreview(await apiUpload<ImportPreview>(ENDPOINTS.tradesImport, fd));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const patch = (
    section: "trades" | "positions",
    i: number,
    p: Partial<PreviewTrade> | Partial<PreviewPosition>
  ) => {
    setPreview((prev) => {
      if (!prev) return prev;
      const arr = [...prev[section]];
      arr[i] = { ...arr[i], ...p } as PreviewTrade & PreviewPosition;
      return { ...prev, [section]: arr };
    });
  };

  const confirm = async () => {
    if (!preview) return;
    setBusy(true);
    setErr(null);
    try {
      await apiPost<{ ok: boolean }>(ENDPOINTS.tradesConfirm, preview);
      setMsg("已确认入库");
      setPreview(null);
      setFile(null);
      setKind("");
      if (inputRef.current) inputRef.current.value = "";
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const reset = () => {
    setPreview(null);
    setFile(null);
    setErr(null);
    setMsg(null);
    if (inputRef.current) inputRef.current.value = "";
  };

  return (
    <div className="flex flex-col gap-3 rounded border border-line bg-card p-4 shadow-card">
      <div className="flex items-center gap-2">
        <h2 className="card-title mb-0">截图导入</h2>
        <span className="text-xs text-muted">同花顺今日交易 / 持仓截图，AI 视觉识别后预览确认</span>
        {msg && <span className="text-xs text-down">{msg}</span>}
        {err && <span className="text-xs text-up">{err}</span>}
      </div>

      {!preview && (
        <div className="flex flex-wrap items-center gap-2">
          {(
            [
              { k: "trade", label: "今日交易" },
              { k: "position", label: "持仓" },
            ] as const
          ).map((o) => (
            <button
              key={o.k}
              onClick={() => setKind(o.k)}
              className={`chip ${kind === o.k ? "chip-accent" : "chip-dim"}`}
            >
              {o.label}
            </button>
          ))}
          <label className="inline-flex h-8 items-center gap-1.5 rounded-sm bg-accent px-3 text-xs font-medium text-white hover:opacity-90">
            <Upload size={12} />
            选择截图
            <input
              ref={inputRef}
              type="file"
              accept="image/png,image/jpeg,image/webp,image/gif"
              className="hidden"
              onChange={(e) => {
                setFile(e.target.files?.[0] ?? null);
                setErr(null);
              }}
            />
          </label>
          {file && (
            <span className="inline-flex items-center gap-1 text-xs text-muted">
              {file.name}
              <button onClick={() => setFile(null)} className="hover:text-up">
                <X size={11} />
              </button>
            </span>
          )}
          <button
            onClick={upload}
            disabled={busy || !kind || !file}
            className="inline-flex h-8 items-center rounded-sm border border-line bg-card px-3 text-xs text-muted hover:text-accent disabled:opacity-40"
          >
            {busy ? "识别中…" : "开始识别"}
          </button>
        </div>
      )}

      {preview && (
        <>
          {preview.kind === "position" ? (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr className="border-b border-line-strong text-xs font-semibold text-muted">
                    <th className="px-2 py-2 text-left">选</th>
                    <th className="px-2 py-2 text-left">代码</th>
                    <th className="px-2 py-2 text-left">名称</th>
                    <th className="px-2 py-2 text-right">数量</th>
                    <th className="px-2 py-2 text-right">成本</th>
                    <th className="px-2 py-2 text-right">现价</th>
                    <th className="px-2 py-2 text-right">市值</th>
                    <th className="px-2 py-2 text-right">盈亏</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.positions.map((r, i) => (
                    <tr key={i} className="border-b border-line last:border-0">
                      <td className="px-2 py-1.5">
                        <input type="checkbox" checked={r.include} onChange={(e) => patch("positions", i, { include: e.target.checked })} />
                      </td>
                      <td className="px-1 py-1.5"><input value={r.code} onChange={(e) => patch("positions", i, { code: e.target.value })} className={INPUT} /></td>
                      <td className="px-1 py-1.5"><input value={r.name} onChange={(e) => patch("positions", i, { name: e.target.value })} className={INPUT} /></td>
                      <td className="px-1 py-1.5"><input value={r.qty} onChange={(e) => patch("positions", i, { qty: e.target.value })} className={`${INPUT} num text-right`} /></td>
                      <td className="px-1 py-1.5"><input value={r.cost_price} onChange={(e) => patch("positions", i, { cost_price: e.target.value })} className={`${INPUT} num text-right`} /></td>
                      <td className="px-1 py-1.5"><input value={r.price} onChange={(e) => patch("positions", i, { price: e.target.value })} className={`${INPUT} num text-right`} /></td>
                      <td className="px-1 py-1.5"><input value={r.market_value} onChange={(e) => patch("positions", i, { market_value: e.target.value })} className={`${INPUT} num text-right`} /></td>
                      <td className="px-1 py-1.5"><input value={r.pl} onChange={(e) => patch("positions", i, { pl: e.target.value })} className={`${INPUT} num text-right`} /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[13px]">
                <thead>
                  <tr className="border-b border-line-strong text-xs font-semibold text-muted">
                    <th className="px-2 py-2 text-left">选</th>
                    <th className="px-2 py-2 text-left">日期</th>
                    <th className="px-2 py-2 text-left">代码</th>
                    <th className="px-2 py-2 text-left">名称</th>
                    <th className="px-2 py-2 text-left">方向</th>
                    <th className="px-2 py-2 text-right">价格</th>
                    <th className="px-2 py-2 text-right">数量</th>
                    <th className="px-2 py-2 text-right">金额</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.trades.map((r, i) => (
                    <tr key={i} className="border-b border-line last:border-0">
                      <td className="px-2 py-1.5">
                        <input type="checkbox" checked={r.include} onChange={(e) => patch("trades", i, { include: e.target.checked })} />
                      </td>
                      <td className="px-1 py-1.5"><input value={r.date} onChange={(e) => patch("trades", i, { date: e.target.value })} className={`${INPUT} num`} /></td>
                      <td className="px-1 py-1.5"><input value={r.code} onChange={(e) => patch("trades", i, { code: e.target.value })} className={INPUT} /></td>
                      <td className="px-1 py-1.5">
                        <span className="flex items-center gap-1">
                          <input value={r.name} onChange={(e) => patch("trades", i, { name: e.target.value })} className={INPUT} />
                          {r.exists && <Chip tone="amber">已存在</Chip>}
                        </span>
                      </td>
                      <td className="px-1 py-1.5">
                        <select value={r.side} onChange={(e) => patch("trades", i, { side: e.target.value })} className={INPUT}>
                          <option value="buy">买入</option>
                          <option value="sell">卖出</option>
                        </select>
                      </td>
                      <td className="px-1 py-1.5"><input value={r.price} onChange={(e) => patch("trades", i, { price: e.target.value })} className={`${INPUT} num text-right`} /></td>
                      <td className="px-1 py-1.5"><input value={r.qty} onChange={(e) => patch("trades", i, { qty: e.target.value })} className={`${INPUT} num text-right`} /></td>
                      <td className="num px-2 py-1.5 text-right text-muted">{r.amount}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="flex items-center gap-2">
            <button
              onClick={confirm}
              disabled={busy}
              className="inline-flex h-8 items-center gap-1.5 rounded-sm bg-accent px-3 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
            >
              {busy ? "确认中…" : "确认导入"}
            </button>
            <button
              onClick={reset}
              className="inline-flex h-8 items-center rounded-sm border border-line bg-card px-3 text-xs text-muted hover:text-up"
            >
              取消
            </button>
          </div>
        </>
      )}
    </div>
  );
}
