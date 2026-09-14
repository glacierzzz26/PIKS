"use client";

import { Link } from "react-router-dom";
import { useTradeImport, isConfigError } from "@/hooks/useTradeImport";
import PhonePick from "@/components/trades/PhonePick";
import PhonePreview from "@/components/trades/PhonePreview";
import type { PreviewPosition, PreviewTrade, PreviewWatch } from "@/lib/types";

/**
 * 手机截图投递页 `/m/upload`（AppShell 之外，无侧栏）。
 * 选类型 → 拍照/选图 → 识别 → 卡片核对 → 确认入库。其余页面仍在电脑上看。
 */
export default function MobileUpload() {
  const imp = useTradeImport();
  const { busy, err, msg, preview } = imp;

  return (
    <div className="m-upload">
      <header className="m-head">
        <h1>截图投递</h1>
        <p className="psub">同花顺截图 → AI 识别 → 核对确认。识别结果请逐笔核对再入库。</p>
      </header>

      {msg && <div className="m-note m-note-ok">{msg}</div>}
      {err && (
        <div className="m-note m-note-err">
          {err}
          {isConfigError(err) && (
            <span className="m-hint">首次使用请先在电脑上打开「设置」配置视觉模型。</span>
          )}
        </div>
      )}

      {!preview && (
        <PhonePick
          kind={imp.kind}
          file={imp.file}
          busy={busy}
          onKind={imp.setKind}
          onFile={(f) => { imp.setFile(f); imp.setErr(null); }}
          onUpload={imp.upload}
        />
      )}

      {preview && (
        <>
          <Summary preview={preview} onToggleRemoveAll={imp.toggleRemoveAll} />
          <PhonePreview
            trades={preview.trades}
            positions={preview.positions}
            watch={preview.watch}
            onPatchTrade={(i, p) => imp.patch<"trades", PreviewTrade>("trades", i, p)}
            onPatchPos={(i, p) => imp.patch<"positions", PreviewPosition>("positions", i, p)}
            onPatchWatch={(i, p) => imp.patch<"watch", PreviewWatch>("watch", i, p)}
          />
          <div className="m-actionbar">
            <button className="m-primary" disabled={busy} onClick={imp.confirm}>
              {busy ? "确认中…" : "确认导入"}
            </button>
            <button className="m-secondary" onClick={() => { imp.reset(); imp.setErr(null); }}>
              重新选图
            </button>
          </div>
        </>
      )}

      <footer className="m-foot">
        也可以回 <Link to="/trades">交易与持仓</Link> 看已入库记录
      </footer>
    </div>
  );
}

/** 顶部汇总：识别到多少笔 / 自选将加入与移出 + 整组取消移出。 */
function Summary({
  preview,
  onToggleRemoveAll,
}: {
  preview: { trades: PreviewTrade[]; positions: PreviewPosition[]; watch: PreviewWatch[] };
  onToggleRemoveAll: (include: boolean) => void;
}) {
  const removes = preview.watch.filter((r) => r.change === "remove");
  const adds = preview.watch.filter((r) => r.change === "add").length;
  const allOff = removes.length > 0 && removes.every((r) => !r.include);
  const n =
    preview.trades.filter((r) => r.include).length +
    preview.positions.filter((r) => r.include).length;

  return (
    <div className="m-summary">
      {preview.watch.length > 0 ? (
        <span>将加入 <b className="text-up">{adds}</b> 只 · 将移出 <b>{removes.length}</b> 只</span>
      ) : (
        <span>勾选 <b>{n}</b> 条待入库</span>
      )}
      {removes.length > 0 && (
        <button className="m-link" onClick={() => onToggleRemoveAll(allOff)}>
          {allOff ? "恢复移出勾选" : "整组取消移出"}
        </button>
      )}
    </div>
  );
}
