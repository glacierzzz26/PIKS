# PIKS 前端重构规范

## 项目背景
PIKS 是 A 股投资知识系统：快讯/涨停池 → 结构化事件与实体 → PostgreSQL → Markdown/Obsidian。
前端全量 React：只读页消费 PostgreSQL 投影数据；交互页（笔记/周报/交易/AI 对话/设置）经 `/api/v1` JSON 写接口操作业务。

## 技术栈
- 构建：Vite 5 + React 18 + TypeScript（纯客户端 SPA，静态产物 `frontend/dist/`）
- 路由：React Router v6（客户端路由；筛选状态写 URL query）
- 服务：nginx（生产网关，单入口 :8090）—— 服务 SPA 静态文件 + 反代 `/api/*`
- 后端：Go web 只监听 `127.0.0.1:8090`（与 nginx 同容器），不直接暴露局域网
- 样式：Tailwind CSS **只做布局**（flex/grid/gap/px）；视觉一律走 `globals.css` 语义类（单系统 token）
- 图表：ECharts 5（`echarts/core` 按需注册，仅 bar + grid + tooltip，禁全量 import）
- 图标：lucide-react（线性图标；禁 emoji）
- 关系图谱：自绘 SVG 力导（`ForceGraph`，无第三方图库）
- 表格：原生 `<table class="table">`（无表格库）
- 动画：纯 CSS（keyframes/transition，无动画库）
- 数据获取：REST API，base URL 默认相对 `/api/v1`（生产同源走 nginx、开发经 vite proxy），可用 `VITE_API_BASE_URL` 覆盖

## 设计令牌（Design Tokens）
> **单一来源 = `frontend/src/globals.css` 的 `:root` / `[data-theme="dark"]`。** 下表为当前实测值;改令牌只改 CSS 变量,组件类不动。Tailwind `tailwind.config.ts` 的颜色/圆角映射亦指向此处。

### 配色（浅色 / 暗色两套）
- 背景：`--bg` #f4f6fb / #0e141b
- 卡片：`--card` #ffffff / #17202c
- 边框：`--line` #e7ebf2 / #263140
- 主色：`--accent`（=品牌 `--brand`）#28457e / #6d8ff7
- 品牌渐变：`--brand-a` → `--brand-b`（暗色另有值,保白字可读）
- 涨红（A 股习惯）：`--red` #e0392b / #f36b6f
- 跌绿（A 股习惯）：`--green` #1f9d57 / #3ec98d
- 文字：`--ink` #1c2433（禁止纯黑 #000000）/ `--muted` / `--ink-faint`

### 字体
- 界面：系统栈（-apple-system / PingFang SC / Microsoft YaHei）
- 数字：`--font-mono` 等宽 + tabular-nums + 右对齐（`.num`）

### 尺寸
- 圆角：**3 档 token** —— `--radius-card` 16px（卡）/ `--radius-md` 10px（控件）/ `--radius-sm` 6px（小标签）；Tailwind 映射 `{ lg:16px, DEFAULT:10px, sm:6px }`。**禁止散落魔法值**（如 `rounded-[9px]`）
- 阴影：`--shadow`（卡片）/ `--shadow-lg`（浮层、按钮），禁止重阴影
- 表格行高：44-52px
- 间距：8px 网格基准
- 触摸目标最小：44x44px

## 强制规则
1. 涨跌色必须是 A 股习惯（涨红跌绿），禁止欧美配色
2. 数字必须等宽、右对齐、tabular-nums
3. 图标统一用 Lucide / Heroicons 线性图标，禁止 emoji
4. 布局用非对称栅格，禁止 3 栏等宽
5. 组件单文件不超过 150 行，JSX 超 80 行必须拆子组件
6. 函数逻辑超 50 行必须抽 custom hook
7. 所有筛选状态必须反映到 URL query（可分享）
8. 全局 ⌘K 命令面板：跳转实体 / 切换页面 / 执行 cmd
9. 所有异步操作必须处理 loading / error / empty 三态
10. 禁止紫粉渐变、禁止 playful 字体、禁止 AI 套话文案

