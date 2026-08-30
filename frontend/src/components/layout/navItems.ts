/**
 * 顶部导航项。全部为 SPA 页面（React Router 客户端路由），无 Go HTML 交互页。
 */
export type NavItem = { href: string; label: string };

export const NAV_ITEMS: NavItem[] = [
  { href: "/", label: "看板" },
  { href: "/events", label: "事件流" },
  { href: "/entities", label: "实体库" },
  { href: "/graph", label: "图谱" },
  { href: "/ladder", label: "涨停梯队" },
  { href: "/flashes", label: "快讯流" },
  { href: "/notes", label: "笔记" },
  { href: "/weekly", label: "周报" },
  { href: "/recon", label: "对账" },
  { href: "/trades", label: "交易" },
  { href: "/reviews", label: "复盘" },
  { href: "/chat", label: "AI 对话" },
  { href: "/settings", label: "设置" },
];
