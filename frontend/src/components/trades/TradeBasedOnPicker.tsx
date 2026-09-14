"use client";

import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import RefPicker from "@/components/note/RefPicker";
import type { StockHub, TradeBasedOn } from "@/lib/types";

const EMPTY: TradeBasedOn = { run_ids: [], event_ids: [], note_ids: [] };

/**
 * 「当时在看什么」选择器（P6-4）：为一张买入单关联当时的研报 / 消息 / 笔记。
 * 数据源 = 该 code 的个股中心聚合（GET /stock/:code），选项天然限定在这只票相关。
 * 代码非 6 位 / 无关联数据 → 折叠且如实说明，不阻塞录入（关联是可选项）。
 */
export default function TradeBasedOnPicker({
  code,
  value,
  onChange,
}: {
  code: string;
  value: TradeBasedOn;
  onChange: (v: TradeBasedOn) => void;
}) {
  const [open, setOpen] = useState(false);
  const valid = /^\d{6}$/.test(code);
  const hub = useData<StockHub>({
    path: valid ? ENDPOINTS.stock.replace(":code", code) : null,
  });

  const total = value.run_ids.length + value.event_ids.length + value.note_ids.length;
  const d = hub.data;
  const optionCount = d
    ? d.research.filter((r) => r.status === "done").length + d.events.length + d.notes.length
    : 0;

  if (!valid) {
    return (
      <p className="text-[12.5px] text-faint">
        填入 6 位代码后，可关联「当时在看什么」的研报 / 消息 / 笔记。
      </p>
    );
  }

  return (
    <div className="rounded-[10px] border border-line bg-card-soft p-2.5">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-1.5 text-left text-[12.5px] font-semibold text-muted"
      >
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        当时在看什么（可选）
        {total > 0 && <span className="st st-accent">已选 {total}</span>}
      </button>

      {open && (
        <div className="mt-2.5">
          {hub.loading ? (
            <p className="py-3 text-center text-[12.5px] text-faint">正在加载这只票的关联数据…</p>
          ) : hub.error ? (
            <p className="py-3 text-center text-[12.5px] text-faint">
              关联数据加载失败：{hub.error}
            </p>
          ) : optionCount === 0 ? (
            <p className="py-3 text-center text-[12.5px] text-faint italic">
              这只票暂时没有可关联的研报 / 消息 / 笔记（如实空）。
            </p>
          ) : (
            <div className="grid gap-2.5 md:grid-cols-3">
              <RefPicker
                title="研报"
                options={
                  d!.research
                    .filter((r) => r.status === "done")
                    .map((r) => ({ id: r.id, label: `${r.as_of} · ${r.profile || "深研"}` }))
                }
                selected={value.run_ids}
                onChange={(ids) => onChange({ ...value, run_ids: ids })}
              />
              <RefPicker
                title="消息"
                options={d!.events.map((e) => ({
                  id: e.id,
                  label: `${e.occurred_at.slice(0, 10)} · ${e.title}`,
                }))}
                selected={value.event_ids}
                onChange={(ids) => onChange({ ...value, event_ids: ids })}
              />
              <RefPicker
                title="笔记"
                options={d!.notes.map((n) => ({ id: n.id, label: n.title }))}
                selected={value.note_ids}
                onChange={(ids) => onChange({ ...value, note_ids: ids })}
              />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export { EMPTY as EMPTY_BASED_ON };
