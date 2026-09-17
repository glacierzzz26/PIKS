"use client";

import { useEffect, useRef, useState } from "react";
import { Camera, ImageIcon, X } from "lucide-react";
import { checkImageType, shrinkForVision } from "@/lib/image";

const KINDS = [
  { k: "trade", label: "今日交易" },
  { k: "position", label: "持仓" },
  { k: "watchlist", label: "自选股" },
] as const;

type Kind = "" | "trade" | "position" | "watchlist";

/** 手机选图步：选类型 + 拍照/相册两个入口 + 缩略图 + 开始识别。 */
export default function PhonePick({
  kind,
  file,
  busy,
  onKind,
  onFile,
  onUpload,
}: {
  kind: Kind;
  file: File | null;
  busy: boolean;
  onKind: (k: Kind) => void;
  onFile: (f: File | null) => void;
  onUpload: () => void;
}) {
  const [err, setErr] = useState<string | null>(null);
  const [url, setUrl] = useState<string | null>(null);
  const camRef = useRef<HTMLInputElement>(null);
  const galRef = useRef<HTMLInputElement>(null);

  // 缩略图 URL 生命周期：换图/卸载时 revoke，避免连传多张占住内存
  useEffect(() => () => { if (url) URL.revokeObjectURL(url); }, [url]);

  const take = async (f: File | null) => {
    if (f) {
      const bad = checkImageType(f);
      if (bad) { setErr(bad); return; }
    }
    setErr(null);
    const shrunk = f ? await shrinkForVision(f) : null;
    if (url) URL.revokeObjectURL(url);
    setUrl(shrunk ? URL.createObjectURL(shrunk) : null);
    onFile(shrunk);
  };

  return (
    <div className="m-pick">
      <div className="m-types">
        {KINDS.map((o) => (
          <button key={o.k} onClick={() => onKind(o.k)}
            className={`m-typebtn ${kind === o.k ? "on" : ""}`}>
            {o.label}
          </button>
        ))}
      </div>

      <div className="m-capture">
        <label className="m-capbtn">
          <Camera size={18} />
          拍照
          <input ref={camRef} type="file" accept="image/*" capture="environment"
            className="hidden" onChange={(e) => take(e.target.files?.[0] ?? null)} />
        </label>
        <label className="m-capbtn">
          <ImageIcon size={18} />
          从相册选
          <input ref={galRef} type="file" accept="image/*"
            className="hidden" onChange={(e) => take(e.target.files?.[0] ?? null)} />
        </label>
      </div>

      {url && file && (
        <div className="m-thumbwrap">
          <img src={url} alt="待识别截图" className="m-thumb" />
          <div className="m-thumbmeta">
            <span className="text-faint">已压缩 · {Math.round(file.size / 1024)} KB</span>
            <button className="m-link" onClick={() => { onFile(null); if (url) URL.revokeObjectURL(url); setUrl(null); }}>
              重新选择 <X size={11} />
            </button>
          </div>
        </div>
      )}

      {err && <div className="m-note m-note-err">{err}</div>}

      <button className="m-primary" disabled={busy || !kind || !file} onClick={onUpload}>
        {busy ? "识别中…" : "开始识别"}
      </button>
    </div>
  );
}
