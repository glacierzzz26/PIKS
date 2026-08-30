"use client";

import { useMemo, useState } from "react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import { EVENT_TYPE_LABEL } from "@/lib/format";
import type { EventItem, Entity, NoteInput, NoteDetail } from "@/lib/types";
import RefPicker from "./RefPicker";

const NOTE_TYPES = [
  { key: "note", label: "我的理解" },
  { key: "belief", label: "信念" },
  { key: "case", label: "案例" },
  { key: "mistake", label: "错误" },
];
const BELIEF_STATUS = ["hypothesis", "active", "confirmed", "questioned", "rejected"];
const NOTE_STATUS = ["active", "archived"];
const STATUS_LABEL: Record<string, string> = {
  hypothesis: "假设",
  active: "活跃",
  confirmed: "已确认",
  questioned: "存疑",
  rejected: "否定",
  archived: "归档",
};

const inputCls =
  "h-9 w-full rounded-sm border border-line bg-card px-2.5 text-sm outline-none placeholder:text-muted focus:border-accent";

/** 笔记编辑表单：type/slug/title/status/confidence/content + 事件/实体关联 */
export default function NoteForm({
  initial,
  submitLabel,
  onSubmit,
}: {
  initial?: NoteDetail | null;
  submitLabel: string;
  onSubmit: (input: NoteInput) => Promise<void>;
}) {
  const [type, setType] = useState(initial?.type ?? "note");
  const [title, setTitle] = useState(initial?.title ?? "");
  const [slug, setSlug] = useState(initial?.slug ?? "");
  const [status, setStatus] = useState(initial?.status ?? "hypothesis");
  const [confidence, setConfidence] = useState(
    initial?.confidence != null ? String(initial.confidence) : ""
  );
  const [content, setContent] = useState(initial?.content ?? "");
  const [selEvents, setSelEvents] = useState<string[]>(initial?.sel_events ?? []);
  const [selEnts, setSelEnts] = useState<string[]>(initial?.sel_entities ?? []);
  const [err, setErr] = useState("");
  const [saving, setSaving] = useState(false);

  const events = useData<EventItem[]>({ path: ENDPOINTS.events });
  const entities = useData<Entity[]>({ path: ENDPOINTS.entities });

  const statusOptions = type === "belief" ? BELIEF_STATUS : NOTE_STATUS;
  const effStatus = statusOptions.includes(status) ? status : statusOptions[0];

  const eventOpts = useMemo(
    () =>
      (events.data ?? []).map((e) => ({
        id: e.id,
        label: `${e.title}（${EVENT_TYPE_LABEL[e.event_type] ?? e.event_type}）`,
      })),
    [events.data]
  );
  const entOpts = useMemo(
    () => (entities.data ?? []).map((e) => ({ id: e.id, label: e.name })),
    [entities.data]
  );

  const submit = async () => {
    if (!title.trim()) return setErr("标题必填");
    if (!content.trim()) return setErr("内容必填");
    setErr("");
    setSaving(true);
    try {
      await onSubmit({
        type,
        slug: slug.trim(),
        title: title.trim(),
        status: effStatus,
        confidence: confidence.trim() || undefined,
        content,
        sel_events: selEvents,
        sel_entities: selEnts,
      });
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <div className="grid gap-4 md:grid-cols-2">
        <div>
          <label className="mb-1.5 block text-[13px] font-semibold text-muted">
            类型
          </label>
          <select
            value={type}
            onChange={(e) => setType(e.target.value)}
            className={inputCls}
          >
            {NOTE_TYPES.map((t) => (
              <option key={t.key} value={t.key}>
                {t.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1.5 block text-[13px] font-semibold text-muted">
            标题 <span className="text-up">*</span>
          </label>
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="一句话概括这条笔记"
            className={inputCls}
          />
        </div>
        <div>
          <label className="mb-1.5 block text-[13px] font-semibold text-muted">
            状态
          </label>
          <select
            value={effStatus}
            onChange={(e) => setStatus(e.target.value)}
            className={inputCls}
          >
            {statusOptions.map((s) => (
              <option key={s} value={s}>
                {STATUS_LABEL[s] ?? s}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1.5 block text-[13px] font-semibold text-muted">
            置信度（0~1，可选）
          </label>
          <input
            value={confidence}
            onChange={(e) => setConfidence(e.target.value)}
            placeholder="如 0.8"
            className={inputCls}
          />
        </div>
      </div>
      <div>
        <label className="mb-1.5 block text-[13px] font-semibold text-muted">
          slug（可选，留空自动生成）
        </label>
        <input
          value={slug}
          onChange={(e) => setSlug(e.target.value)}
          placeholder="note-xxx"
          className={inputCls}
        />
      </div>
      <div>
        <label className="mb-1.5 block text-[13px] font-semibold text-muted">
          内容（Markdown）<span className="text-up">*</span>
        </label>
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          rows={10}
          placeholder="## 逻辑链&#10;…"
          className="w-full rounded-sm border border-line bg-card px-2.5 py-2 font-mono text-sm leading-relaxed outline-none placeholder:text-muted focus:border-accent"
        />
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <RefPicker
          title="关联事件"
          options={eventOpts}
          selected={selEvents}
          onChange={setSelEvents}
        />
        <RefPicker
          title="关联实体"
          options={entOpts}
          selected={selEnts}
          onChange={setSelEnts}
        />
      </div>
      {err && <p className="m-0 text-[13px] text-up">{err}</p>}
      <div className="flex items-center gap-3">
        <button
          onClick={submit}
          disabled={saving}
          className="h-9 rounded-sm bg-accent px-5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
        >
          {saving ? "保存中…" : submitLabel}
        </button>
      </div>
    </div>
  );
}
