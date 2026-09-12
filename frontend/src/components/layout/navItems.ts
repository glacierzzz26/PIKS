/**
 * 左侧导航项。全部为 SPA 页面（React Router 客户端路由），无 Go HTML 交互页。
 * 顺序按 2026-09 全站重设计定稿，把 13 项按用途分成 4 组平铺展示
 * （分组仅作视觉分隔，不可折叠 —— 保证任意页面一键直达）。
 */
import type { LucideIcon } from "lucide-react";
import {
  LayoutDashboard,
  Newspaper,
  Boxes,
  Share2,
  TrendingUp,
  Zap,
  ClipboardCheck,
  FileText,
  NotebookPen,
  Wallet,
  Scale,
  MessageSquare,
  Settings,
} from "lucide-react";

export type NavItem = { href: string; label: string; icon: LucideIcon };
export type NavGroup = { title: string; items: NavItem[] };

/** 看板单独置顶，不归组 */
export const PRIMARY_NAV: NavItem = {
  href: "/",
  label: "看板",
  icon: LayoutDashboard,
};

export const NAV_GROUPS: NavGroup[] = [
  {
    title: "数据",
    items: [
      { href: "/events", label: "事件流", icon: Newspaper },
      { href: "/entities", label: "实体库", icon: Boxes },
      { href: "/graph", label: "图谱", icon: Share2 },
      { href: "/ladder", label: "涨停梯队", icon: TrendingUp },
      { href: "/flashes", label: "快讯流", icon: Zap },
    ],
  },
  {
    title: "复盘",
    items: [
      { href: "/reviews", label: "复盘", icon: ClipboardCheck },
      { href: "/weekly", label: "周报", icon: FileText },
      { href: "/notes", label: "笔记", icon: NotebookPen },
    ],
  },
  {
    title: "交易",
    items: [
      { href: "/trades", label: "交易", icon: Wallet },
      { href: "/recon", label: "对账", icon: Scale },
    ],
  },
  {
    title: "系统",
    items: [
      { href: "/chat", label: "AI 对话", icon: MessageSquare },
      { href: "/settings", label: "设置", icon: Settings },
    ],
  },
];

/** 平铺顺序（含看板），供命令面板等按序展示 */
export const NAV_ITEMS: NavItem[] = [
  PRIMARY_NAV,
  ...NAV_GROUPS.flatMap((g) => g.items),
];
