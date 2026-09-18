"use client";

import { useNavigate } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import { RESEARCH_PROFILE_LABEL } from "@/lib/constants";
import { groupRunsByProfile } from "@/lib/research";
import { RunStatusBadge, LintBadge } from "@/components/research/StatusBadges";
import CollapsedRuns from "@/components/research/CollapsedRuns";
import StockProfileBlock from "@/components/stock/StockProfileBlock";
import { StockSectionEmpty } from "@/components/stock/StockHeader";
import { useUrlState } from "@/hooks/useUrlState";
import type { ResearchRunSummary } from "@/lib/types";

/** 按功能拆的两块（P9-4 / issue #11）—— 块名与 profile key 单一映射 */
const SPLIT = [
  { profile: "company", title: "公司质地（公司研报）", hint: "季频 · 财务/估值/行业/风险" },
  { profile: "stock", title: "个股分析", hint: "日频 · 行情/量价/换手/形态" },
] as const;

/** 一行最新报告：日期 + 状态 + 机检 + 进入。 */
function LatestRow({ run, onOpen }: { run: ResearchRunSummary; onOpen: () => void }) {
  return (
    <div
      role="button"
      tabIndex={0}
      className="flex cursor-pointer items-center gap-3 px-1 py-2.5 hover:bg-hover"
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
    >
      <span className="num w-24 text-[12.5px] text-muted">{run.as_of}</span>
      <span className="flex items-center gap-1.5">
        <RunStatusBadge status={run.status} />
        <LintBadge s={run} />
      </span>
      <span className="ml-auto inline-flex items-center gap-1 text-[12px] text-accent">
        查看 <ArrowRight size={12} />
      </span>
    </div>
  );
}

/**
 * 个股中心「深研」区块：该股历史报告（已按 code 过滤）。
 *
 * P9-4 / issue #11：按**功能**拆成两块 —— 「公司质地」（`company` 档案）与
 * 「个股分析」（`stock` 档案），各自独立触发。此前这里是按 profile 平铺任意分组，
 * 现在两块是**固定**的，因为功能边界是确定的（不是「有多少种报告类型」）。
 *
 * 其余档案（`complete-stock` 兼容档案 / `short-term` / `prebuy`）若有历史 run，
 * 仍如实列出但合并在「其它历史报告」里 —— 不隐藏用户已有的数据。
 *
 * ⚠️ 分组只在展示层：`StockResearchDelta` 仍收未分组的完整 runs 去算前后对比。
 */
export default function StockResearch({
  runs,
  code,
}: {
  runs: ResearchRunSummary[];
  code: string;
}) {
  const navigate = useNavigate();
  const [query, setParam] = useUrlState();
  const open = query.rh === "1"; // rh = research history 展开态

  if (runs.length === 0) {
    return <StockSectionEmpty tip="尚无深研报告 —— 点上面「发起」跑第一份" />;
  }

  const byProfile = new Map(groupRunsByProfile(runs).map((g) => [g.key, g]));
  // 固定两块之外的档案（兼容档案 / 短线 / 速评）合并列出
  const others = groupRunsByProfile(runs).filter(
    (g) => !SPLIT.some((s) => s.profile === g.key)
  );
  const go = (runId: string) => navigate(`/research/${runId}`);

  return (
    <div>
      <div className="mb-1 flex items-center justify-end">
        <button
          onClick={() => setParam("rh", open ? "" : "1")}
          className="inline-flex items-center gap-1 text-[12px] text-muted hover:text-accent"
        >
          {open ? "收起历史版本" : "展开历史版本"}
        </button>
      </div>

      <div className="flex flex-col gap-4">
        {SPLIT.map((s) => (
          <StockProfileBlock
            key={s.profile}
            code={code}
            group={byProfile.get(s.profile)}
            title={s.title}
            hint={s.hint}
            profile={s.profile}
            open={open}
          />
        ))}

        {others.length > 0 && (
          <div className="flex flex-col gap-3 border-t border-line pt-3">
            <div className="px-1 text-[11.5px] text-faint">其它历史报告</div>
            {others.map((g) => (
              <div key={g.key} className="flex flex-col divide-y divide-line">
                <div className="px-1 pb-1 text-[12px] text-faint">
                  {RESEARCH_PROFILE_LABEL[g.key] ?? g.key}
                  {g.total > 1 && <span className="ml-1">· 共 {g.total} 份</span>}
                </div>
                {g.latest ? (
                  <LatestRow run={g.latest} onOpen={() => go(g.latest!.run_id)} />
                ) : (
                  <div className="px-1 py-2 text-[12.5px] text-faint">尚无完成的报告</div>
                )}
                {open && <CollapsedRuns runs={g.history} kind="history" onOpen={go} />}
                {(open || !g.latest) && (
                  <CollapsedRuns runs={g.unfinished} kind="unfinished" onOpen={go} forceOpen={!g.latest} />
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
