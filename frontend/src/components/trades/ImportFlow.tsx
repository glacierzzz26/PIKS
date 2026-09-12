"use client";

import { useRef, useState } from "react";
import { Upload, X } from "lucide-react";
import { apiPost, apiUpload, ENDPOINTS } from "@/lib/api";
import { Chip } from "@/components/ui/Num";
import type { ImportPreview, PreviewPosition, PreviewTrade } from "@/lib/types";

const INPUT =
  "h-8 w-full min-w-0 rounded-[9px] border border-line bg-card px-2 text-sm text-ink outline-none focus:border-accent";

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
    <div className="panel panel-pad flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="mb-0 text-[15px] font-bold tracking-wide">截图导入</h2>
        <span className="text-xs text-faint">
          同花顺今日交易 / 持仓截图，AI 视觉识别后预览确认
        </span>
        {msg && (
          <span className="text-xs" style={{ color: "var(--green)" }}>
            {msg}
          </span>
        )}
        {err && (
          <span className="text-xs" style={{ color: "var(--red)" }}>
            {err}
          </span>
        )}
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
              className={`chip-btn ${kind === o.k ? "on" : ""}`}
            >
              {o.label}
            </button>
          ))}
          <label className="inline-flex h-8 cursor-pointer items-center gap-1.5 rounded-[9px] bg-accent px-3 text-xs font-semibold text-white hover:opacity-90">
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
            <span className="inline-flex items-center gap-1 text-xs text-faint">
              {file.name}
              <button
                onClick={() => setFile(null)}
                style={{ color: "var(--ink-faint)" }}
              >
                <X size={11} />
              </button>
            </span>
          )}
          <button
            onClick={upload}
            disabled={busy || !kind || !file}
            className="inline-flex h-8 items-center rounded-[9px] border border-line bg-card px-3 text-xs text-muted hover:text-accent disabled:opacity-40"
          >
            {busy ? "识别中…" : "开始识别"}
          </button>
        </div>
      )}

      {preview && (
        <>
          {preview.kind === "position" ? (
            <div className="overflow-x-auto">
              <table className="table">
                <thead>
                  <tr>
                    <th style={{ textAlign: "left" }}>选</th>
                    <th style={{ textAlign: "left" }}>代码</th>
                    <th style={{ textAlign: "left" }}>名称</th>
                    <th>数量</th>
                    <th>成本</th>
                    <th>现价</th>
                    <th>市值</th>
                    <th>盈亏</th>
                  </tr>
                </thead>
                <tbody>
                  {preview.positions.map((r, i) => (
                    <tr key={i}>
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
                  {preview.trades.map((r, i) => (
                    <tr key={i}>
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
                      <td className="num-t" style={{ color: "var(--ink-faint)" }}>
                        {r.amount}
                      </td>
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
              className="inline-flex h-8 items-center gap-1.5 rounded-[9px] bg-accent px-3 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
            >
              {busy ? "确认中…" : "确认导入"}
            </button>
            <button
              onClick={reset}
              className="inline-flex h-8 items-center rounded-[9px] border border-line bg-card px-3 text-xs text-muted hover:text-up"
            >
              取消
            </button>
          </div>
        </>
      )}
    </div>
  );
}
