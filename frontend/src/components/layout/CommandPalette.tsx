"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { Search, CornerDownLeft, RefreshCw } from "lucide-react";
import { MOCK_ENTITIES } from "@/lib/mock/entities";
import { NAV_ITEMS } from "./navItems";

type Item = {
  key: string;
  label: string;
  hint: string;
  run: () => void;
};

/** 全局 ⌘K 命令面板：跳转页面 / 跳转实体 / 刷新数据（规范第 8 条） */
export default function CommandPalette({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const [input, setInput] = useState("");
  const [cursor, setCursor] = useState(0);

  const items = useMemo<Item[]>(() => {
    const pages: Item[] = NAV_ITEMS.map((n) => ({
      key: `page:${n.href}`,
      label: n.label,
      hint: "页面",
      run: () => router.push(n.href),
    }));
    const entities: Item[] = MOCK_ENTITIES.slice(0, 12).map((e) => ({
      key: `ent:${e.id}`,
      label: e.name,
      hint: "实体",
      run: () => router.push(`/entities?id=${e.id}`),
    }));
    const cmd: Item[] = [
      {
        key: "cmd:refresh",
        label: "刷新数据",
        hint: "命令",
        run: () => router.refresh(),
      },
    ];
    return [...pages, ...entities, ...cmd];
  }, [router]);

  const filtered = useMemo(() => {
    const q = input.trim().toLowerCase();
    if (!q) return items.slice(0, 10);
    return items
      .filter((i) => i.label.toLowerCase().includes(q))
      .slice(0, 10);
  }, [items, input]);

  useEffect(() => setCursor(0), [input]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setCursor((c) => Math.min(c + 1, filtered.length - 1));
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setCursor((c) => Math.max(c - 1, 0));
      }
      if (e.key === "Enter" && filtered[cursor]) {
        filtered[cursor].run();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [filtered, cursor, onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/25 pt-[18vh]"
      onClick={onClose}
    >
      <div
        className="w-[520px] overflow-hidden rounded border border-line bg-card shadow-pop"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex h-12 items-center gap-3 border-b border-line px-4">
          <Search size={15} className="text-muted" />
          <input
            autoFocus
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="跳转页面、实体，或执行命令…"
            className="w-full bg-transparent text-[13px] outline-none placeholder:text-muted"
          />
        </div>
        <ul className="max-h-[320px] overflow-auto p-1.5">
          {filtered.length === 0 && (
            <li className="px-3 py-6 text-center text-xs text-muted">
              没有匹配项
            </li>
          )}
          {filtered.map((item, i) => (
            <li
              key={item.key}
              onMouseEnter={() => setCursor(i)}
              onClick={() => {
                item.run();
                onClose();
              }}
              className={`flex h-10 cursor-pointer items-center gap-3 rounded px-3 text-[13px] ${
                i === cursor ? "bg-primary-soft text-primary" : ""
              }`}
            >
              {item.key.startsWith("cmd:") ? (
                <RefreshCw size={14} strokeWidth={1.8} />
              ) : (
                <span className="w-1.5" />
              )}
              <span className="flex-1">{item.label}</span>
              <span className="text-2xs text-muted">{item.hint}</span>
              {i === cursor && (
                <CornerDownLeft size={12} className="text-muted" />
              )}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}
