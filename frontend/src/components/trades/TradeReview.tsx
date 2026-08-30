"use client";

import { useState } from "react";
import { Wand2 } from "lucide-react";
import { apiPost } from "@/lib/api";
import MarkdownBody from "@/components/md/MarkdownBody";
import type { TradeRow } from "@/lib/types";

/** 单笔交易 AI 复盘：解读 → 展示复盘点 → 存为笔记 */
export default function TradeReview({
  trade,
  refresh,
}: {
  trade: TradeRow;
  refresh: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  const review = async () => {
    setBusy(true);
    setMsg(null);
    try {
      await apiPost<{ ok: boolean }>(`/trades/${trade.id}/review`);
      setMsg("✅ 复盘已生成");
      refresh();
    } catch (e) {
      setMsg(`⚠️ ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setBusy(false);
    }
  };

  const save = async (n: number) => {
    try {
      const res = await apiPost<{ message: string }>(`/trades/${trade.id}/save-mistake/${n}`);
      setMsg(res.message);
    } catch (e) {
      setMsg(`⚠️ ${e instanceof Error ? e.message : String(e)}`);
    }
  };

  return (
    <div className="flex flex-col gap-2">
      {msg && <p className="text-xs text-muted">{msg}</p>}
      {trade.review ? (
        <>
          <MarkdownBody content={trade.review} />
          {trade.mistakes && trade.mistakes.length > 0 && (
            <div className="flex flex-col gap-1.5">
              {trade.mistakes.map((m, i) => (
                <div
                  key={i}
                  className="flex items-start justify-between gap-3 rounded border border-line px-3 py-2"
                >
                  <div className="flex-1">
                    <p className="text-sm font-medium">{m.title}</p>
                    <p className="text-xs text-muted">{m.content}</p>
                  </div>
                  <button
                    onClick={() => save(i)}
                    className="shrink-0 rounded-sm border border-line bg-card px-2 py-1 text-xs text-muted hover:text-accent"
                  >
                    存为笔记
                  </button>
                </div>
              ))}
            </div>
          )}
        </>
      ) : (
        <p className="text-[13px] text-muted">暂无 AI 复盘。</p>
      )}
      <button
        onClick={review}
        disabled={busy}
        className="inline-flex h-7 w-fit items-center gap-1.5 rounded-sm border border-line bg-card px-2.5 text-xs text-muted hover:text-accent disabled:opacity-50"
      >
        <Wand2 size={12} />
        {busy ? "解读中…" : trade.review ? "重新解读" : "AI 解读"}
      </button>
    </div>
  );
}
