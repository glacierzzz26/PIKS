"use client";

import { Link, useLocation } from "react-router-dom";
import { useEffect, useState, useCallback } from "react";
import { Search, Sun, Moon } from "lucide-react";
import { NAV_ITEMS } from "./navItems";

/** 顶部导航条：品牌 + 横向导航 + 主题切换 + ⌘K（对齐 base.css .topbar） */
export default function TopNav({ onOpenPalette }: { onOpenPalette: () => void }) {
  const { pathname } = useLocation();
  const [dark, setDark] = useState(false);

  useEffect(() => {
    setDark(document.documentElement.getAttribute("data-theme") === "dark");
  }, []);

  const toggleTheme = useCallback(() => {
    const next = !dark;
    setDark(next);
    document.documentElement.setAttribute(
      "data-theme",
      next ? "dark" : "light"
    );
    try {
      localStorage.setItem("piks-theme", next ? "dark" : "light");
    } catch {}
  }, [dark]);

  const isActive = (href: string) =>
    href === "/" ? pathname === "/" : pathname.startsWith(href);

  return (
    <header className="topbar-glass fixed inset-x-0 top-0 z-40 border-b border-line">
      <div className="mx-auto flex h-[58px] max-w-[1400px] items-center gap-7 px-5">
        <Link to="/" className="flex items-baseline gap-2 no-underline">
          <span className="text-lg font-extrabold tracking-wide text-accent">
            PIKS
          </span>
          <span className="hidden text-[13px] text-muted sm:inline">
            投资知识系统
          </span>
        </Link>

        <nav className="flex flex-1 gap-1 overflow-x-auto">
          {NAV_ITEMS.map(({ href, label }) => (
            <Link
              key={href}
              to={href}
              className={`whitespace-nowrap rounded-sm px-3.5 py-1.5 text-[13.5px] font-medium transition-colors no-underline ${
                isActive(href)
                  ? "bg-accent-soft text-accent"
                  : "text-muted hover:bg-bg-soft hover:text-ink"
              }`}
            >
              {label}
            </Link>
          ))}
        </nav>

        <button
          onClick={onOpenPalette}
          className="flex h-9 items-center gap-2 rounded-sm border border-line bg-card px-3 text-[13px] text-muted hover:text-ink"
        >
          <Search size={14} />
          <kbd className="font-mono text-2xs">⌘K</kbd>
        </button>

        <button
          onClick={toggleTheme}
          aria-label="切换主题"
          className="flex h-9 w-9 items-center justify-center rounded-sm border border-line bg-card text-ink transition-transform hover:rotate-12"
        >
          {dark ? <Sun size={15} /> : <Moon size={15} />}
        </button>
      </div>
    </header>
  );
}
