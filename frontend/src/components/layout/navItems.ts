/**
 * 顶部导航项。external = Go HTML 交互页（笔记/周报/交易/对话/设置），
 * 经 nginx 反代（见 configs/nginx.conf）渲染 Go 编辑页，需全量跳转而非 SPA 路由。
 */
export type NavItem = { href: string; label: string; external?: boolean };

export const NAV_ITEMS: NavItem[] = [
  { href: "/", label: "看板" },
  { href: "/events", label: "事件流" },
  { href: "/entities", label: "实体库" },
  { href: "/graph", label: "图谱" },
  { href: "/ladder", label: "涨停梯队" },
  { href: "/flashes", label: "快讯流" },
  { href: "/notes", label: "笔记", external: true },
  { href: "/weekly", label: "周报", external: true },
  { href: "/recon", label: "对账" },
  { href: "/trades", label: "交易", external: true },
  { href: "/reviews", label: "复盘" },
  { href: "/chat", label: "AI 对话", external: true },
  { href: "/settings", label: "设置", external: true },
];
