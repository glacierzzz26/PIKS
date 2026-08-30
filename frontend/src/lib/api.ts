/**
 * REST 数据层：前端只读 PostgreSQL 投影 API + JSON 写 API。
 * base URL 默认相对路径 /api/v1：生产经 nginx 反代到 Go(:8090 同源)，
 * 开发经 vite proxy(configs/nginx.conf 复刻)。可用 VITE_API_BASE_URL 覆盖。
 * 请求失败如实抛错（前端显 error 态），不做演示数据降级。
 */

export type FetchState<T> = {
  data: T | null;
  loading: boolean;
  error: string | null;
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
  note: "/notes/:id", // GET/PUT/DELETE
  dashboard: "/dashboard", // GET
  recon: "/recon", // GET
  reviews: "/reviews", // GET
  trades: "/trades", // GET/POST
  chat: "/chat", // GET/POST
  settings: "/settings", // GET/POST
  weekly: "/weekly", // GET
  weeklyDetail: "/weekly/detail", // GET ?offset=
  weeklyGenerate: "/weekly/generate", // POST ?offset=
  tradesImport: "/trades/import", // POST multipart(type+file)
  tradesConfirm: "/trades/confirm", // POST
  settingsForm: "/settings/form", // GET
  chatClear: "/chat/clear", // POST
} as const;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(API_BASE + path, init);
  if (!res.ok) {
    let msg = `API ${res.status}: ${res.statusText}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* 非 JSON 错误体，保留默认文案 */
    }
    throw new Error(msg);
  }
  return res.json() as Promise<T>;
}

export function apiGet<T>(
  path: string,
  params?: Record<string, string | undefined>,
  signal?: AbortSignal
): Promise<T> {
  // request() 内部已拼 API_BASE;这里只处理 query string。
  // 注意:不能用 new URL(相对路径) —— 浏览器对相对 URL 构造会抛 Invalid URL。
  let p = path;
  if (params) {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined && v !== "") qs.set(k, v);
    }
    const s = qs.toString();
    if (s) p += `?${s}`;
  }
  return request<T>(p, {
    signal,
    headers: { Accept: "application/json" },
  });
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function apiPut<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function apiDelete<T>(path: string): Promise<T> {
  return request<T>(path, { method: "DELETE" });
}

/** multipart 上传（截图导入 / AI 对话图片）。FormData 由调用方构造。 */
export function apiUpload<T>(path: string, form: FormData): Promise<T> {
  return request<T>(path, { method: "POST", body: form });
}
