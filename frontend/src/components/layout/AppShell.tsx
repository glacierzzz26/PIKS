"use client";

import { useState, useCallback, useEffect } from "react";
import SideNav from "./SideNav";
import CommandPalette from "./CommandPalette";

/** 全局壳：左侧分组导航 + 内容区 + 页脚 + ⌘K 命令面板 */
export default function AppShell({ children }: { children: React.ReactNode }) {
  const [paletteOpen, setPaletteOpen] = useState(false);

  const openPalette = useCallback(() => setPaletteOpen(true), []);
  const closePalette = useCallback(() => setPaletteOpen(false), []);

  // 全局 ⌘K / Ctrl+K 打开命令面板（规范第 8 条）
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="flex items-start">
      <SideNav onOpenPalette={openPalette} />
      <div className="flex min-h-screen min-w-0 flex-1 flex-col">
        <main className="mx-auto w-full max-w-[1440px] px-6 pb-4">{children}</main>
        <footer className="footer">
          <div className="src">
            数据来源：东方财富 7×24 快讯 · 涨停池 · PIKS 自动整理 · PostgreSQL 唯一数据源
          </div>
          <div>不预测涨跌、不给买卖建议 · 数据缺失如实留白，宁缺毋假。</div>
        </footer>
      </div>
      {paletteOpen && <CommandPalette onClose={closePalette} />}
    </div>
  );
}
