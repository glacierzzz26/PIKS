"use client";

import { useState } from "react";
import { BookmarkPlus } from "lucide-react";
import { apiPost } from "@/lib/api";
import type { ReviewPoint } from "@/lib/types";

/** 一条诊断/复盘结论 + 「存为笔记」按钮;沉淀回流由后端补关系边(P6-5)。 */
function PointRow({
  point,
  label,
  savePath,
  onSaved,
}: {
  point: ReviewPoint;
  label: string;
  savePath: string; // 相对 /api/v1 的存为笔记路径(不含 index 之外的参数由调用方拼好)
  onSaved: (msg: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  const save = async () => {
    setBusy(true);
    try {
      const res = await apiPost<{ message: string }>(savePath);
      setDone(true);
      onSaved(res.message);
    } catch (e) {
      onSaved(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex items-start justify-between gap-3 rounded-[10px] border border-line px-3 py-2">
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium text-ink">{point.title}</p>
        {point.content && (
          <p className="mt-0.5 text-xs leading-relaxed text-muted">{point.content}</p>
        )}
      </div>
      <button
        onClick={save}
        disabled={busy || done}
        className="inline-flex shrink-0 items-center gap-1 rounded-[10px] border border-line bg-card px-2 py-1 text-xs text-muted hover:text-accent disabled:opacity-50"
      >
        <BookmarkPlus size={12} />
        {done ? "已存" : busy ? "保存中…" : label}
      </button>
    </div>
  );
}

/** 诊断结论列表(风险点/复盘点):逐条可存为个人笔记,存后回流个股页与笔记库。 */
export default function ReviewPointList({
  title,
  points,
  icon,
  pathFor,
  onSaved,
}: {
  title: string;
  points: ReviewPoint[];
  icon?: React.ReactNode;
  pathFor: (i: number) => string;
  onSaved: (msg: string) => void;
}) {
  if (points.length === 0) return null;
  return (
    <div className="flex flex-col gap-1.5">
      <p className="flex items-center gap-1.5 text-xs font-medium text-muted">
        {icon}
        {title}
      </p>
      {points.map((p, i) => (
        <PointRow
          key={i}
          point={p}
          label="存为笔记"
          savePath={pathFor(i)}
          onSaved={onSaved}
        />
      ))}
    </div>
  );
}
