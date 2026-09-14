"use client";

import { Suspense, useState } from "react";
import { Plus, Upload } from "lucide-react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { TradeRow, PositionRow } from "@/lib/types";
import Pagination from "@/components/ui/Pagination";
import { LoadingBlock, EmptyState, ErrorState } from "@/components/ui/States";
import { Chip, Num } from "@/components/ui/Num";
import { usePagedQuery } from "@/hooks/usePagedQuery";
import TradeTable from "@/components/trades/TradeTable";
import PositionTable from "@/components/trades/PositionTable";
import TradeAddForm from "@/components/trades/TradeAddForm";
import ImportFlow from "@/components/trades/ImportFlow";
import PosReview from "@/components/trades/PosReview";

type TradesData = { trades: TradeRow[]; positions: PositionRow[] };

/** 交易（交互）：成交/持仓表 + 手动录入 + 截图导入 + AI 解读 */
export default function Page() {
  return (
    <Suspense fallback={<div className="mt-6 h-40 animate-pulse rounded bg-card" />}>
      <TradesInner />
    </Suspense>
  );
}

function TradesInner() {
  const { query, setFilter, page, size, setPage, setSize, paginate } =
    usePagedQuery();
  const side = query.side ?? "";
  const [panel, setPanel] = useState<"" | "add" | "import">("");

  const tr = useData<TradesData>({ path: ENDPOINTS.trades });
  const records = tr.data?.trades ?? [];
  const positions = tr.data?.positions ?? [];

  const all = side ? records.filter((t) => t.side === side) : records;
  const rows = paginate(all);
  const buys = records.filter((t) => t.side === "buy");
  const sells = records.filter((t) => t.side === "sell");
  const buyAmt = buys.reduce((s, t) => s + t.amount, 0);
  const sellAmt = sells.reduce((s, t) => s + t.amount, 0);

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>交易</h1>
          <div className="psub">成交记录与持仓快照 · 手动录入 / 截图导入 / AI 解读</div>
        </div>
        <div className="meta">
          <button
            onClick={() => setPanel(panel === "add" ? "" : "add")}
            className={`inline-flex h-8 items-center gap-1.5 rounded-[10px] border px-3 text-xs ${panel === "add" ? "border-accent bg-accent-soft text-accent" : "border-line bg-card text-muted hover:text-accent"}`}
          >
            <Plus size={13} />
            手动录入
          </button>
          <button
            onClick={() => setPanel(panel === "import" ? "" : "import")}
            className={`inline-flex h-8 items-center gap-1.5 rounded-[10px] border px-3 text-xs ${panel === "import" ? "border-accent bg-accent-soft text-accent" : "border-line bg-card text-muted hover:text-accent"}`}
          >
            <Upload size={13} />
            截图导入
          </button>
        </div>
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <Chip tone="up">买入 {buys.length} 笔</Chip>
        <Chip tone="down">卖出 {sells.length} 笔</Chip>
        <Chip tone="dim">
          买入额 <Num value={buyAmt} className="inline text-ink" /> 元
        </Chip>
        <Chip tone="dim">
          卖出额 <Num value={sellAmt} className="inline text-ink" /> 元
        </Chip>
      </div>

      {panel === "add" && (
        <div className="mb-3.5">
          <TradeAddForm onDone={() => tr.refresh()} />
        </div>
      )}
      {panel === "import" && (
        <div className="mb-3.5">
          <ImportFlow onDone={() => tr.refresh()} />
        </div>
      )}

      {tr.loading ? (
        <div className="panel">
          <LoadingBlock rows={6} />
        </div>
      ) : tr.error ? (
        <div className="panel">
          <ErrorState msg={tr.error} />
        </div>
      ) : records.length === 0 && positions.length === 0 ? (
        <div className="panel">
          <EmptyState tip="暂无成交记录与持仓，可用「截图导入」或「手动录入」补充" />
        </div>
      ) : (
        <>
          <div className="panel">
            <div className="flex h-12 items-center gap-2 border-b border-line px-5">
              <h2 className="mb-0 text-[15px] font-bold tracking-wide">成交记录</h2>
              <div className="ml-2 flex items-center gap-1.5">
                {[
                  { k: "", label: "全部" },
                  { k: "buy", label: "买入" },
                  { k: "sell", label: "卖出" },
                ].map((o) => (
                  <button
                    key={o.k}
                    onClick={() => setFilter("side", o.k)}
                    className={`chip-btn ${side === o.k ? "on" : ""}`}
                  >
                    {o.label}
                  </button>
                ))}
              </div>
            </div>
            <TradeTable rows={rows} refresh={tr.refresh} />
            <Pagination
              page={page}
              pageSize={size}
              total={all.length}
              onPage={setPage}
              onPageSize={setSize}
            />
          </div>

          <div className="panel mt-3.5">
            <div className="flex h-12 items-center border-b border-line px-5">
              <h2 className="mb-0 text-[15px] font-bold tracking-wide">当前持仓</h2>
            </div>
            <PositionTable positions={positions} />
          </div>

          <PosReview />
        </>
      )}
    </div>
  );
}
