"use client";

import { useRef, useState } from "react";
import { apiPost, apiUpload, ENDPOINTS } from "@/lib/api";
import ImportControls from "@/components/trades/ImportControls";
import TradePreviewTable from "@/components/trades/TradePreviewTable";
import PositionPreviewTable from "@/components/trades/PositionPreviewTable";
import WatchPreviewTable from "@/components/trades/WatchPreviewTable";
import type { ImportPreview, PreviewPosition, PreviewTrade, PreviewWatch } from "@/lib/types";

type Kind = "" | "trade" | "position" | "watchlist";

/** 截图导入：选类型 → 上传识别 → 预览可编辑(勾选) → 确认入库（含自选镜像） */
export default function ImportFlow({ onDone }: { onDone: () => void }) {
  const [kind, setKind] = useState<Kind>("");
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

  const patch = <S extends "trades" | "positions" | "watch", T>(
    section: S,
    i: number,
    p: Partial<T>
  ) =>
    setPreview((prev) => {
      if (!prev) return prev;
      const arr = [...(prev[section] as unknown as T[])];
      arr[i] = { ...arr[i], ...p };
      return { ...prev, [section]: arr };
    });

  // 整组取消/恢复移出勾选（仅影响 change==='remove' 行）
  const toggleRemoveAll = (include: boolean) =>
    setPreview((prev) =>
      prev
        ? {
            ...prev,
            watch: prev.watch.map((w) =>
              w.change === "remove" ? { ...w, include } : w
            ),
          }
        : prev
    );

  const confirm = async () => {
    if (!preview) return;
    setBusy(true);
    setErr(null);
    try {
      await apiPost<{ ok?: boolean; applied?: number }>(ENDPOINTS.tradesConfirm, preview);
      setMsg(preview.kind === "watchlist" ? "自选已同步" : "已确认入库");
      reset();
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
    setKind("");
    if (inputRef.current) inputRef.current.value = "";
  };

  return (
    <div className="panel panel-pad flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="mb-0 text-[15px] font-bold tracking-wide">截图导入</h2>
        <span className="text-xs text-faint">
          同花顺今日交易 / 持仓 / 自选截图，AI 视觉识别后预览确认
        </span>
        {msg && <span className="text-xs" style={{ color: "var(--green)" }}>{msg}</span>}
        {err && <span className="text-xs" style={{ color: "var(--red)" }}>{err}</span>}
      </div>

      {!preview && (
        <ImportControls
          kind={kind}
          file={file}
          busy={busy}
          inputRef={inputRef}
          onKind={setKind}
          onFile={(f) => {
            setFile(f);
            setErr(null);
          }}
          onClearFile={() => setFile(null)}
          onUpload={upload}
        />
      )}

      {preview && (
        <>
          {preview.kind === "watchlist" ? (
            <WatchPreviewTable rows={preview.watch} onPatch={(i, p) => patch<"watch", PreviewWatch>("watch", i, p)} onToggleRemoveAll={toggleRemoveAll} />
          ) : preview.kind === "position" ? (
            <PositionPreviewTable rows={preview.positions} onPatch={(i, p) => patch<"positions", PreviewPosition>("positions", i, p)} />
          ) : (
            <TradePreviewTable rows={preview.trades} onPatch={(i, p) => patch<"trades", PreviewTrade>("trades", i, p)} />
          )}

          <div className="flex items-center gap-2">
            <button
              onClick={confirm}
              disabled={busy}
              className="inline-flex h-8 items-center gap-1.5 rounded-[10px] bg-accent px-3 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
            >
              {busy ? "确认中…" : "确认导入"}
            </button>
            <button
              onClick={() => {
                reset();
                setErr(null);
                setMsg(null);
              }}
              className="inline-flex h-8 items-center rounded-[10px] border border-line bg-card px-3 text-xs text-muted hover:text-up"
            >
              取消
            </button>
          </div>
        </>
      )}
    </div>
  );
}
