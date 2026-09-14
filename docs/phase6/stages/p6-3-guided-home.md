# P6-3 引导式首页 + 自选富化 —— 实现与验收

> 阶段:前端决绝重构 P6 第 3 阶段。设计依据 `docs/phase6/design/ux-ia.md` §2/§3③。
> 目标:打开首页即知「今天该看什么 / 我的票怎么样」——编排层是引导,自选表是内容。

## 交付

### 1. 后端:`GET /api/v1/watchlist` 富化（零 schema）
- 每项新增:`latest_event`（最近一条 affects 到该实体的事件:标题/发生日/累计条数/深链 id）、
  `position_date`（持仓快照日）、`has_research` / `latest_research_asof`。
- 顶层新增:`position_date`（全部持仓共用的最新快照日）、`researched`（做过深研的数量 → 覆盖率）。
- **零新 store 方法以外的 schema 变更**:新增 1 个批量 read `ListEventsByEntityIDs`
  （`internal/store/events.go`,复用多态 `relationships`,一次查喂满首页,免 N 次往返）;
  其余复用 `ListEntitiesByStatus` / `LatestPositions` / `ListResearchRuns`。
- 排序:稳定按「有新消息 → 未深研过 → 其余」(`watchNeedRank`),首页取 Top 5。
- 抽出纯函数 `groupWatchEvents` / `watchNeedRank` 便于 DB-free 回归
  （`internal/web/watchlist_enrich_test.go`,2 例）。

### 2. 前端:首页改引导式（`pages/watchlist.tsx` → `pages/home.tsx`,编排 ~85 行）
- 编排四段 + 页脚,均拆子组件（组件 <150 行）:
  | 区块 | 组件 | 数据 |
  |---|---|---|
  | 页头:日期 + 自选/持有计数 | `home.tsx` | `/watchlist` |
  | 市场概览条 | `watch/WatchOverview`（复用） | `/dashboard.market` |
  | **今天该看什么** | `home/TodayFocus`（新） | `/watchlist` 富化 |
  | **我的自选**:持有中 / 观察中 | `watch/WatchGroups`（新,编排） | `/watchlist` |
  | 快速开始（3 步） | `home/QuickStart`（新） | 静态 |
  | 页脚:查不在自选里的票 | `home.tsx` | 静态 |
- **数据诚实硬约束**:无实时行情源 —— 盈亏列只在「持有中」组出现,标题带「盈亏截至 {快照日} 快照」;
  观察中组不渲染价格列（避免 `—` 堆成噪声）。
- **行徽标**（`watch/WatchTable` 重写）:新消息（红,lucide Newspaper,点进 `/events/:id`）/
  还没深研（琥珀）/ 深研 {日期};都没有如实 `—`。
- **首次引导**:空自选 = 新手实际第一屏（`FirstRun`）——「还没有自选股 / 来自同花顺截图 / 三步同步」+ 快速开始。
- **T1/T3 诚实分级**:只做 T1（纯前端概览句）+ T2（富化徽标）;
  「买入后出了新报告」这类依赖决策边的 T3 不在本阶段伪造（P6-4+ 落）。

### 3. 截图导入失败如实报错 + 指路（缺陷修复）
- `api_write.go`:错误文案去内部配置键,改「截图识别需要先在「设置」里配置 AI 与视觉模型。也可以先用「手动录入」。」
- `ImportFlow.tsx`:错误含「配置/设置」时附「去设置 →」链接到 `/settings`(不静默、不误导)。

## 验收

| 项 | 结果 |
|---|---|
| `go build ./...` / `go vet ./...` / `go test ./...` | ✅ 全过(新增 `watchlist_enrich_test.go` 2 例)|
| `tsc --noEmit`（`npm run lint`） | ✅ |
| `npm run build` | ✅（JS 959.93 kB / gzip 308.66）|
| `curl /api/v1/watchlist` 形状对齐 `types.ts` | ✅（置 2 只 watch 实测:`latest_event`/`position_date`/`has_research` 有值;测后 DB 还原）|
| 首页渲染（有自选） | ✅ 页头/市场条/今天该看什么/持有中·观察中/快速开始/页脚全渲染,console 0 错误 |
| 首页空自选 = 首次引导 | ✅ 渲染「还没有自选股」+ 三步引导 |
| 盈亏带「截至 {快照日}」 | ✅ 持有中组标题标注;无持仓则不出现百分比 |
| e2e 冒烟 | ✅ **28 通过 / 0 失败**（本地 PG:5433 + Go:8090 + vite:3100）|
| 截图 CTA 失败指路 `/settings` | ✅（文案 + 链接;生产 `ai_model_vision` 空时的路径）|

## 偏差与遗留
- **富化查询为全量扫描**:`ListResearchRuns(ctx, "", 0)` 取全部 run、`LatestPositions` 取最新快照,
  当前数据量下无问题;规模上来后应改为按 code 批量过滤(留给性能优化,非正确性问题)。
- 生产自选实测为空(`/watchlist` 返回 `{items:[]}`),故首页在生产是**首次引导屏**;
  有自选数据的完整渲染以本地 DB 置 watch 实测覆盖(已还原)。
- ⌘K 的首页个股查询框(设计 §1)未新增——现首页无独立搜索框,可发现性由侧栏「查个股 / 搜索」+ ⌘K 承担;
  若需要独立输入框留作后续。
