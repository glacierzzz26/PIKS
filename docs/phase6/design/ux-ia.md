# P6 前端决绝重构 + 投研闭环补齐(设计)

> 状态:**已定稿(2026-09-14),按此执行,改动需走变更**。范围:把前端呈现层(设计系统 + IA + 页面)决绝重建为专业、简约、清爽的个人投研平台,并补齐 epic [#1](https://github.com/glacierzzz26/PIKS/issues/1) 的四段投研闭环。契约依据:`frontend/src/components/layout/navItems.ts`(导航单一真源)、`frontend/src/App.tsx`(路由表)、`frontend/src/globals.css`(设计令牌)、`internal/web/{api_v1,api_watchlist,api_stock,trades}.go`、`internal/store/relationships.go`、`migrations/0010_trades.sql`、`0011_position_reviews.sql`、`0012_research_runs.sql`。前序:`docs/phase5/design/frontend-ia.md`(P5 个股轴心,已上生产 镜像 `c81c6d2`)。
>
> **定稿门结论(2026-09-14)**:§9.1 `/reviews` 改名「持仓诊断」✅;§9.2 `/flashes` 并入消息页 tab ✅;§9.6 顺手修 e2e 冒烟脚本 ✅。§9.3 echarts tree-shake / §9.4 `/help` 深度 / §9.5 图谱过滤 按推荐执行(见各节)。

---

## 1. 背景与现状

### 1.1 问题(用户 2026-09-14 原话)

> 「我还是想重构这个项目,按照一个专业的个人投研平台来组织,要求直观,方便易用,美观,简约,清爽,但是最重要的是专业和易用,对我这样的新手友好。」

四条痛点(用户多选,全中):**无从下手/导航看不懂 · 信息太散/找不到 · 视觉不够美观清爽 · 缺功能/闭环没打通**。

### 1.2 现状诊断

| 维度 | 事实 | 证据 |
|---|---|---|
| 导航 | 按**后端产物类型**分组(「发现」= 看板/涨停/快讯/事件/图谱/实体库),14 项平铺,无主轴;管线内部产物(实体库/图谱)是一等公民 | `navItems.ts` |
| 视觉 | **三套样式系统并存**:`globals.css` 1,735 行/~101 语义类 + Tailwind utilities + 39 处内联 `style={{var(--x)}}`;19 个类零引用;14 种圆角值;6 处硬编码浅色渐变在暗色下失真 | 实测 |
| 组件 | 6 页面超 150 行硬规则(settings 217 / weekly 219 / entities 203 / events 190 / trades 148 / dashboard 144)+ `ForceGraph(281)`/`NoteForm(200)`/`weekly Sections(175)`/`dashboard widgets(173)`/`CommandPalette(152)` | 实测 |
| 依赖 | `@tanstack/react-table`(796KB)、`@xyflow/react`(3.2MB)**零 import**;`framer-motion` 仅 1 文件;echarts 全量 import(bundle 1.63MB) | `package.json` + grep |
| 闭环 | 研究→决策→持仓链路缺失;复盘→沉淀断裂(见 §3) | 见 §3 |
| 数据诚实 | ladder 的 `turnover/float_mv/first_time/reason` 源数据未采集(恒零),UI 却渲染 `0.0%`/`0 亿` | `ladder.tsx:126-131`;`api_v1.go:528` |
| 文案 | 页头副标题泄漏管线黑话(`reconcile`/`quote-collector`/`crontab`/`幂等可重跑`/`daily-review`);`DOC_TYPE_LABEL` 缺 `belief`/`case`;Go 用户可见字符串含 ~30 个 `⚠️`/`✅` emoji(违 CLAUDE.md 禁 emoji) | `lib/format.ts:46`;`internal/web/{chat,trades,api_write}.go` |
| 文档 | `frontend/README.md` 仍描述已删除的 Next.js;`scripts/e2e_check.mjs` 仍断言 Go 渲染页(必失败) | 实测 |

### 1.3 探查后对「推倒重建」的修正(已与用户确认)

前端 3 周内重写 3 次(Go 模板 → Next.js → Vite SPA → 全站重设 → 个股轴心 IA)。**真正的风险不是「代码不够新」,而是再重写一次**。实测前端 **8,030 行 / 73 文件,60 个组件均值 ~70 行**;`useData`(`hooks/useData.ts`,48 行)是干净的统一三态 hook;`App.tsx` 67 行;`navItems.ts` 是声明式单一真源。缺陷全是**表层与缺口**而非架构。故定为:

> **决绝重构 = 重建全部呈现层(设计系统 + IA + 页面编排),移植数据层与业务逻辑。视觉与体验全新,不重写已能跑的代码。**

移植清单见 §6。

### 1.4 与既定方向的关系

`docs/项目详解.md` 一句话定位:**Knowledge System,不是 Trading System**。P6 不改变定位——引导式首页与闭环是让「我终于能解释为什么市场发生了这件事」**可被新手走到**,而非转向荐股/信号。红线(Fact ≠ Inference ≠ Belief、PG 唯一 Source of Truth、AI 不直接写库)不改。

---

## 2. 目标 IA(白话导航 + 隐藏管线内部)

导航单一真源仍是 `frontend/src/components/layout/navItems.ts`(`SideNav` 渲染、`CommandPalette` 枚举)。**14 → 11 项**,按**用户的闭环**分组:

```
我的     今天        /          (Home)          ← 引导式首页(= 自选 + 今天该看什么)
发现     市场概况    /market    (LayoutDashboard)
         涨停股      /ladder    (TrendingUp)    原「涨停梯队」
         消息        /events    (Newspaper)     原「事件流」;内含「快讯」tab
研究     研究报告    /research  (Microscope)    原「个股分析」
         问 AI       /chat      (MessageSquare) 原「AI 对话」
交易     交易与持仓  /trades    (Wallet)
复盘     持仓诊断    /reviews   (ClipboardCheck) 原「复盘」
         周报        /weekly    (FileText)
         笔记        /notes     (NotebookPen)
系统     设置        /settings  (Settings)
```

**移出侧栏(路由不变)**:

| 路由 | 原 | 新归属 |
|---|---|---|
| `/entities` 实体库 | 发现组 | `/settings`「数据与运维」卡片 + ⌘K |
| `/graph` 图谱 | 发现组 | `/settings`「数据与运维」卡片 + ⌘K |
| `/recon` 对账 | 已降级 | 维持(同卡片) |
| `/flashes` 快讯 | 发现组 | **`/events` 的第二个 tab**(「消息」页 = 重要消息 + 快讯);路由不变 |

**关键决定**:
- **不改任何路由名、不加重定向**。P5 刚建的深链全部保留,这是 churn 最小化的核心。
- **保留 `/entities` 路由**:⌘K 经它做「实体名 → `/stock/:code`」跳转(`CommandPalette.tsx:52`),删路由会打断搜索。**隐藏入口 ≠ 删除能力**。
- **`/stock/:code` 不进侧栏**(参数路由无法一键直达),但补可发现性:侧栏搜索按钮文案 `搜索` → `查个股 / 搜索`;⌘K 占位符 → `输入股票代码或名称…`;首页给显眼的个股查询框。
- **不追小侧栏**:痛点是`导航看不懂`而非`导航太长`。合并 复盘/周报/笔记 到 8 项只增 churn,不买账。
- **唯一新增路由**:`/help`(词汇表 + 使用指南,静态无 API)。

---

## 3. 闭环功能(epic #1 剩余四项,零 schema)

> **总原则**:全部复用既有表与 `relationships` 多态,P6 **不新增表、不改既有 schema**。(用户已授权新表;探查证明本轮无需,留作后续。)

### 3.1 决策记录(研究→决策→持仓)· 史诗闭环 #1「我为什么买它」· 零 schema

**已有**:`trades.note TEXT` + `trades.review JSONB`;`TradeAddForm` 已 POST `note`;`relationships` 多态表(无 FK,UNIQUE 5 元组,`migrations/0001_init.sql:89-103`);`CreateRelationship`;`RefPicker.tsx` 已为笔记选事件/实体。
**缺**:没记录**买入决策当时在看哪份研究/哪条事件**。

**方案**:复用 `relationships` 建决策边:
```
(from_type='trade', from_id=<trades.id UUID>, to_type='research_run'|'event'|'personal_note',
 to_id=<UUID>, rel_type='decided_by'|'based_on')
```
- ⚠️ **用 `research_runs.id`(UUID),绝不用 `run_id`(TEXT)**。`relationships.to_id` 是 UUID 且无 FK,接错会**静默悬空**。
- 后端:`POST /api/v1/trades` 收 `based_on:{run_ids[],event_ids[],note_ids[]}` → 逐条 `CreateRelationship`;`apiTrade` 加 `based_on[]`;个股中心聚合返回该股交易的决策边;新增 `ListRelationshipsFrom(fromType, fromID)`(近 `ListEntityRelationships`,`relationships.go:34`)+ 按 trade IDs 的批量变体。
- 前端:`RefPicker.tsx` 复用于 `TradeAddForm` 与交易行;`/stock/:code` 新增首屏区块「当时在看什么」。
- ⚠️ **同 commit 必修**:新 `from_type='trade'` 边会经 `ListAllRelationships`(`relationships.go:61`,无过滤)漏进 `/graph` 成**无名节点**。图谱两处查询(`ListAllRelationships` 消费方 + `ListGraphEdges`,`relationships.go:127`)加 `rel_type NOT IN ('decided_by','based_on')`,并断言节点/边数前后不变。
- **零增量**:`/trades` 录入已自动补公司实体(`api_write.go:227`),决策边即「决策→持仓」链,无需再关联 positions 快照(截图无身份)。

### 3.2 复盘→沉淀修复 · 史诗闭环 #3「我错在哪」· 零 schema

**两个已确认的断裂**(非风格意见):
1. `/api/v1/reviews` **用 risks+mistakes 算状态,却丢弃 mistakes**(`api_v1.go:868-888`)→ 复盘页永远看不到复盘点,只有交易行能看。
2. `tradeSaveMistakeCore` 建了笔记但**不建 relationships 边**(`trades.go:340-346`)→ 「存为笔记」的复盘结论**不进个股页「我的笔记」**(`ListNotesReferencingEntity` 按 entity 边查,`personal_notes.go:183`)。**沉淀从未回流**。

**方案**:
- 修①:`apiReview` 补出 `mistakes`(映射代码已在作用域内)。
- 修②:`tradeSaveMistakeCore` / `positionSaveRiskCore` 于 `CreatePersonalNote` 后,对该股/该组合的 code 补 `CreateRelationship(笔记→entity,'references')`(及笔记→event)。零 schema。
- 前端:`/reviews` 渲染 mistakes + 「存为笔记」;`StockNotes` 免费受益。

### 3.3 研究覆盖率 / 自选富化(支撑引导式首页)· 零 schema

- 后端:`GET /api/v1/watchlist` 每项加 `latest_event:{title,date,count}`、`position_date`、`has_research`、`latest_research_asof`。复用现成**批量** `ListEventsAffectingEntities`(`relationships.go:99`)+ `LatestPositions` + `ListResearchRuns`;**无新 store 方法**。
- 覆盖率口径:「组合 5 只持仓,3 只做过深研」。

### 3.4 研究 delta · 纯前端

- `/research-runs?code=X&limit=2`(summary 无 metrics)→ 取 top **2 个 done** run 的详情,diff `metrics.price.*` + `metrics.scorecard.overall`,渲染 `/stock/:code`「研究变化」。
- 价值较低(唯一价值不来自断链的闭环项),工期紧时先砍。

### 3.5 排序(值/工)

| 序 | 项 | 值 | 工 | schema | 理由 |
|---|---|---|---|---|---|
| 1 | 3.1 决策记录 | ★★★★★ | 中 | 无 | epic 唯一缺失的真闭环链 |
| 2 | 3.2 复盘→沉淀修复 | ★★★★☆ | 低 | 无 | 修真实断链,diff 小 |
| 3 | 3.3 覆盖率/富化 | ★★★★☆ | 低 | 无 | 双职:解锁引导式首页 T2 |
| 4 | 3.4 delta | ★★★☆☆ | 中 | 无 | 可砍 |

---

## 4. 引导式首页(`/` = 今天)

**核心决定:`/` 既是引导首页也是自选页**,不做「今天」与「我的票」两跳。自选表回答「我的票怎么样」,引导层是它的框。

### 4.1 数据诚实硬约束

**无实时行情源。** 现价/盈亏来自同花顺持仓截图快照,真实但**过期**。故:
- 首页**绝不出现无标注的 `+3.21%`**;每处盈亏带 `截至 {snapshot_date} 快照`。
- 不能说「今天涨了 X%」,只能说「最新快照 09-11,盈亏 +3.21%」。
- `/watchlist` 由 `LatestPositions`(单日 `max(snapshot_date)`,`trades.go:112`)派生 → **一个日期标签即诚实且充分**。

### 4.2 区块

| # | 区块 | 数据源 |
|---|---|---|
| 1 | 页头:日期 + 市场一句话 | `GET /dashboard` `.market` |
| 2 | 市场概览条(情绪/涨停/跌停/炸板/成交/指数) + 「详情 → /market」 | 同上(复用 `components/watch/WatchOverview.tsx`,单请求) |
| 3 | **「今天该看什么」** | P6-3 富化后的 `GET /watchlist` |
| 4 | **我的自选**:持有中 / 观察中两组 | `GET /watchlist`(分组 + 快照日标注) |
| 5 | 快速开始(3 按钮) | 同步自选截图 → `/trades?import=watchlist`;录一笔交易 → `/trades?panel=add`;问 AI → `/chat` |
| 6 | 底部:查不在自选里的票 | 复用现有页脚 |

**首页只请求 `/watchlist`(富化)+ `/dashboard` 两个端点**。无服务端分页,其余一律走链接,避免 4 份全表负载。

### 4.3 「今天该看什么」三层(诚实分级)

- **T1(纯前端)**:「你有 N 只票在自选,其中 M 只持仓。市场今天偏热(情绪 62),涨停 52 家,最多的是半导体。」
- **T2(P6-3 后端)**:每只自选徽标 —— `有 3 条新消息` / `上次深研 20 天前` / `持仓快照 5 天前` / `还没深研过` → 渲染「需要你处理的」卡片(Top 5 最陈旧/缺失)。
- **T3(P6-4+)**:「你买入时关联过报告,之后出了新报告」——依赖决策边,**不在 T2 伪造**。

### 4.4 空态 = 首次引导

生产 `/api/v1/watchlist` 现为 `{"items":[]}`,故这是新手实际见到的第一屏:

```
还没有自选股。PIKS 的自选来自你的同花顺自选截图 —— 不用手动维护第二份列表。
  ① 同花顺 → 自选股页截图(一屏截全)
  ② 到这里上传        [同步自选截图]
  ③ 回到本页,每只票点进去就是它的档案
[看一下 PIKS 能做什么] → /help
```

> ⚠️ **前置(非 UX 任务)**:生产 `ai_model_vision` 为空 → 截图导入在生产不可用(自选/交易均然)。「同步自选截图」CTA **失败时必须如实报错并指向 `/settings`**,不静默 400。这是本设计的硬边界,不掩盖。

---

## 5. 视觉系统:三套并存 → 一套

**做法:重写 `globals.css`(1,735 行 → ~400 行)为单一 token + 组件词表。** 确立规则:

> **Tailwind 只做布局(flex/grid/gap/px/max-w),视觉一律走 `globals.css` 语义类。**

- **保留词表**(承载 token 的部分):`.panel .panel-pad .page-head .psub .section .section-head .two-col .kpi-grid .hero .ent-grid .note-grid .rvrow .table .num .num-t .st .chip .btn .input .form-card .prose-piks .pager .side-* .filter-bar` 等。
- **删除 19 个零引用类**:`badge-warm card-title chip-accent chip-amber chip-dim chip-down chip-up expo-warn-line form-grid g-label gauge-sub import-banner num-left rv-point side-buy side-sell side-tag snap-grid topbar-glass`(`.chip-*` 族已由 `Chip`→`.st st-{tone}` 取代,保裸 `.chip`)。
- **圆角 14 值 → 3 token**:`--radius-card:16px / --radius-md:10px / --radius-sm:6px`;`tailwind.config.ts` 映射 `{sm:'6px',DEFAULT:'10px',lg:'16px'}`;机械替换(`rounded-[9px]`×26 等)。**同时改 CLAUDE.md**(现写 6px,与实际/文档均不符;定 **卡 16px** 为准,而非降级 UI——phase5 §7.5 已登记此债)。
- **修 6 处硬编码浅色渐变**(暗色 bug):`.pct-bar i`(`globals.css:788-793`)、`.expo-track i.ok/lead`、`.hero`、`.wk-head .btn` → 改 `linear-gradient(90deg, var(--brand-2), var(--brand))`(`--brand-2` 已存在且随主题)。
- **拆内联样式层**:39 处 `style={{color}}` 多为 `var(--ink-faint)` + `textAlign` → 抽 `.txt-faint` 类;删冗余 `style={{textAlign:"left"}}`(CSS 已承载,`WatchTable.tsx:14,25,31,48` 等)。
- **死依赖**:删 `@tanstack/react-table`(796KB)/`@xyflow/react`(3.2MB)(零 import,手写 `.table`/SVG 力导已稳);删 `framer-motion`(1 处 drawer → CSS transform/opacity);echarts 改 `echarts/core` + 按需注册(2 文件)。同步修 CLAUDE.md 技术栈。
- **`/market` KPI 归「我」**:`dashboard.stats` 现 结构化事件/统一实体/知识笔记/交易记录 → 改 自选数/持仓数/我的笔记/本周交易(违「隐藏管线内部」的现存项)。
- **超大文件拆分**(随各阶段编辑顺手拆):`ForceGraph(281)`→`useForceSim.ts`;其余抽子组件。

---

## 6. 新手引导机制(按 成本×效果)

| # | 机制 | 成本 | 落点 |
|---|---|---|---|
| 1 | **重写 `.psub`/`.hint` 文案**去管线黑话 | ~2h 机械 | 每页页头 |
| 2 | **`EmptyState` 加 action CTA**(现 15 处仅被动文案) | 1 组件 + ~10 调用点 | `ui/States.tsx` |
| 3 | **白话导航标签** | 1 文件 | `navItems.ts` |
| 4 | **`<Term>` 行内术语 tooltip**(lucide `HelpCircle` + CSS popover) | 1 组件 + 1 映射 | 涨停/事件/诊断/个股 |
| 5 | **`/help` 词汇表 + 使用指南**(共享 #4 映射) | 1 静态页 | 新路由 |
| 6 | **引导式首页 + 首次引导块** | P6-3 | `/` |

> 探查结论:#1、#2 单独一项 ROI 都高于引导式首页——**先做文案与空态**。

### 6.1 移植清单(保留逻辑,重贴样式,不重写)

`lib/{api,types,format,constants,chartTheme}.ts`;`hooks/*`(`useData`/`useUrlState`/`usePagedQuery`/`useResearchRun`/`useResearchTrigger`);`components/{stock,research,trades}/*` 业务逻辑;`ForceGraph` 的 SVG 力导逻辑。

---

## 7. 分阶段实施(每阶段独立可验收/可回滚)

仓库惯例:设计文档 → **定稿门** → 分阶段实现 → 归档 `docs/phase6/stages/` + 登记 `docs/进度总表.md` + `docs/phase6/design/README.md` 索引。dev 主线,`feat/*` 起于 dev,master 仅由 dev 合并。

### P6-1 地基与诚实:设计系统重建 + 数据诚实 + 死重清理 ★ 最先

**为何最先**:改 `globals.css`(后续每阶段渲染其上);修掉会被**新首页原样重新焊进去**的诚实 bug。

- `globals.css` 重写为单系统;圆角 3 token;删 19 死类;修 6 处暗色渐变;拆内联样式。
- 修缺陷:#3(Go emoji → 文案/lucide)、#4(ladder 恒零字段 → `—`)、#5(`DOC_TYPE_LABEL` 补 信念/案例)、#1(reviews 出 mistakes)。
- 删 3 死依赖;echarts tree-shake;拆 `settings.tsx` + `widgets.tsx`;更新 CLAUDE.md(圆角/技术栈/页面模块)。
- **验收**:`tsc --noEmit` + `npm run build` 过;bundle 体积前后记录;ladder 显 `—`;笔记筛选出「信念/案例」;`curl /api/v1/reviews | jq '.[0].mistakes'` 非 null。

### P6-2 白话导航 + 隐藏管线 + 词汇表 + 文案

- `navItems.ts` 按 §2 改标签/分组;实体库·图谱 → `/settings` 数据与运维;快讯 → 消息页 tab。
- 新 `/help` + `lib/glossary.ts`;`<Term>` 组件;重写全部 `.psub`/`.hint` 文案;`EmptyState` 加 action。
- **验收**:侧栏 11 项;P5 全部 15 深链仍 200;⌘K 可搜实体;`/help` 渲染。

### P6-3 引导式首页 + 自选富化

- 后端(可独立 curl 验):`/api/v1/watchlist` 富化(§3.3,零 schema)。
- 前端:`pages/watchlist.tsx` → `pages/home.tsx`(§4);`WatchTable` 分组 + 快照日。
- **验收**:`curl /watchlist` 形状;空自选渲染三步引导;盈亏带 `截至 MM-DD`;`/` 无未标注百分比;截图 CTA 失败时指向 `/settings`。
- **依赖**:§4.4 前置(vision 模型)不阻塞开发,但 CTA 文案须按失败路径设计。

### P6-4 决策记录闭环(研究→决策→持仓)

- 后端 §3.1 + ⚠️ **同 commit 加图谱过滤**。
- 前端:`RefPicker` 进 `TradeAddForm` 与交易行;`/stock/:code`「当时在看什么」。
- **验收**:`POST /trades {based_on:{run_ids:[X]}}` → `GET /stock/:code` 返回该边;`/graph` 节点/边数前后不变(回归断言);无关联交易如实空态。

### P6-5 复盘→沉淀 + 研究 delta + 收尾

- 后端 §3.2(笔记↔实体/事件边 + reviews 出 mistakes 前端消费)。
- 前端:`/reviews` 渲染 mistakes + 存为笔记;`/stock/:code`「研究变化」(§3.4)+ 覆盖率行。
- 收尾:`ForceGraph` → `useForceSim`;剩余超大文件拆分。
- **验收**:`/reviews` 存一条风险 → 该笔记出现在 `/stock/:code`「我的笔记」**且** `/notes`;delta 正确 diff 两个真实 run;归档 `docs/phase6/`。

---

## 8. 边界与诚实降级

| 情况 | 行为 |
|---|---|
| 无实时行情 | 首页不出现无标注涨跌幅;一律带快照日;不承诺「实时」 |
| 生产 vision 模型为空 | 截图 CTA 如实报错 + 指向 `/settings`;不静默失败、不伪造成功 |
| ladder 源字段缺失 | 恒零字段渲染 `—` 或删列;不把 0 当真实数字 |
| 无服务端分页 | 首页仅 2 请求;列表页客户端切片(现状) |
| `entities.detail->>'code'` 无索引 | 维持(量级小);如 hub 延迟出问题,独立迁移 `0013` 单独批准 |
| 同 code 多实体脏数据 | 维持 `GetCompanyEntityByCode` 取最早;不清理(独立任务) |
| `relationships` 无 FK | 新增边可能悬空;无 trade 删除端点,暴露低;登记不改 schema |
| 研究 delta 无第二个 done run | 如实空态「暂无历史报告可对比」,不伪造 |

---

## 9. 待确认的开放问题

1. **`/reviews` 改名「持仓诊断」**——其内容实为 `position_reviews`,与「复盘」闭环语义撞名。建议改名(免费),请确认。
2. **`/flashes` 合并入 `/events` tab**——路由保留,侧栏去位。若用户仍想侧栏直达快讯,可保留一项(则侧栏 12 项)。
3. **echarts tree-shake**——2 文件,预期大幅降 bundle;若图表有回归风险可延后。
4. **`/help` 的深度**——词汇表(必做,~15 词)vs 图文使用指南(可选)。建议先词汇表。
5. **P6-4 图谱过滤**——必须同 commit;若发现 `/graph` 另有消费方需一并过滤。
6. **e2e 回归网**——本仓库前端无单测,`e2e_check.mjs` 已失效。是否本轮修为可跑 SPA 冒烟(需 chromium 系统依赖 `libnspr4`,可能要 sudo)?建议做但列为可选项。

---

## 10. 验收方式

**每阶段**:`cd frontend && npm run lint`(`tsc --noEmit`)+ `npm run build`;`go build ./... && go vet ./... && go test ./...`;改动端点 `curl` 形状对齐 `types.ts`;**P5 全部 15 个深链仍 200**(防路由回归)。

**端到端(联调)**:`cd frontend && npm run dev`(:3100,proxy → Go :8090)+ 本地 PG(:5433)。逐条走查:首页引导 → 自选 → 个股中心「当时在看什么/研究变化」→ 交易录入带 based_on → 持仓诊断 → 复盘存为笔记 → 该笔记回流个股页与 `/notes`。

**闭环三条链(epic #1 验收)**:
1. 「我为什么买它」——交易↔研究/事件/笔记可回溯(决策边渲染);
2. 「它现在怎么样」——个股页最新深研 + 持仓盈亏同屏 + 快照日诚实标注;
3. 「我错在哪」——复盘风险/复盘点一键沉淀为笔记,且**回流对应个股页与笔记库**。

**文档**:`docs/phase6/design/ux-ia.md`(本文件,定稿门)→ `docs/phase6/stages/ux-ia.md`(实现验收)→ `docs/进度总表.md` 登记 → `frontend/README.md` 重写。
