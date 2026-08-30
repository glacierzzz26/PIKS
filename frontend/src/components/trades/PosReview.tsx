"use client";

import { useState } from "react";
import { Wand2 } from "lucide-react";
import { useData } from "@/hooks/useData";
import { apiPost, ENDPOINTS } from "@/lib/api";
import MarkdownBody from "@/components/md/MarkdownBody";
import type { ReviewRow } from "@/lib/types";

/** 持仓组合 AI 诊断：生成/更新 → 展示诊断 → 风险候选存为笔记 */
export default function PosReview() {
  const reviews = useData<ReviewRow[]>({ path: ENDPOINTS.reviews });
  const latest = reviews.data?.[0];
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  const run = async () => {
    setBusy(true);
    setMsg(null);
    try {
      await apiPost<{ ok: boolean }>(`/trades/positions/review`);
      setMsg("✅ 诊断已生成");
      reviews.refresh();
    } catch (e) {
      setMsg(`⚠️ ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setBusy(false);
    }
  };

  const save = async (n: number) => {
    try {
      const res = await apiPost<{ message: string }>(`/trades/positions/save-risk/${n}`);
      setMsg(res.message);
    } catch (e) {
      setMsg(`⚠️ ${e instanceof Error ? e.message : String(e)}`);
    }
  };

  return (
    <div className="mt-3.5 rounded border border-line bg-card shadow-card">
      <div className="flex h-12 items-center gap-2 border-b border-line px-4">
        <h2 className="card-title mb-0">持仓组合诊断</h2>
        <button
          onClick={run}
          disabled={busy}
          className="ml-auto inline-flex h-8 items-center gap-1.5 rounded-sm border border-line bg-card px-3 text-xs text-muted hover:text-accent disabled:opacity-50"
        >
          <Wand2 size={12} />
          {busy ? "诊断中…" : latest ? "重新诊断" : "生成诊断"}
        </button>
      </div>
      <div className="flex flex-col gap-3 px-4 py-3">
        {msg && <p className="text-xs text-muted">{msg}</p>}
        {reviews.loading ? (
          <p className="text-[13px] text-muted">加载诊断…</p>
        ) : latest?.summary ? (
          <>
            <MarkdownBody content={latest.summary} />
            {latest.risks && latest.risks.length > 0 && (
              <div className="flex flex-col gap-1.5">
                {latest.risks.map((r, i) => (
                  <div
                    key={i}
                    className="flex items-start justify-between gap-3 rounded border border-line px-3 py-2"
                  >
                    <div className="flex-1">
                      <p className="text-sm font-medium">{r.title}</p>
                      <p className="text-xs text-muted">{r.content}</p>
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
          <p className="text-[13px] text-muted">暂无诊断（点击上方「生成诊断」触发）。</p>
        )}
      </div>
    </div>
  );
}
