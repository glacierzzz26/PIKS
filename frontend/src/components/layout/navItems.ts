/**
 * 左侧导航项。全部为 SPA 页面（React Router 客户端路由），无 Go HTML 交互页。
 * 2026-09-14 P6-2 白话导航重排（设计 phase6/design/ux-ia.md §1）：
 *   按「用户的闭环」分组、标签去黑话 —— 我的 / 发现 / 研究 / 交易 / 复盘 / 系统。
 * 分组仅作视觉分隔，不可折叠 —— 保证任意页面一键直达。
 * 隐藏管线内部：实体库 /graph 与图谱 /entities **移出侧栏**（路由保留，并入设置页
 *   「数据与运维」卡；⌘K 仍经 /entities 做实体名→个股跳转）；快讯 /flashes 并为
 *   「消息」页的一个 tab，不再占侧栏位。
 */
import type { LucideIcon } from "lucide-react";
import {
  Sun,
  Newspaper,
  TrendingUp,
  LayoutDashboard,
  ClipboardCheck,
  FileText,
  NotebookPen,
  Wallet,
  MessageSquare,
  Settings,
  Microscope,
  BookOpen,
} from "lucide-react";

export type NavItem = { href: string; label: string; icon: LucideIcon };
export type NavGroup = { title: string; items: NavItem[] };

/** 今天（新首页）单独置顶，不归组 */
export const PRIMARY_NAV: NavItem = {
  href: "/",
  label: "今天",
  icon: Sun,
};

export const NAV_GROUPS: NavGroup[] = [
  {
    title: "发现",
    items: [
      { href: "/market", label: "市场概况", icon: LayoutDashboard },
      { href: "/ladder", label: "涨停股", icon: TrendingUp },
      { href: "/events", label: "消息", icon: Newspaper },
    ],
  },
  {
    title: "研究",
    items: [
      { href: "/reports", label: "研报", icon: BookOpen },
      { href: "/research", label: "个股分析", icon: Microscope },
      { href: "/chat", label: "问 AI", icon: MessageSquare },
    ],
  },
  {
    title: "交易",
    items: [{ href: "/trades", label: "交易与持仓", icon: Wallet }],
  },
  {
    title: "复盘",
    items: [
      { href: "/reviews", label: "持仓诊断", icon: ClipboardCheck },
      { href: "/weekly", label: "周报", icon: FileText },
      { href: "/notes", label: "笔记", icon: NotebookPen },
    ],
  },
  {
    title: "系统",
    items: [{ href: "/settings", label: "设置", icon: Settings }],
  },
];

/** 平铺顺序（含今天），供命令面板等按序展示 */
export const NAV_ITEMS: NavItem[] = [
  PRIMARY_NAV,
  ...NAV_GROUPS.flatMap((g) => g.items),
];