## 页面模块（2026-08-30 起：全部页面由 React SPA 提供，无 Go HTML；2026-09-14 P6-2 白话导航 + 隐藏管线内部）
- **导航骨架（`frontend/src/components/layout/navItems.ts`，单一真源）**：**今天**（首页 `/`，Sun，置顶不归组）/ **发现**（市场概况 `/market` · 涨停股 `/ladder` · 消息 `/events`）/ **研究**（研报 `/reports` · 个股分析 `/research` · 问 AI `/chat`）/ **交易**（交易与持仓 `/trades`）/ **复盘**（持仓诊断 `/reviews` · 周报 `/weekly` · 笔记 `/notes`）/ **系统**（设置 `/settings`）。共 **12 项**，标签一律白话、去黑话。分组仅视觉分隔、不可折叠。
- **隐藏管线内部（P6-2）**：`/entities` 实体库 · `/graph` 图谱 · `/recon` 对账 **移出侧栏**（路由保留），并入设置页「数据与运维」卡（`frontend/src/components/settings/OpsCard.tsx`）；⌘K 仍经 `/entities` 做实体名→个股跳转，故不可删路由。**快讯 `/flashes` 并入消息页第二个 tab**（`/events` 渲染 `pages/messages.tsx`，`?tab=important|flash` 写 URL query；`/flashes` 旧深链落快讯 tab）。
- **新手支持（P6-2）**：`frontend/src/lib/glossary.ts`（术语表单一真源）+ `components/ui/Term.tsx`（行内 tooltip）+ `pages/help.tsx`（`/help` 使用指南 + 词汇表）。新增用户可见文案**禁止**出现 `quote-collector`/`daily-review`/`crontab`/`幂等`/`reconcile`/`research-run:*`/`Fact ≠ Inference` 等实现黑话 —— 用白话，或经 `glossary.ts` 解释。
- **个股中心 `/stock/:code`**（不进侧栏，⌘K 可直达）：以 `code` 为主键、`entity` 为可选富化，聚合持仓/成交/深研/事件/笔记/行业/涨停；数据源 `GET /api/v1/stock/:code`。所有带 code 的入口（涨停股/交易表/持仓表/实体库/事件 affected/报告头/⌘K）导流至此。首屏含 **「当时在看什么」**（P6-4 决策记录）。
- **决策记录（P6-4，史诗闭环 #1「我为什么买它」）**：零 schema，复用多态 `relationships` —— 边形 `(from_type='trade', from_id=<trades.id UUID>, to_type='research_run'|'event'|'personal_note', to_id=<UUID>, rel_type='based_on')`。**`to_id` 必须是各表主键 UUID（研报用 `research_runs.id`，绝不用 `run_id` TEXT）**——无 FK，接错静默悬空。写入：`POST /api/v1/trades` 收 `based_on:{run_ids,event_ids,note_ids}`；读回：`apiTrade.based_on[]` + `/stock/:code` 顶层 `decisions`；前端 `TradeBasedOnPicker`（`TradeAddForm` 内）/ `TradeTable` 展开行 / `StockDecisions`。⚠️ **图谱隔离**：`ListAllRelationships` 用 `graphRelFilter`（`internal/store/relationships.go`）排除 `decided_by`/`based_on`，否则决策边漏进 `/graph` 成无名节点；新增决策边类型须同步列入该过滤。
- **自选（`entities.status`）**：`watch` = 在自选 / `active` = 库中有不在自选 / `archived` = 曾自选已移出（保留历史/深研/笔记）。自选首页数据源 `GET /api/v1/watchlist`；引入/移出**只经同花顺自选截图镜像同步**（`/trades` 页截图导入 `kind='watchlist'`，服务端 diff add/keep/remove，无手动星标）。
- **全部页面（`frontend/src/pages/`，React Router 注册）**：今天（自选）`/` / 市场概况 `/market` / 消息 `/events`（含快讯 tab、`/events/:id` 详情抽屉兜底）/ 实体库 `/entities`（含 `/entities/:id` → `?id=` 重定向）/ 图谱 `/graph` / 涨停股 `/ladder` / 研报 `/reports`（列表，主体为轴；含 `/reports/:runId` 阅读器）/ 个股分析 `/research`（触发入口 + 全库历史表；含 `/research/:runId` 运维仪表盘）/ 使用指南 `/help` / 对账 `/recon` / 持仓诊断 `/reviews` / 笔记（列表 + 新建 `/notes/new` + 阅读 `/notes/:id` + 编辑 `/notes/:id/edit`）/ 周报 `/weekly` / 交易与持仓 `/trades`（手动录入 + 截图导入 + AI 解读 + 组合诊断）/ 问 AI `/chat` / 设置 `/settings` / 个股中心 `/stock/:code`；`/flashes` 亦渲染消息页（保留旧深链）。
- 写操作经 `/api/v1` JSON 写接口（`internal/web/api_write.go`）；nginx 仅反代 `/api/*` + 服务 SPA 静态文件，无交互页反代
- 分流规则见 `configs/nginx.conf`（生产）与 `frontend/vite.config.ts`（dev proxy 复刻）
- ⚠️ **`UpsertEntity` 状态语义**：空 `Status` = 保持既有（entity-build 每日 upsert 不带 status，若把空当 `active` 会清空自选）

## 禁用清单
- 禁止直接改 PostgreSQL schema（前端重构不涉及）
- 禁止引入与现有栈冲突的 UI 库
- 禁止破坏性 API 变更
- 禁止在核心数字区使用 skeleton loader
