# PIKS Frontend

PIKS 的呈现层:**Vite 5 + React 18 + TypeScript 纯客户端 SPA**。
消费 Go 的 `/api/v1` JSON 接口(只读投影 + 写接口),静态产物 `frontend/dist/` 由 nginx 服务。

> 完整架构见 `../docs/架构总览.md`;前端规范见根目录 `../CLAUDE.md`。

## 技术栈

| 用途 | 选型 | 备注 |
|---|---|---|
| 构建 | Vite 5 | dev 端口 **3100**;无代码分割(全部路由静态 import) |
| 框架 | React 18 + TypeScript(strict) | `tsconfig` **未开** `noUnusedLocals` / `noUncheckedIndexedAccess` |
| 路由 | React Router v6 | 单 `BrowserRouter`,27 条路由;筛选态写 URL query |
| 样式 | Tailwind(**只做布局**)+ `src/globals.css`(**唯一视觉源**) | 3 档圆角 token;A 股涨红跌绿 |
| 图表 | ECharts 5 | `echarts/core` 按需注册(`BarChart`/`LineChart`/`Grid`/`Tooltip`/`LabelLayout`/`CanvasRenderer`),**禁全量 import** |
| 图谱 | 自绘 SVG 力导 | `components/graph/useForceSim.ts`,无第三方图库 |
| 图标 | lucide-react | 线性图标,**禁 emoji** |
| Markdown | react-markdown + remark-gfm | 无 heading id,故页面自行按 `##` 切章并注入锚点(`lib/report.ts`) |
| E2E | Playwright | 经 `scripts/e2e_check.mjs` 调用 |

## 运行

```bash
npm install
npm run dev      # http://localhost:3100(代理 /api → localhost:8090)

npm run lint     # tsc --noEmit
npm run build    # 产物 dist/
npm run e2e      # node scripts/e2e_check.mjs(需本地 PG + Go :8090)
```

dev 需先起 Go 后端(`../bin/web`,:8090)与 PostgreSQL(:5433)。

## 数据源

`API_BASE = import.meta.env.VITE_API_BASE_URL || "/api/v1"`(`src/lib/api.ts`)。
**默认相对路径** —— 生产同源走 nginx,dev 走 vite proxy(`vite.config.ts` 单条 `"/api" → http://localhost:8090`)。

- **无演示数据兜底**:后端不可达即报错,不做 mock 降级(数据诚实)。
- **无重试、无超时**;失败尽力读 `body.error`。
- **分页是纯客户端**(`hooks/usePagedQuery.ts`),API 不收 `page`/`size`。

### 端点(`src/lib/api.ts` 的 `ENDPOINTS`,27 键)

| 端点 | 方法 / 查询 |
|---|---|
| `/events` | GET `?type=&status=&q=&from=&to=` |
| `/entities` | GET `?type=&q=&status=` |
| `/relationships` | GET |
| `/market/snapshot` | GET `?date=` |
| `/flashes` | GET `?q=&source=` |
| `/notes` · `/notes/:id` | GET · GET/PUT/DELETE |
| `/dashboard` `/watchlist` `/recon` `/reviews` `/account` | GET |
| `/trades` · `/trades/import` · `/trades/confirm` | GET/POST |
| `/stock/:code` | GET(调用方自行 `.replace(":code", code)`) |
| `/chat` · `/chat/clear` | GET/POST |
| `/settings` · `/settings/form` | GET/POST |
| `/weekly` · `/weekly/detail` · `/weekly/generate` | GET/POST |
| `/research-runs` · `/research-runs/:runId` | GET/POST |

## 目录结构

```
src/
  App.tsx            路由表(27 条)+ ShellLayout
  main.tsx           入口
  globals.css        唯一视觉源(2,091 行语义类 + token)
  pages/             27 个页面(含 note/[id]/edit、stock/[code]、m/upload 等子目录)
  components/        graph/ stock/ trades/ report/ research/ weekly/ reviews/
                     dashboard/ events/ home/ ladder/ watch/ chat/ settings/
                     layout/ md/ note/ charts/ ui/
  hooks/             useData(唯一 fetch 原语) / usePagedQuery / useUrlState
                     useResearchRun / usePrebuy / useResearchDelta / useTradeImport
  lib/               api.ts / types.ts(666 行,全领域类型) / format.ts
                     chartTheme.ts / image.ts / constants.ts / glossary.ts(术语表)
                     report.ts / reportList.ts / research.ts
```

## 关键约定

- **页面路由名 ≠ 文件名**:`/research` → `pages/analyst.tsx`;`/events` → `pages/events.tsx` 而 `/flashes` → `pages/messages.tsx`。
- **`/entities` `/graph` `/recon` 不在侧栏**,唯一应用内入口 = `components/settings/OpsCard.tsx`。
- **导航单一真源** = `components/layout/navItems.ts`(12 项)。
- **`useResearchRun` 的 `profile` 必填** —— 隐式默认会静默造出遗留 `complete-stock` run。
- **`hooks/useData` 是唯一 fetch 原语**(`AbortController` 清理,`AbortError` 静默不报错)。
- **三态纪律**:统一用 `components/ui/States.tsx` 的 `LoadingBlock` / `ErrorState` / `EmptyState`;
  核心数字区**不用 skeleton loader**。
- **`"use client"` 是惰性残留**(纯 Vite 构建无 RSC),无需模仿。
- **力导图 ref 内的 Map 不得在 effect 内清空**,否则节点全落 (0,0)(`useForceSim.ts` 有注释)。

## 已知技术债

登记于 `../docs/架构总览.md` §11,摘要:

- 组件超 150 行:`NoteForm.tsx`(200)、`CommandPalette.tsx`(152)。
- 45 处 `rounded-[…]` 魔法值(39 处等价于 DEFAULT token,应用 `rounded`)。
- `pages/graph.tsx` 筛选态用局部 `useState`,**不可分享 URL**。
- `pages/chat.tsx:55` 错误气泡用了 ⚠️ emoji(全站唯一用户可见违例)。
- 无代码分割;118 个文件顶部 `"use client"` 可整批清理。
