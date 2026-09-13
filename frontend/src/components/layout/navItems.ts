/**
 * 左侧导航项。全部为 SPA 页面（React Router 客户端路由），无 Go HTML 交互页。
 * 2026-09-13 个股轴心重排（设计 phase5/design/frontend-ia.md）：
 *   自选（首页）/ 研究 / 发现 / 复盘 / 交易 / 系统。
 * 分组仅作视觉分隔，不可折叠 —— 保证任意页面一键直达。
 * 对账（/recon）降为设置页子入口，侧栏不再占位。
 */
import type { LucideIcon } from "lucide-react";
import {
  Star,
  Newspaper,
  Boxes,
  Share2,
  TrendingUp,
  Zap,
  LayoutDashboard,
  ClipboardCheck,
  FileText,
  NotebookPen,
  Wallet,
  MessageSquare,
  Settings,
  Microscope,
} from "lucide-react";

export type NavItem = { href: string; label: string; icon: LucideIcon };
export type NavGroup = { title: string; items: NavItem[] };

/** 自选单独置顶（新首页），不归组 */
export const PRIMARY_NAV: NavItem = {
  href: "/",
  label: "自选",
  icon: Star,
};

export const NAV_GROUPS: NavGroup[] = [
  {
    title: "研究",
    items: [{ href: "/research", label: "个股分析", icon: Microscope }],
  },
  {
    title: "发现",
    items: [
      { href: "/market", label: "市场看板", icon: LayoutDashboard },
      { href: "/ladder", label: "涨停梯队", icon: TrendingUp },
      { href: "/flashes", label: "快讯流", icon: Zap },
      { href: "/events", label: "事件流", icon: Newspaper },
      { href: "/graph", label: "图谱", icon: Share2 },
      { href: "/entities", label: "实体库", icon: Boxes },
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
    items: [{ href: "/trades", label: "交易", icon: Wallet }],
  },
  {
    title: "系统",
    items: [
      { href: "/chat", label: "AI 对话", icon: MessageSquare },
      { href: "/settings", label: "设置", icon: Settings },
    ],
  },
];

/** 平铺顺序（含自选），供命令面板等按序展示 */
export const NAV_ITEMS: NavItem[] = [
  PRIMARY_NAV,
  ...NAV_GROUPS.flatMap((g) => g.items),
];
