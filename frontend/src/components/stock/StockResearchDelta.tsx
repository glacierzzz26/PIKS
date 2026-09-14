"use client";

import { ArrowRight, Minus, TrendingDown, TrendingUp } from "lucide-react";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import { useResearchDelta, type DeltaRow } from "@/hooks/useResearchDelta";
import { RISK_LEVEL_LABEL } from "@/lib/constants";
import type { ResearchRunSummary } from "@/lib/types";

/** 涨跌用 A 股习惯（涨红跌绿）；评分/风险/回撤为中性语义，不着涨跌色。 */
function deltaTone(row: DeltaRow): string {
  if (row.kind === "level" || row.kind === "score") return "text-muted";
  if (row.key === "max_drawdown_pct") return "text-muted"; // 回撤非涨跌，不着色（同 Fact 区约定）
  if (row.delta === null) return "text-faint";
  if (row.delta > 0) return "text-up";
  if (row.delta < 0) return "text-down";
  return "text-muted";
}

const fmtVal = (row: DeltaRow, v: number | null, text?: string): string => {
  if (v === null) return text ? "" : "—";
  if (row.kind === "score") return v > 0 ? `+${v}` : `${v}`;
  if (row.kind === "price") return v.toFixed(2);
  return `${v > 0 ? "+" : ""}${v.toFixed(2)}%`;
};

const fmtDelta = (row: DeltaRow): string => {
  if (row.delta === null) return "—";
  const sign = row.delta > 0 ? "+" : "";
  if (row.kind === "price") return `${sign}${row.delta.toFixed(2)}`;
  if (row.kind === "score") return `${sign}${row.delta}`;
  return `${sign}${row.delta.toFixed(2)}%`;
};

/** 综合评分等带档位文字的数值：值 + 小号档位。 */
function Val({ row, v, text }: { row: DeltaRow; v: number | null; text?: string }) {
  return (
    <>
      {fmtVal(row, v, text)}
      {row.kind === "score" && text && (
        <span className="ml-1 text-[11px] font-normal text-faint">{text}</span>
      )}
    </>
  );
}

function DeltaIcon({ row }: { row: DeltaRow }) {
  if (row.kind === "level" || row.delta === null)
    return <Minus size={12} className="text-faint" />;
  if (row.delta > 0) return <TrendingUp size={12} className="text-up" />;
  if (row.delta < 0) return <TrendingDown size={12} className="text-down" />;
  return <Minus size={12} className="text-faint" />;
}

/** 文本型行：风险等级（上次 → 最新），值无可比性，变化列显方向箭头。 */
function TextRow({ row }: { row: DeltaRow }) {
  const p = RISK_LEVEL_LABEL[row.prevText ?? ""] ?? row.prevText;
  const c = RISK_LEVEL_LABEL[row.curText ?? ""] ?? row.curText;
  const changed = (row.prevText ?? "") !== (row.curText ?? "");
  return (
    <tr className="h-[40px]">
      <td className="text-left text-muted">{row.label}</td>
      <td className="num-t text-faint">{p || "—"}</td>
      <td className={`num-t font-semibold ${changed ? "text-ink" : "text-muted"}`}>
        {c || "—"}
      </td>
      <td className="text-right">
        {changed ? <ArrowRight size={12} className="inline text-muted" /> : <Minus size={12} className="inline text-faint" />}
      </td>
    </tr>
  );
}

function NumRow({ row }: { row: DeltaRow }) {
  return (
    <tr className="h-[40px]">
      <td className="text-left text-muted">{row.label}</td>
      <td className="num-t text-faint">
        <Val row={row} v={row.prev} text={row.prevText} />
      </td>
      <td className="num-t text-ink">
        <Val row={row} v={row.cur} text={row.curText} />
      </td>
      <td className={`num-t font-semibold ${deltaTone(row)}`}>
        <DeltaIcon row={row} />
        <span className="ml-1">{fmtDelta(row)}</span>
      </td>
    </tr>
  );
}

/**
 * 个股中心「研究变化」（设计 ux-ia §3.4）：最近两份 done 报告的关键指标 diff。
 * <2 份 done → 如实空态，不伪造对比。
 */
export default function StockResearchDelta({ runs }: { runs: ResearchRunSummary[] }) {
  const { view, loading, error, hasPair } = useResearchDelta(runs);

  if (!hasPair) {
    return (
      <StockSectionEmpty tip="暂无历史报告可对比 —— 同一只票做过两次深研后，这里显示变化" />
    );
  }
  if (loading) {
    return <p className="text-[13px] text-faint">正在对比最近两份报告…</p>;
  }
  if (error || !view) {
    return <StockSectionEmpty tip={`对比失败：${error ?? "数据不可得"}`} />;
  }

  return (
    <div>
      <div className="mb-2 flex items-center gap-2 text-[12.5px] text-faint">
        <span className="num">{view.prev.as_of}</span>
        <ArrowRight size={12} />
        <span className="num text-muted">{view.cur.as_of}(最新)</span>
      </div>
      <table className="table">
        <thead>
          <tr className="h-[34px]">
            <th className="text-left text-muted">指标</th>
            <th className="text-right text-muted">上次</th>
            <th className="text-right text-muted">最新</th>
            <th className="text-right text-muted">变化</th>
          </tr>
        </thead>
        <tbody>
          {view.rows.map((row) =>
            row.kind === "level" ? (
              <TextRow key={row.key} row={row} />
            ) : (
              <NumRow key={row.key} row={row} />
            )
          )}
        </tbody>
      </table>
    </div>
  );
}
