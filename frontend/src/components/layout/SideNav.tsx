"use client";

import { Link, useLocation } from "react-router-dom";
import { useEffect, useState, useCallback } from "react";
import {
  Sun,
  Moon,
  Search,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import { PRIMARY_NAV, NAV_GROUPS, type NavItem } from "./navItems";

const COLLAPSE_KEY = "piks-nav-collapsed";

/** 侧边导航：今天置顶 + 分组平铺 10 页 + 搜索/主题/折叠，可折叠给宽表格/图谱让宽 */
export default function SideNav({ onOpenPalette }: { onOpenPalette: () => void }) {
  const { pathname } = useLocation();
  const [dark, setDark] = useState(false);
  const [today, setToday] = useState("");
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    setDark(document.documentElement.getAttribute("data-theme") === "dark");
    setToday(new Date().toLocaleDateString("zh-CN", { month: "2-digit", day: "2-digit" }));
    try {
      setCollapsed(localStorage.getItem(COLLAPSE_KEY) === "1");
    } catch {}
  }, []);

  const toggleTheme = useCallback(() => {
    const next = !dark;
    setDark(next);
    document.documentElement.setAttribute("data-theme", next ? "dark" : "light");
    try {
      localStorage.setItem("piks-theme", next ? "dark" : "light");
    } catch {}
  }, [dark]);

  const toggleCollapse = useCallback(() => {
    setCollapsed((c) => {
      const next = !c;
      try {
        localStorage.setItem(COLLAPSE_KEY, next ? "1" : "0");
      } catch {}
      return next;
    });
  }, []);

  const isActive = (href: string) =>
    href === "/" ? pathname === "/" : pathname.startsWith(href);

  const Item = ({ item }: { item: NavItem }) => {
    const Icon = item.icon;
    return (
      <Link
        to={item.href}
        title={collapsed ? item.label : undefined}
        className={`side-item ${isActive(item.href) ? "on" : ""} ${
          collapsed ? "justify-center px-0" : ""
        }`}
      >
        <Icon size={16} strokeWidth={2} className="side-ico" />
        {!collapsed && <span>{item.label}</span>}
      </Link>
    );
  };

  return (
    <aside className={`side-nav ${collapsed ? "collapsed" : ""}`}>
      <Link to="/" className={`side-brand ${collapsed ? "justify-center px-0" : ""}`}>
        <span className="side-logo" aria-hidden="true">
          <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="#fff"
            strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M3 3v18h18" />
            <path d="M7 14l4-4 3 3 5-6" />
          </svg>
        </span>
        {!collapsed && <b className="side-wordmark">PIKS</b>}
      </Link>

      {/* 搜索 + 主题切换：置顶，紧跟品牌 */}
      <div className="side-actions">
        <button
          onClick={onOpenPalette}
          title="查个股 / 搜索（⌘K）"
          className={`side-icon-btn ${collapsed ? "justify-center px-0" : "flex-1 justify-start gap-2 px-2.5"}`}
        >
          <Search size={14} />
          {!collapsed && <span className="text-[12px]">查个股 / 搜索</span>}
          {!collapsed && <kbd className="side-kbd">⌘K</kbd>}
        </button>
        <button onClick={toggleTheme} aria-label="切换主题" className="side-icon-btn">
          {dark ? <Sun size={14} /> : <Moon size={14} />}
        </button>
      </div>

      <nav className="side-list">
        <Item item={PRIMARY_NAV} />

        {NAV_GROUPS.map((g) => (
          <div key={g.title} className="side-group">
            {collapsed ? (
              <div className="side-rule" />
            ) : (
              <div className="side-group-title">{g.title}</div>
            )}
            {g.items.map((it) => (
              <Item key={it.href} item={it} />
            ))}
          </div>
        ))}
      </nav>

      <div className="side-foot">
        {!collapsed && <span className="side-date num">{today}</span>}
        <button
          onClick={toggleCollapse}
          aria-label={collapsed ? "展开导航" : "折叠导航"}
          title={collapsed ? "展开导航" : "折叠导航"}
          className="side-icon-btn"
        >
          {collapsed ? <PanelLeftOpen size={14} /> : <PanelLeftClose size={14} />}
        </button>
      </div>
    </aside>
  );
}
