"use client";

import { useEffect, useRef, useState } from "react";
import { Eraser } from "lucide-react";
import { useData } from "@/hooks/useData";
import { apiPost, apiUpload, ENDPOINTS } from "@/lib/api";
import { LoadingBlock, ErrorState } from "@/components/ui/States";
import { Bubble, HintCard, Composer } from "@/components/chat/parts";
import type { ChatMsg } from "@/lib/types";

function nowTime() {
  return new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
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

  const ask = async () => {
    const q = text.trim();
    const f = file;
    if ((!q && !f) || sending) return;
    setExtra((x) => [
      ...x,
      { role: "user", content: q || (f ? `[图片 ${f.name}]` : ""), time: nowTime() },
    ]);
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
        {
          role: "assistant",
          content: `⚠️ ${e instanceof Error ? e.message : String(e)}`,
          time: nowTime(),
        },
      ]);
    } finally {
      setSending(false);
    }
  };

  const askHint = (q: string) => setText(q);

  const clear = async () => {
    if (!window.confirm("清空当前对话记录？")) return;
    await apiPost<{ ok: boolean }>(ENDPOINTS.chatClear);
    setExtra([]);
    setHint(null);
    initial.refresh();
  };

  return (
    <div>
      <div className="page-head">
        <div>
          <h1>AI 对话</h1>
          <div className="psub">带知识库引用作答 · 可截图提问</div>
        </div>
        <div className="meta">
          <button
            onClick={clear}
            className="inline-flex h-8 items-center gap-1.5 rounded-[10px] border border-line bg-card px-3 text-xs text-muted hover:text-up"
          >
            <Eraser size={12} />
            清空会话
          </button>
        </div>
      </div>

      <div className="chat-wrap">
        <div className="panel flex h-[calc(100vh-190px)] min-h-[440px] flex-col overflow-hidden">
          <div className="chat-log flex-1 overflow-y-auto">
            {initial.loading ? (
              <LoadingBlock rows={3} />
            ) : initial.error ? (
              <ErrorState msg={initial.error} />
            ) : msgs.length === 0 ? (
              <p className="m-auto text-[13px] text-faint">
                还没有对话记录 —— 输入问题，或上传同花顺截图让 AI 解读。
              </p>
            ) : (
              msgs.map((m, i) => <Bubble key={i} m={m} />)
            )}
            {sending && <div className="msg ai">AI 思考中…</div>}
            <div ref={bottomRef} />
          </div>
          <Composer
            text={text}
            setText={setText}
            file={file}
            setFile={setFile}
            send={ask}
            disabled={sending || (!text.trim() && !file)}
          />
        </div>

        <div className="flex flex-col gap-3.5">
          {hint && <p className="hint-card text-[12px] text-faint">{hint}</p>}
          <HintCard onAsk={askHint} />
        </div>
      </div>
    </div>
  );
}
