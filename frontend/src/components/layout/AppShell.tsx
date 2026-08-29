"use client";

import { useState, useCallback, Suspense } from "react";
import TopNav from "./TopNav";
import CommandPalette from "./CommandPalette";

/** 全局壳：顶部玻璃导航 + ⌘K 命令面板（对齐现有前端 topbar 布局） */
export default function AppShell({ children }: { children: React.ReactNode }) {
  const [paletteOpen, setPaletteOpen] = useState(false);

  const openPalette = useCallback(() => setPaletteOpen(true), []);
  const closePalette = useCallback(() => setPaletteOpen(false), []);

  return (
    <div className="min-h-screen">
      <TopNav onOpenPalette={openPalette} />
      <main className="mx-auto max-w-[1400px] px-5 pb-16 pt-2">{children}</main>
      {paletteOpen && <CommandPalette onClose={closePalette} />}
    </div>
  );
}
