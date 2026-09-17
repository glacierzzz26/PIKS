"use client";

import { Link } from "react-router-dom";
import { useTradeImport, isConfigError } from "@/hooks/useTradeImport";
import ImportControls from "@/components/trades/ImportControls";
import TradePreviewTable from "@/components/trades/TradePreviewTable";
import PositionPreviewTable from "@/components/trades/PositionPreviewTable";
import WatchPreviewTable from "@/components/trades/WatchPreviewTable";
import type { ImportPreview, PreviewPosition, PreviewTrade, PreviewWatch } from "@/lib/types";

/** 截图导入：选类型 → 上传识别 → 预览可编辑(勾选) → 确认入库（含自选镜像） */
export default function ImportFlow({ onDone }: { onDone: () => void }) {
  const imp = useTradeImport(onDone);
  const { kind, file, busy, err, msg, preview, inputRef } = imp;

  return (
    <div className="panel panel-pad flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="mb-0 text-[15px] font-bold tracking-wide">截图导入</h2>
        <span className="text-xs text-faint">
          同花顺今日交易 / 持仓 / 自选截图，AI 视觉识别后预览确认
        </span>
        {msg && <span className="text-xs" style={{ color: "var(--green)" }}>{msg}</span>}
        {err && (
          <span className="text-xs" style={{ color: "var(--red)" }}>
            {err}
            {isConfigError(err) && (
              <Link to="/settings" className="ml-2 text-accent no-underline hover:underline">
                去设置 →
              </Link>
            )}
          </span>
        )}
      </div>

      {!preview && (
        <ImportControls
          kind={kind}
          file={file}
          busy={busy}
          inputRef={inputRef}
          onKind={imp.setKind}
          onFile={(f) => {
            imp.setFile(f);
            imp.setErr(null);
          }}
          onClearFile={() => imp.setFile(null)}
          onUpload={imp.upload}
        />
      )}

      {preview && (
        <>
          <PreviewSection preview={preview} onPatch={imp.patch} onToggleRemoveAll={imp.toggleRemoveAll} />

          <div className="flex items-center gap-2">
            <button
              onClick={imp.confirm}
              disabled={busy}
              className="inline-flex h-8 items-center gap-1.5 rounded-[10px] bg-accent px-3 text-xs font-semibold text-white hover:opacity-90 disabled:opacity-50"
            >
              {busy ? "确认中…" : "确认导入"}
            </button>
            <button
              onClick={() => {
                imp.reset();
                imp.setErr(null);
                imp.setMsg(null);
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

/** 按类型分发到对应的宽表预览（桌面）。 */
function PreviewSection({
  preview,
  onPatch,
  onToggleRemoveAll,
}: {
  preview: ImportPreview;
  onPatch: <S extends "trades" | "positions" | "watch", T>(s: S, i: number, p: Partial<T>) => void;
  onToggleRemoveAll: (include: boolean) => void;
}) {
  if (preview.kind === "watchlist") {
    return (
      <WatchPreviewTable
        rows={preview.watch}
        onPatch={(i, p) => onPatch<"watch", PreviewWatch>("watch", i, p)}
        onToggleRemoveAll={onToggleRemoveAll}
      />
    );
  }
  if (preview.kind === "position") {
    return (
      <PositionPreviewTable
        rows={preview.positions}
        onPatch={(i, p) => onPatch<"positions", PreviewPosition>("positions", i, p)}
      />
    );
  }
  return (
    <TradePreviewTable
      rows={preview.trades}
      onPatch={(i, p) => onPatch<"trades", PreviewTrade>("trades", i, p)}
    />
  );
}
