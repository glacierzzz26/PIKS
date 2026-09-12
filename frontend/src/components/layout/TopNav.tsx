"use client";

import { Link, useLocation } from "react-router-dom";
import { useEffect, useState, useCallback } from "react";
import { Sun, Moon } from "lucide-react";
import { NAV_ITEMS } from "./navItems";

/** 顶部导航条：渐变 logo + 胶囊导航 + 主题切换 + ⌘K（对齐 HTML .nav） */
export default function TopNav({ onOpenPalette }: { onOpenPalette: () => void }) {
  const { pathname } = useLocation();
  const [dark, setDark] = useState(false);
  const [today, setToday] = useState("");

  useEffect(() => {
    setDark(document.documentElement.getAttribute("data-theme") === "dark");
    setToday(new Date().toLocaleDateString("zh-CN"));
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
      <div className="mx-auto flex h-[54px] max-w-[1440px] items-center gap-4 px-6">
        <Link to="/" className="flex shrink-0 items-center gap-2.5 no-underline">
          <span
            className="flex h-[30px] w-[30px] items-center justify-center rounded-[9px] shadow-[0_4px_10px_rgba(40,69,126,.3)]"
            style={{ background: "linear-gradient(135deg,#28457e,#3d6cb0)" }}
            aria-hidden="true"
          >
            <svg
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="#fff"
              strokeWidth="2.2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M3 3v18h18" />
              <path d="M7 14l4-4 3 3 5-6" />
            </svg>
          </span>
          <b className="text-[15px] tracking-[.5px] text-ink">PIKS</b>
        </Link>

        <nav className="flex flex-1 gap-0.5 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {NAV_ITEMS.map(({ href, label }) => (
            <Link
              key={href}
              to={href}
              className={`whitespace-nowrap rounded-[9px] px-[11px] py-1.5 text-[13px] font-medium no-underline transition-colors ${
                isActive(href)
                  ? "bg-accent-soft font-bold text-accent"
                  : "text-muted hover:bg-accent-soft hover:text-accent"
              }`}
            >
              {label}
            </Link>
          ))}
        </nav>

        <div className="ml-auto flex shrink-0 items-center gap-2.5">
          <span className="num hidden text-[12px] text-faint md:inline">
            {today}
          </span>
          <button
            onClick={onOpenPalette}
            className="flex items-center gap-[7px] rounded-[9px] border border-line bg-card px-2.5 py-[5px] text-[12px] text-faint hover:text-ink"
          >
            搜索
            <kbd className="rounded-[5px] border border-line bg-bg-soft px-[5px] py-0.5 font-mono text-[10px] font-semibold text-muted">
              ⌘K
            </kbd>
          </button>
          <button
            onClick={toggleTheme}
            aria-label="切换主题"
            className="flex h-[30px] w-[30px] items-center justify-center rounded-[9px] border border-line bg-card text-muted hover:text-accent"
          >
            {dark ? <Sun size={14} /> : <Moon size={14} />}
          </button>
        </div>
      </div>
    </header>
  );
}
