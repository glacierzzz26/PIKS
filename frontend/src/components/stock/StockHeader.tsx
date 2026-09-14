"use client";

import { Link } from "react-router-dom";
import { ArrowLeft, Microscope } from "lucide-react";
import DeepResearchButton from "@/components/research/DeepResearchButton";
import type { StockEntity, StockIndustry } from "@/lib/types";

/**
 * 个股中心页头：代码/名称/所属行业 + 深研入口。
 * entity 可为 null（未建公司实体）：仍显示代码，名称退化为「未知标的」，如实不编造。
 */
export default function StockHeader({
  code,
  symbol,
  entity,
  industry,
}: {
  code: string;
  symbol: string;
  entity: StockEntity | null;
  industry: StockIndustry | null;
}) {
  const name = entity?.name ?? "未知标的";
  return (
    <div className="page-head">
      <div>
        <div className="mb-1 flex items-center gap-2 text-[12.5px] text-faint">
          <Link to="/" className="inline-flex items-center gap-1 hover:text-accent">
            <ArrowLeft size={13} /> 返回自选
          </Link>
          <span>· 个股中心</span>
        </div>
        <h1 className="flex items-center gap-3">
          <span className="chip">{code}</span>
          {name}
          {industry && <span className="st st-dim">{industry.name}</span>}
          {entity?.status === "watch" && <span className="st st-amber">自选</span>}
        </h1>
        <div className="psub">
          {symbol} · {entity ? "已建档案" : "暂无档案，消息 / 笔记 / 行业还没关联"}
        </div>
      </div>
      <div className="meta">
        <DeepResearchButton code={code} />
      </div>
    </div>
  );
}

/** 空态：该区块无数据时如实标注「暂无」，附可选引导链接。 */
export function StockSectionEmpty({
  tip,
  action,
}: {
  tip: string;
  action?: { to: string; label: string; icon?: typeof Microscope };
}) {
  const Icon = action?.icon;
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-10 text-faint">
      <p className="text-[13px] italic">{tip}</p>
      {action && (
        <Link
          to={action.to}
          className="inline-flex items-center gap-1 text-[12.5px] text-accent hover:underline"
        >
          {Icon && <Icon size={13} />}
          {action.label}
        </Link>
      )}
    </div>
  );
}
