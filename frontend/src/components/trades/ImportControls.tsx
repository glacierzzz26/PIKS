"use client";

import { Upload, X } from "lucide-react";

type Kind = "" | "trade" | "position" | "watchlist";

const KINDS = [
  { k: "trade", label: "今日交易" },
  { k: "position", label: "持仓" },
  { k: "watchlist", label: "自选股" },
] as const;

/** 截图导入的「选类型 + 选文件 + 开始识别」控件条。 */
export default function ImportControls({
  kind,
  file,
  busy,
  inputRef,
  onKind,
  onFile,
  onClearFile,
  onUpload,
}: {
  kind: Kind;
  file: File | null;
  busy: boolean;
  inputRef: React.RefObject<HTMLInputElement>;
  onKind: (k: Kind) => void;
  onFile: (f: File | null) => void;
  onClearFile: () => void;
  onUpload: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      {KINDS.map((o) => (
        <button
          key={o.k}
          onClick={() => onKind(o.k)}
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
          onChange={(e) => onFile(e.target.files?.[0] ?? null)}
        />
      </label>
      {file && (
        <span className="inline-flex items-center gap-1 text-xs text-faint">
          {file.name}
          <button onClick={onClearFile} style={{ color: "var(--ink-faint)" }}>
            <X size={11} />
          </button>
        </span>
      )}
      <button
        onClick={onUpload}
        disabled={busy || !kind || !file}
        className="inline-flex h-8 items-center rounded-[9px] border border-line bg-card px-3 text-xs text-muted hover:text-accent disabled:opacity-40"
      >
        {busy ? "识别中…" : "开始识别"}
      </button>
    </div>
  );
}
