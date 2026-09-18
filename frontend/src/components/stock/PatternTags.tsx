"use client";

import type { PatternLabel, PatternDivergence } from "@/lib/types";

/**
 * 量价形态（规则判定 = Inference，与事实区分区）：标签 + 每条 evidence，
 * 以及换手率峰值与股价高点的背离事实。形态不写进数字区（Fact ≠ Inference）。
 */
export default function PatternTags({
  labels,
  divergence,
  note,
}: {
  labels: PatternLabel[];
  divergence?: PatternDivergence;
  note?: string;
}) {
  return (
    <section className="mt-4 rounded-[var(--radius-md)] border border-line bg-bg-soft p-3">
      <div className="mb-2 flex items-center gap-2">
        <h3 className="m-0 text-[14px] font-bold">量价形态</h3>
        <span className="st st-dim">规则判定 · 非事实</span>
        <span className="text-[11px] text-faint">换手率：A股流通股本口径（与同花顺一致）</span>
      </div>

      {note ? (
        <p className="m-0 text-[13px] text-faint italic">{note}</p>
      ) : labels.length === 0 ? (
        <p className="m-0 text-[13px] text-faint italic">
          近 60 个交易日内未识别出显著量价形态。
        </p>
      ) : (
        <ul className="m-0 list-none space-y-2 p-0">
          {labels.map((lb) => (
            <li key={lb.code} className="flex items-start gap-2">
              <span className="st st-accent mt-0.5 shrink-0">{lb.label}</span>
              <span className="text-[12.5px] leading-relaxed text-muted">
                <span className="num mr-1 text-faint">{lb.date}</span>
                {lb.evidence}
              </span>
            </li>
          ))}
        </ul>
      )}

      {divergence?.peak_turnover != null && (
        <p className="mt-3 mb-0 border-t border-line pt-2 text-[12px] text-faint">
          换手峰值 <span className="num">{divergence.peak_turnover.toFixed(2)}%</span>
          （{divergence.peak_date}）与股价高点{" "}
          <span className="num">{divergence.high_close?.toFixed(2)}</span> 元（
          {divergence.high_date}）
          {divergence.peak_high_coincide ? "量价同步见顶" : "未共振"}；峰值后{" "}
          <span className="num">{divergence.after_peak_return_pct?.toFixed(2)}%</span>
        </p>
      )}
    </section>
  );
}
