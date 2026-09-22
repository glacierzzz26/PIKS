# PIKS 前端重构规范

## 项目背景
PIKS 是 A 股投资知识系统：快讯/涨停池 → 结构化事件与实体 → PostgreSQL → Markdown/Obsidian。
前端全量 React：只读页消费 PostgreSQL 投影数据；交互页（笔记/周报/交易/AI 对话/设置）经 `/api/v1` JSON 写接口操作业务。

## 技术栈
- 构建：Vite 5 + React 18 + TypeScript（纯客户端 SPA，静态产物 `frontend/dist/`）
- 路由：React Router v6（客户端路由；筛选状态写 URL query）
- 服务：nginx（`piks-gateway` 独立容器，生产网关，单入口 :8090）—— 服务 SPA 静态文件 + 反代 `/api/*` → `web:8090`
- 后端：Go web（`piks-web` 独立容器）监听 `0.0.0.0:8090`，**不发布宿主端口**（仅 Docker 私网内可路由，对外只经 gateway），不直接暴露局域网
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
- **导航骨架（`frontend/src/components/layout/navItems.ts`，单一真源）**：**今天**（首页 `/`，Sun，置顶不归组）/ **发现**（市场概况 `/market` · 涨停股 `/ladder` · 消息 `/events` · 热榜 `/hot-topics`）/ **研究**（研报 `/reports` · 个股分析 `/research` · 问 AI `/chat`）/ **交易**（交易与持仓 `/trades`）/ **复盘**（持仓诊断 `/reviews` · 周报 `/weekly` · 笔记 `/notes`）/ **系统**（设置 `/settings`）。共 **13 项**，标签一律白话、去黑话。分组仅视觉分隔、不可折叠。
- **隐藏管线内部（P6-2）**：`/entities` 实体库 · `/graph` 图谱 · `/recon` 对账 **移出侧栏**（路由保留），并入设置页「数据与运维」卡（`frontend/src/components/settings/OpsCard.tsx`）；⌘K 仍经 `/entities` 做实体名→个股跳转，故不可删路由。**快讯 `/flashes` 并入消息页第二个 tab**（`/events` 渲染 `pages/messages.tsx`，`?tab=important|flash` 写 URL query；`/flashes` 旧深链落快讯 tab）。
- **新手支持（P6-2）**：`frontend/src/lib/glossary.ts`（术语表单一真源）+ `components/ui/Term.tsx`（行内 tooltip）+ `pages/help.tsx`（`/help` 使用指南 + 词汇表）。新增用户可见文案**禁止**出现 `quote-collector`/`daily-review`/`crontab`/`幂等`/`reconcile`/`research-run:*`/`Fact ≠ Inference` 等实现黑话 —— 用白话，或经 `glossary.ts` 解释。
- **个股中心 `/stock/:code`**（不进侧栏，⌘K 可直达）：以 `code` 为主键、`entity` 为可选富化，聚合持仓/成交/深研/事件/笔记/行业/涨停；数据源 `GET /api/v1/stock/:code`。所有带 code 的入口（涨停股/交易表/持仓表/实体库/事件 affected/报告头/⌘K）导流至此。首屏含 **「买入前速评」**（P7，见下）与 **「当时在看什么」**（P6-4 决策记录）。
- **买入前速评（P7，`components/stock/StockPrebuy.tsx`）**：个股页首屏置顶，一键现场重跑一份新鲜体检（`profile=prebuy` + `quick:true`），**原地出卡不跳页**。内含 结论横幅（评分卡 `overall_label` + 风险等级 + `veto_buy` 一票否决）· 风险红线（`metrics.risk.items[]`，P7 首次上屏）· 财务估值（`metrics.financial`，P7 首次上屏）· 量价形态（`metrics.patterns`：60 日双轴图 + 规则标签）· AI 综合研判（有则显）。**结论全取自规则确定性**（评分卡 + 风险规则），AI 可选 —— `POST /api/v1/research-runs` 收 `quick:true` 随行落库（`research_runs.quick`，迁移 0015）→ 认领它的 `research-worker` 据此置 `Options.RequireSynthesis=false`，LLM 缺失/失败/超预算不 fail，出骨架报告 + 确定性结论。⚠️ **量价形态是规则判定（Inference）不是事实**，前端单独分区、「换手率仅流通口径（腾讯，拿不到同花顺自由流通口径）」如实标注。
- **研究档案 profile（`research/profiles/*.yaml`）**：`complete-stock`（full，11 节含 `industry`，最重）· `short-term`（express，7 节，仅 2 维评分不含 risk）· **`prebuy`**（express，除 `industry` 外全要，保留完整 6 维评分卡含 risk —— 去掉最重的行业采集以提速，供买入前速评）。⚠️ 行为由 `sections` 驱动，`mode` 字段不参与分支。
- **决策记录（P6-4，史诗闭环 #1「我为什么买它」）**：零 schema，复用多态 `relationships` —— 边形 `(from_type='trade', from_id=<trades.id UUID>, to_type='research_run'|'event'|'personal_note', to_id=<UUID>, rel_type='based_on')`。**`to_id` 必须是各表主键 UUID（研报用 `research_runs.id`，绝不用 `run_id` TEXT）**——无 FK，接错静默悬空。写入：`POST /api/v1/trades` 收 `based_on:{run_ids,event_ids,note_ids}`；读回：`apiTrade.based_on[]` + `/stock/:code` 顶层 `decisions`；前端 `TradeBasedOnPicker`（`TradeAddForm` 内）/ `TradeTable` 展开行 / `StockDecisions`。⚠️ **图谱隔离**：`ListAllRelationships` 用 `graphRelFilter`（`internal/store/relationships.go`）排除 `decided_by`/`based_on`，否则决策边漏进 `/graph` 成无名节点；新增决策边类型须同步列入该过滤。
- **自选（`entities.status`）**：`watch` = 在自选 / `active` = 库中有不在自选 / `archived` = 曾自选已移出（保留历史/深研/笔记）。自选首页数据源 `GET /api/v1/watchlist`；引入/移出**只经同花顺自选截图镜像同步**（`/trades` 页截图导入 `kind='watchlist'`，服务端 diff add/keep/remove，无手动星标）。
- **热榜（数据分层 D 层，issue #68）**：页 `/hot-topics`（「发现」组），数据源 `GET /api/v1/hot-topics`，**2 源分列**（同花顺话题榜 15 条 / 财联社首页热文 13 条）。🔴 **不进事件链路**：独立表 `hot_topic_items`（迁移 0018）+ 独立接口 `HotTopicSource`（`internal/collector/hottopic.go`，**刻意不复用** `Driver`）+ 独立常驻进程 `cmd/hot-topic`（盘中每 30 分钟）—— 热榜**可被操纵**，与印证度（`source_count`/`cluster_sources`）**正交**，**不得合并计算**，只作展示。⚠️ 两源**不合并/不加权/不排名**（实测 615 对同题 = 0，粒度不同）；`rank` = 上游数组下标 + 1、跳行留空号、**不得重排**（财联社数组顺序 ≠ `readNum` 降序）；`hot_value` **`NULL ≠ 0`**（前端显示 `—`）；SSR 漂移**报错不空成功**（#64 教训）。设计 `docs/phase11/design/hot-topic.md`。
- **全部页面（`frontend/src/pages/`，React Router 注册）**：今天（自选）`/` / 市场概况 `/market` / 消息 `/events`（含快讯 tab、`/events/:id` 详情抽屉兜底）/ 实体库 `/entities`（含 `/entities/:id` → `?id=` 重定向）/ 图谱 `/graph` / 涨停股 `/ladder` / **热榜 `/hot-topics`** / 研报 `/reports`（列表，主体为轴；含 `/reports/:runId` 阅读器）/ 个股分析 `/research`（触发入口 + 全库历史表；含 `/research/:runId` 运维仪表盘）/ 使用指南 `/help` / 对账 `/recon` / 持仓诊断 `/reviews` / 笔记（列表 + 新建 `/notes/new` + 阅读 `/notes/:id` + 编辑 `/notes/:id/edit`）/ 周报 `/weekly` / 交易与持仓 `/trades`（手动录入 + 截图导入 + AI 解读 + 组合诊断）/ 问 AI `/chat` / 设置 `/settings` / 个股中心 `/stock/:code`；`/flashes` 亦渲染消息页（保留旧深链）。
- 写操作经 `/api/v1` JSON 写接口（`internal/web/api_write.go`）；nginx 仅反代 `/api/*` + 服务 SPA 静态文件，无交互页反代
- 分流规则见 `configs/nginx.conf`（生产）与 `frontend/vite.config.ts`（dev proxy 复刻）
- ⚠️ **`UpsertEntity` 状态语义**：空 `Status` = 保持既有（entity-build 每日 upsert 不带 status，若把空当 `active` 会清空自选）

