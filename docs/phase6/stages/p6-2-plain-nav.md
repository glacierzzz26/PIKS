# P6-2 白话导航 + 隐藏管线 + 词汇表 + 文案 —— 实现与验收

> 阶段:前端决绝重构 P6 第 2 阶段。设计依据 `docs/phase6/design/ux-ia.md` §1/§5。
> 目标:让新手看得懂 —— 侧栏去黑话、管线内部藏起来、页面文案改白话、术语有处可查、
> 空态能指路;并把失效的 e2e 冒烟脚本修成可跑的 SPA 回归网。

## 交付

### 1. 白话导航（`components/layout/navItems.ts` 单一真源）
- 侧栏 **14 → 11 项**,按用户闭环分组:`今天`(置顶) / 发现 / 研究 / 交易 / 复盘 / 系统。
- 标签去黑话:涨停梯队→**涨停股**、事件流→**消息**、个股分析→**研究报告**、AI 对话→**问 AI**、
  复盘→**持仓诊断**、交易→**交易与持仓**。分组仅视觉分隔、不可折叠。
- **移出侧栏（路由保留）**:`/entities` 实体库、`/graph` 图谱 → 并入 `/settings` 新增「数据与运维」卡
  (`components/settings/OpsCard.tsx`);`/recon` 对账维持已降级。保留路由因 ⌘K 经 `/entities` 做实体名→个股跳转。

### 2. 快讯并入消息页（`pages/messages.tsx` 新）
- 「消息」= **重要消息(事件) + 快讯**双 tab,tab 状态写 URL query(`?tab=important|flash`,规范第 7 条)。
- 原 `pages/events.tsx` / `pages/flashes.tsx` 抽出为 `EventsTab` / `FlashesTab`;`components/events/EventTable.tsx` 抽出共用表。
- `/flashes` 旧深链落到快讯 tab;`/events/:id` 详情抽屉兜底不变 —— P5 深链零断链。

### 3. 词汇表 + 使用指南
- `lib/glossary.ts`(新):16 条白话术语(快讯/消息/实体/置信度/事实·推断·信念/涨停股/封单/情绪/研究报告/**机检**/持仓诊断/快照/笔记/决策记录/自选)。
- `components/ui/Term.tsx`(新):行内术语 tooltip(纯 CSS popover,lucide `HelpCircle`,无 JS 状态)。
- `pages/help.tsx`(新,路由 `/help`,静态无 API):四步走通闭环 + 名词解释全表。`messages` 页导流至此。

### 4. 文案白话化（去管线黑话）
重写全部 `.psub` / `.hint` / 页脚 / 空态文案,清掉 `quote-collector` / `daily-review` / `market-state` /
`crontab` / `幂等可重跑` / `reconcile` / `Fact ≠ Inference ≠ Belief` / `Fact 区·Opinion 区` / `affects` /
`research-run:*` 这类实现黑话:
- `dashboard.tsx`:提示改「仅在交易日更新 / 每日收盘后更新 / PIKS 自动生成」;「管线状态」→**「数据更新状态」**;
  hero 抬头改「市场概况」,导语改白话。
- `analyst.tsx` 页名改「研究报告」,去「Fact / Opinion / 机检三域」;`recon.tsx` 去 `reconcile`;
  `stock/[code].tsx` 去 `research ·` 与 `affects`;`notes/weekly/entities/messages/reviews/ladder/watchlist/chat` 同步改。
- **研究报告三分区标题**:`一、Fact 区`→「一、事实（机器算的）」、`二、Opinion 区`→「二、AI 研判（仅供参考）」、
  `三、机检详情`→「三、数字机检详情」;空态「应以 Fact 区确定性结论为准」→白话。
- **`dashboard/Pipeline.tsx` 重写**:原样暴露 `research-run:gather` 等内部命令名,现按 `TASK_LABEL` 映射为
  「研究报告 · 取数」等白话名 + lucide 状态图标(替换彩色圆点)。顺带修 React 重复 key(同命令名可多次出现,key 加索引)。
- `AppShell` 页脚去 `Fact ≠ Inference ≠ Belief`,改「不预测涨跌、不给买卖建议 · 数据缺失如实留白,宁缺毋假」。

### 5. KPI 归「我」（`internal/web/api_v1.go`）
- 看板 KPI 由管线规模(结构化事件/统一实体/知识笔记/交易记录)改为**我的**:`我的自选 / 持仓股票 / 我的笔记 / 交易记录`。
- 复用既有查询(`ListEntitiesByStatus("watch")` + `LatestPositions`),**零新 store 方法**;顺带清掉不再使用的 `store.Counts` 调用。

### 6. e2e 冒烟脚本修复（`frontend/scripts/e2e_check.mjs`）
- 原脚本已失效:仍断言 Go HTML 交互页(`kind:"go"`),且缺 chromium 依赖。
- 重写为**全 SPA** 冒烟:16 条路由(含 P5 深链 `/flashes`/`/events/:id`/`/entities/:id` 与 P6-2 保留路由);
  **后端可达性自动探测**——可达则断言数据加载完成,不可达则仅断言 SPA 壳 + 无 console 异常(导航回归网仍有效);
  **导航遍历**:侧栏各项逐个点进,断言 SPA 壳挂载。
- 图谱断言改 SVG(自绘力导非 echarts canvas)。`package.json` 加 `npm run e2e`。

## 验收

| 项 | 结果 |
|---|---|
| `tsc --noEmit`（`npm run lint`） | ✅ |
| `npm run build` | ✅（JS 950.95 kB / gzip 305.90,CSS 41.72 kB）|
| `go build ./...` / `go vet ./...` / `go test ./...` | ✅ 全过 |
| **e2e 冒烟（本地 PG :5433 + Go :8090,vite :3100）** | ✅ **28 通过 / 0 失败** |
| 侧栏项数 | ✅ 11 项,`nav` 断言「侧栏可点 11 项」|
| P5 深链仍可达 | ✅ `/flashes`→消息页快讯 tab;`/stock/600519`/`/entities/:id`/`/notes/new` 等壳正常 |
| 页面无「加载中…」残留(全量档) | ✅ 16/16(含 `/graph`)|
| console 零错误 | ✅ 修掉 `Pipeline` 重复 key 后 16/16 |
| KPI 为「我的」 | ✅ `curl /api/v1/dashboard .stats` = 我的自选/持仓股票/我的笔记/交易记录 |

## 偏差与遗留
- 生产 `ai_model_vision` 为空 → 截图导入在生产暂不可用;「同步自选截图」CTA 的**失败如实报错 + 指向 `/settings`**
  在 P6-3 引导式首页里落实(本阶段未触及)。
- `Term.tsx` 与词汇表已就绪,但**行内 tooltip 的埋点**只落在少量页面(`messages` 导流 `/help`);逐页埋 `<Term>` 待 P6-3/后续按需补。
- e2e 依赖本机系统库(libnspr4/libnss3/libasound)。本机无 sudo,验收时以
  `LD_LIBRARY_PATH=/tmp/pwlibs/x/usr/lib/x86_64-linux-gnu`(本地解包 .deb)运行;CI/lab 需 `npx playwright install --with-deps`。
