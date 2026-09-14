"use client";

import { Link } from "react-router-dom";
import { FileText, Newspaper, NotebookPen } from "lucide-react";
import type { DecisionRef } from "@/lib/types";

const KIND = {
  research: { icon: FileText, label: "研报" },
  event: { icon: Newspaper, label: "消息" },
  note: { icon: NotebookPen, label: "笔记" },
} as const;

/** 决策关联引用（P6-4）：一条可点深链的「当时在看什么」。
 *  注意：prop 名不能叫 ref（React 保留属性，不会作为普通 prop 传入）。 */
export function DecisionRefChip({ item }: { item: DecisionRef }) {
  const meta = KIND[item.kind];
  const Icon = meta.icon;
  const body = (
    <>
      <Icon size={12} className="shrink-0 text-faint" />
      <span className="text-faint">{meta.label}</span>
      <span className="max-w-[240px] truncate">{item.title}</span>
      {item.date && <span className="num text-faint">{item.date}</span>}
    </>
  );
  const cls =
    "inline-flex items-center gap-1.5 rounded-[6px] border border-line bg-card px-2 py-0.5 text-[12.5px]";
  return item.url ? (
    <Link to={item.url} className={`${cls} no-underline hover:border-accent hover:text-accent`}>
      {body}
    </Link>
  ) : (
    <span className={cls}>{body}</span>
  );
}

/** 决策关联列表；空时如实说明「无记录」，不编造。 */
export function DecisionRefList({
  refs,
  emptyText = "未记录当时依据",
}: {
  refs: DecisionRef[];
  emptyText?: string;
}) {
  if (refs.length === 0) {
    return <span className="text-[12.5px] text-faint italic">{emptyText}</span>;
  }
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {refs.map((r) => (
        <DecisionRefChip key={`${r.kind}-${r.id}`} item={r} />
      ))}
    </span>
  );
}