## 分支与发布纪律

> 通用规则见全局 `~/.claude/CLAUDE.md`（dev 为唯一源 / master 只接受 dev 同步 / 发布带版本+hash）。以下为 PIKS 的实现。

### 分支

- **`dev` = 唯一主开发线**。一切功能/修复从 `dev` 拉分支，PR 合回 `dev`。
- **`master` = 发布镜像线**。唯一合法变更 = `dev` → `master` 同步（merge 或快进）。**禁止**从 feature/bugfix/release 分支直接合入 master，禁止在 master 上直接提交（发布标记除外）。
- **反向分叉是异常**：2026-09-16 曾出现 dev 缺 P7/P8、master 缺 P9 的「各缺一半」（见 `docs/进度总表.md` / PR #20）。若再现，先查根因、补完并记录，此后不允许反向分叉。

### 版本号

- **默认版本恒为 `v0.0.0`** —— 未发版的一切构建/部署都用它。**不要**因「改动挺大」就造号。
- **正式发版**才打语义化 tag `v<主>.<次>.<修>`（在 master 上，tag 名即版本号，附发布记录）。
- 生产镜像标识 **`<版本>-<短hash>`**，两者缺一不可：光有 hash 不表意、无法沟通版本先后。
- 镜像内烘焙：`PIKS_VERSION` + `PIKS_GIT_SHORT`（`scripts/deploy.sh` 的 `--build-arg`）。

