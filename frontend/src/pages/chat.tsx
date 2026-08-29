"use client";

import { Lock } from "lucide-react";
import { useData } from "@/hooks/useData";
import { ENDPOINTS } from "@/lib/api";
import type { ChatMsg } from "@/lib/mock/trading";
import { CHAT_HISTORY } from "@/lib/mock/trading";
import { Chip } from "@/components/ui/Num";

/** AI 对话（只读；对齐 dev /chat）：历史消息与引用展示，提问/上传留在 Go 端 */
export default function Page() {
  const chat = useData<ChatMsg[]>({
    path: ENDPOINTS.chat,
    fallback: () => CHAT_HISTORY,
  });
  const msgs = chat.data ?? [];

  return (
    <div>
      <div className="mb-1 mt-5">
        <div className="flex items-baseline gap-3">
          <h1 className="mb-0 text-2xl font-bold tracking-wide">AI 对话</h1>
          <span className="text-[13px] text-muted">
            问答带知识库引用 · 历史只读
          </span>
        </div>
      </div>

      <div className="mx-auto mt-4 flex max-w-[860px] flex-col gap-3.5">
        <div className="flex items-center justify-center gap-1.5 rounded border border-dashed border-line bg-card-soft py-2.5 text-xs text-muted">
          <Lock size={12} />
          只读视图：发送消息 / 上传截图请在 Go 端 /chat 操作
        </div>

        {msgs.map((m, i) => (
          <div
            key={i}
            className={`max-w-[86%] rounded-sm border px-3.5 py-2.5 ${
              m.role === "user"
                ? "self-end border-accent bg-accent-soft"
                : "self-start border-line bg-card"
            }`}
          >
            <div className="num mb-1 text-left text-[11px] text-muted">
              {m.role === "user" ? "我" : "AI"} · {m.time}
            </div>
            <div className="whitespace-pre-wrap break-words text-sm leading-relaxed">
              {m.content}
            </div>
            {m.refs && (
              <div className="mt-2 flex flex-wrap items-center gap-1.5">
                {m.refs.map((r) => (
                  <Chip key={r} tone="dim">
                    {r}
                  </Chip>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
