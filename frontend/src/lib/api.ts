/**
 * REST 数据层：前端只读 PostgreSQL 投影 API，不直接写业务。
 * base URL 默认相对路径 /api/v1：生产经 nginx 反代到 Go(:8090 同源)，
 * 开发经 vite proxy(configs/nginx.conf 复刻)。可用 VITE_API_BASE_URL 覆盖。
 * 请求失败自动降级为演示数据。
 */

export type FetchState<T> = {
  data: T | null;
  loading: boolean;
  error: string | null;
  /** true = 当前数据来自内置演示数据（后端不可达时的降级） */
  demo: boolean;
};

export const API_BASE = import.meta.env.VITE_API_BASE_URL || "/api/v1";

/** API 端点约定（供后端 cmd/web 增加只读投影接口时对齐） */
export const ENDPOINTS = {
  events: "/events", // GET ?type=&status=&q=&from=&to=
  entities: "/entities", // GET ?type=&q=
  relationships: "/relationships", // GET
  marketSnapshot: "/market/snapshot", // GET ?date=YYYY-MM-DD
  flashes: "/flashes", // GET ?q=&source=
  notes: "/notes", // GET
  note: "/notes/:id", // GET
  dashboard: "/dashboard", // GET
  recon: "/recon", // GET
  reviews: "/reviews", // GET
  trades: "/trades", // GET
  chat: "/chat", // GET
  settings: "/settings", // GET
  weekly: "/weekly", // GET
} as const;

export async function apiGet<T>(
  path: string,
  params?: Record<string, string | undefined>,
  signal?: AbortSignal
): Promise<T> {
  const url = new URL(API_BASE + path);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== "") url.searchParams.set(k, v);
    }
  }
  const res = await fetch(url.toString(), {
    signal,
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(`API ${res.status}: ${res.statusText}`);
  return res.json() as Promise<T>;
}