### 部署

- 编排在 dev 侧：`./scripts/deploy.sh`（**四镜像**分别 build → 按 tag 跳过/传输 → compose 同步 → `up postgres` → `migrate` → `up web research collector hot-topic` → `up gateway` → 落 stack 清单）。
- **四镜像**（2026-09-20 容器拆分，issue #47）：`piks-gateway`（纯 nginx，唯一对外）/ `piks-web`（纯 Go API，无 python3）/ `piks-research`（Python 运行时 + 深研队列 worker，常驻）/ `piks-tools`（**10** 管线命令，profile=run；常驻的 `collector`/`hot-topic` 服务**复用**它）。单 Dockerfile 多 `--target`。
- **各镜像版本 = 自己输入的 git tree hash**（非 HEAD hash）：改前端不动 web/tools/research 的 tag → 它们既不重建也不传输。tag 形如 `v0.0.0-<该镜像短hash>`。⚠️ `TAG_TOOLS` 取 **10 个命令依赖闭包的并集**（`go_deps_hash_all`），不是只 hash `migrate` —— 否则改 `internal/collector` 不动 tag，生产跑旧像。
- **顺序是硬约束**：`migrate` 必须先于 `web`（`cmd/web/main.go` 读 `app_config`，缺表即 fatal 崩溃循环）；`gateway` 必须最后。`hot-topic` 依赖迁移 0018 的 `hot_topic_items` 表，故也排在 `migrate` 之后。
- **干净树门控**：`deploy.sh` 默认拒绝脏树（镜像从工作树构建却以版本 tag 命名）；调试用 `PIKS_ALLOW_DIRTY=1`（tag 附 `-dirty`）。
- ⚠️ **预算护栏 `ai_daily_token_budget` 的 0 = 「护栏关闭」**（不是「不限预算」，issue #75）：为 0 时 `cmd/cluster`/`cmd/worker` 打 WARN 并记 `task_runs.meta.guard_disabled=true`，**不拦任何 LLM 调用**；须经 `/settings` 设为非 0（建议 `1000000`）。日账本口径 = **北京午夜**（`config.BeijingMidnight`），不是 UTC。改 `cmd/cluster` 的收敛/预算行为须同改 `docs/phase2/design/cluster-quality.md` §3.4。
- **发布前必打 tag**；未发版也须在验证记录里写明 `v0.0.0-<hash>`（栈清单记四镜像 tag）。
- 每次部署**保留回滚镜像** `<name>:rollback-pre-<阶段>`，并在文档登记旧 hash。⚠️ **回滚快照只打一次、存在即跳过**（回滚点是一次性的；无脑 `tag latest` 会在第二次部署时把回滚点静默改写成新镜像，名字不变、内容已换，真回滚时才发现回滚不了）。
- ⚠️ **深研不在 web 进程内跑**：web 只建 `pending` 行 + `NOTIFY`，`piks-research` 的 `research-worker` 认领执行（`migrations/0015`、`internal/store/research_queue.go`）。web 容器无 python3，**任何新增的 web 内 `os/exec python3` 都会复现 2026-09-12 事故**（`check-research-isolation.sh` 白名单守卫）。

## 禁用清单
- 禁止直接改 PostgreSQL schema（前端重构不涉及）
- 禁止引入与现有栈冲突的 UI 库
- 禁止破坏性 API 变更
- 禁止在核心数字区使用 skeleton loader
