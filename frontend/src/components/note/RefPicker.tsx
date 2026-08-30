"use client";

import { useState } from "react";

/** 可搜索多选器：笔记事件/实体关联选择（选项来自真实 API） */
export default function RefPicker({
  title,
  options,
  selected,
  onChange,
}: {
  title: string;
  options: { id: string; label: string }[];
  selected: string[];
  onChange: (ids: string[]) => void;
}) {
  const [kw, setKw] = useState("");
  const q = kw.trim().toLowerCase();
  const shown = options
    .filter((o) => !q || o.label.toLowerCase().includes(q))
    .slice(0, 40);

  const toggle = (id: string) =>
    onChange(
      selected.includes(id)
        ? selected.filter((s) => s !== id)
        : [...selected, id]
    );

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <label className="text-[13px] font-semibold text-muted">{title}</label>
        <span className="num text-xs text-muted">已选 {selected.length}</span>
      </div>
      <input
        value={kw}
        onChange={(e) => setKw(e.target.value)}
        placeholder={`搜索${title}…`}
        className="h-8 w-full rounded-sm border border-line bg-card px-2.5 text-xs outline-none placeholder:text-muted focus:border-accent"
      />
      <div className="mt-1.5 flex max-h-44 flex-col gap-0.5 overflow-auto rounded-sm border border-line bg-card p-1.5">
        {shown.length === 0 && (
          <div className="px-2 py-3 text-center text-xs text-muted">
            无匹配项
          </div>
        )}
        {shown.map((o) => (
          <label
            key={o.id}
            className="flex cursor-pointer items-center gap-2 rounded px-1.5 py-1 text-[13px] hover:bg-bg-soft"
          >
            <input
              type="checkbox"
              checked={selected.includes(o.id)}
              onChange={() => toggle(o.id)}
              className="h-3.5 w-3.5 shrink-0 accent-[var(--accent)]"
            />
            <span className="flex-1 truncate">{o.label}</span>
          </label>
        ))}
      </div>
    </div>
  );
}
