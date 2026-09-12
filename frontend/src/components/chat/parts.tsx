"use client";

import { Paperclip, Send, X } from "lucide-react";
import type { ChatMsg } from "@/lib/types";

const HINTS = [
  "今天情绪温度如何？和昨天比有什么变化",
  "最近三日高置信事件里有哪些反复出现的实体",
  "我持仓里最需要关注的风险是什么",
];

/** 消息气泡（user 右 / assistant 左，引用 chips） */
export function Bubble({ m }: { m: ChatMsg }) {
  const user = m.role === "user";
  return (
    <div className={`msg ${user ? "user" : "ai"}`}>
      <div className="num mb-1 text-left text-[11px] opacity-70">
        {user ? "我" : "AI"} · {m.time}
      </div>
      <div className="whitespace-pre-wrap break-words">{m.content}</div>
      {m.refs && m.refs.length > 0 && (
        <div className="refs">
          {m.refs.map((r) => (
            <span key={r} className="chip">
              {r}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

/** 知识库提示卡（含可点问题） */
export function HintCard({ onAsk }: { onAsk: (q: string) => void }) {
  return (
    <div className="hint-card">
      <div className="ht">问答范围</div>
      <p>
        基于已入库的快讯 / 事件 / 实体 / 笔记回答，回答会附知识库引用；截图提问走视觉模型。
      </p>
      <div className="mt-3 text-[12px] font-semibold text-faint">试试这些问题</div>
      {HINTS.map((q) => (
        <button key={q} className="q text-left" onClick={() => onAsk(q)}>
          {q}
        </button>
      ))}
    </div>
  );
}

/** 输入区：附件 + 文本框 + 发送 */
export function Composer({
  text,
  setText,
  file,
  setFile,
  send,
  disabled,
}: {
  text: string;
  setText: (v: string) => void;
  file: File | null;
  setFile: (f: File | null) => void;
  send: () => void;
  disabled: boolean;
}) {
  return (
    <div className="border-t border-line bg-card-soft p-3">
      {file && (
        <span className="mb-2 inline-flex items-center gap-1.5 rounded-[9px] border border-line bg-card px-2 py-1 text-xs text-faint">
          <Paperclip size={11} />
          {file.name}
          <button onClick={() => setFile(null)} className="text-faint hover:text-up">
            <X size={11} />
          </button>
        </span>
      )}
      <div className="flex items-center gap-2">
        <label className="inline-flex h-9 w-9 cursor-pointer items-center justify-center rounded-[9px] border border-line bg-card text-muted hover:text-accent">
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
          className="input h-9 flex-1"
        />
        <button
          onClick={send}
          disabled={disabled}
          className="inline-flex h-9 items-center gap-1.5 rounded-[9px] bg-accent px-3 text-xs font-semibold text-white disabled:opacity-40"
        >
          <Send size={12} />
          发送
        </button>
      </div>
    </div>
  );
}
