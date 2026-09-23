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
  eventTypes: "/event-types", // GET 事件类型枚举 key+label（真源=后端，issue #61）
  board: "/board", // GET ?stage=early|late&date=YYYY-MM-DD 早/晚档榜单（issue #83 P-3）
  entities: "/entities", // GET ?type=&q=&status=
  relationships: "/relationships", // GET
  marketSnapshot: "/market/snapshot", // GET ?date=YYYY-MM-DD
  flashes: "/flashes", // GET ?q=&source=
  announcements: "/announcements", // GET ?q=&source= 公告（原始事件源，issue #50）
  hotTopics: "/hot-topics", // GET 热榜（issue #68 D 层；两源分列、不合并）
  notes: "/notes", // GET
  note: "/notes/:id", // GET/PUT/DELETE
  dashboard: "/dashboard", // GET
  watchlist: "/watchlist", // GET 自选聚合（设计 frontend-ia §2.3）
  recon: "/recon", // GET
  reviews: "/reviews", // GET
  account: "/account", // GET 最近账户资金汇总（issue #19）
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
  researchRuns: "/research-runs", // GET ?code=&entity=&limit= | POST {code,profile?,days?}
  researchRun: "/research-runs/:runId", // GET
  stock: "/stock/:code", // GET 个股中心聚合（设计 frontend-ia §2.4）
  authLogin: "/auth/login", // POST 登录（P12 / issue #78）
  authLogout: "/auth/logout", // POST 登出
  authMe: "/auth/me", // GET 登录探活
  healthz: "/healthz", // GET 存活探针（免鉴权）
} as const;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  // credentials: "include" —— 带上会话 cookie(P12 / issue #78)。同源单入口下
  // cookie 本就随请求走,这里显式声明以防将来跨子域部署时静默丢 cookie。
  const res = await fetch(API_BASE + path, { credentials: "include", ...init });
  if (res.status === 401) {
    // 会话缺失/过期 → 跳登录页并记住来路。用整页跳转(hard navigate)而非 SPA 内
    // navigate:api 层不持有 router,且登录后回跳最稳。
    const next = window.location.pathname + window.location.search;
    if (!next.startsWith("/login")) {
      window.location.href = `/login?next=${encodeURIComponent(next)}`;
    }
    throw new Error("未登录");
  }
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

// ==================== 访问控制（P12 / issue #78）====================

/**
 * 登录探活：返回是否已登录。**不抛错**（未登录返回 false）—— 供鉴权门首屏调用，
 * 401 不该被当成异常，也不该触发跳转（门自己决定跳不跳）。
 */
export async function fetchAuthed(): Promise<boolean> {
  try {
    const res = await fetch(API_BASE + "/auth/me", { credentials: "include" });
    if (!res.ok) return false;
    const body = await res.json();
    return body?.authed === true;
  } catch {
    return false;
  }
}

/** 登录：成功即落会话 cookie。失败抛「密码错误」等后端文案。 */
export function apiLogin(password: string): Promise<{ ok: boolean }> {
  return apiPost("/auth/login", { password });
}

/** 登出：清会话 cookie（幂等）。 */
export function apiLogout(): Promise<{ ok: boolean }> {
  return apiPost("/auth/logout");
}
