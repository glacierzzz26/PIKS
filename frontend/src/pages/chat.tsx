"use client";

import { useEffect, useRef, useState } from "react";
import { Eraser, Paperclip, Send, X } from "lucide-react";
import { useData } from "@/hooks/useData";
import { apiPost, apiUpload, ENDPOINTS } from "@/lib/api";
import { Chip } from "@/components/ui/Num";
import { LoadingBlock, ErrorState } from "@/components/ui/States";
import type { ChatMsg } from "@/lib/types";

function nowTime() {
  return new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
}

/** 消息气泡（user 右 / assistant 左，引用 chips） */
function Bubble({ m }: { m: ChatMsg }) {
  const user = m.role === "user";
  return (
    <div className={`max-w-[86%] rounded-sm border px-3.5 py-2.5 ${user ? "self-end border-accent bg-accent-soft" : "self-start border-line bg-card"}`}>
      <div className="num mb-1 text-left text-[11px] text-muted">
        {user ? "我" : "AI"} · {m.time}
      </div>
      <div className="whitespace-pre-wrap break-words text-sm leading-relaxed">{m.content}</div>
      {m.refs && m.refs.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {m.refs.map((r) => (
            <Chip key={r} tone="dim">{r}</Chip>
          ))}
        </div>
      )}
    </div>
  );
}

/** AI 对话（交互）：历史 + 提问/截图上传 + 引用 chips + 清空 */
export default function Page() {
  const initial = useData<ChatMsg[]>({ path: ENDPOINTS.chat });
  const [extra, setExtra] = useState<ChatMsg[]>([]);
  const [text, setText] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [sending, setSending] = useState(false);
  const [hint, setHint] = useState<string | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  const msgs = [...(initial.data ?? []), ...extra];

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [msgs.length, sending]);

  const send = async () => {
    const q = text.trim();
    const f = file;
    if ((!q && !f) || sending) return;
    setExtra((x) => [...x, { role: "user", content: q || (f ? `[图片 ${f.name}]` : ""), time: nowTime() }]);
    setText("");
    setFile(null);
    setSending(true);
    setHint(null);
    try {
      const fd = new FormData();
      if (q) fd.append("question", q);
      if (f) fd.append("file", f);
      const res = await apiUpload<{ message: ChatMsg; note?: string }>(ENDPOINTS.chat, fd);
      setExtra((x) => [...x, res.message]);
      if (res.note) setHint(res.note);
    } catch (e) {
      setExtra((x) => [
        ...x,
        { role: "assistant", content: `⚠️ ${e instanceof Error ? e.message : String(e)}`, time: nowTime() },
      ]);
    } finally {
      setSending(false);
    }
  };

  const clear = async () => {
    if (!window.confirm("清空当前对话记录？")) return;
    await apiPost<{ ok: boolean }>(ENDPOINTS.chatClear);
    setExtra([]);
    setHint(null);
    initial.refresh();
  };

  return (
    <div className="mx-auto flex h-[calc(100vh-210px)] min-h-[440px] max-w-[860px] flex-col">
      <div className="mb-1 mt-5 flex items-center gap-3">
        <h1 className="mb-0 text-2xl font-bold tracking-wide">AI 对话</h1>
        <span className="text-[13px] text-muted">问答带知识库引用 · 支持截图</span>
        <button
          onClick={clear}
          className="ml-auto inline-flex h-8 items-center gap-1.5 rounded-sm border border-line bg-card px-3 text-xs text-muted hover:text-up"
        >
          <Eraser size={12} />
          清空会话
        </button>
      </div>

      {hint && (
        <p className="mb-2 rounded border border-dashed border-line bg-card-soft px-3 py-2 text-xs text-muted">
          {hint}
        </p>
      )}

      <div className="mt-2 flex min-h-0 flex-1 flex-col overflow-hidden rounded border border-line bg-card shadow-card">
        <div className="flex flex-1 flex-col gap-3 overflow-y-auto px-4 py-4">
          {initial.loading ? (
            <LoadingBlock rows={3} />
          ) : initial.error ? (
            <ErrorState msg={initial.error} />
          ) : msgs.length === 0 ? (
            <p className="m-auto text-[13px] text-muted">
              还没有对话记录 —— 输入问题，或上传同花顺截图让 AI 解读。
            </p>
          ) : (
            msgs.map((m, i) => <Bubble key={i} m={m} />)
          )}
          {sending && (
            <div className="self-start rounded border border-line bg-card px-3.5 py-2 text-xs text-muted">
              AI 思考中…
            </div>
          )}
          <div ref={bottomRef} />
        </div>

        <div className="border-t border-line bg-card-soft p-3">
          {file && (
            <span className="mb-2 inline-flex items-center gap-1.5 rounded-sm border border-line bg-card px-2 py-1 text-xs text-muted">
              <Paperclip size={11} />
              {file.name}
              <button onClick={() => setFile(null)} className="text-muted hover:text-up">
                <X size={11} />
              </button>
            </span>
          )}
          <div className="flex items-center gap-2">
            <label className="inline-flex h-9 w-9 cursor-pointer items-center justify-center rounded-sm border border-line bg-card text-muted hover:text-accent">
              <Paperclip size={14} />
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp,image/gif"
                className="hidden"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
            </label>
            <input
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.nativeEvent.isComposing) send();
              }}
              placeholder="输入问题（Enter 发送）"
              className="h-9 flex-1 rounded-sm border border-line bg-card px-3 text-sm outline-none focus:border-accent"
            />
            <button
              onClick={send}
              disabled={sending || (!text.trim() && !file)}
              className="inline-flex h-9 items-center gap-1.5 rounded-sm bg-accent px-3 text-xs font-medium text-white disabled:opacity-40"
            >
              <Send size={12} />
              发送
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
