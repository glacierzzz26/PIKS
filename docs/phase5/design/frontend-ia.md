# 前端信息架构重组设计文档（个股轴心的个人投研平台）

> 状态:**已实现并上生产(Phase 1–4,2026-09-13)**。范围:把 Web 信息架构从「按后端产物类型平铺」重组为**以个股为轴心**的个人投研平台——首页=自选列表,个股中心一站看全,市场数据退为「发现」辅助;引入自选股并以**同花顺自选截图镜像同步**。实现情况与偏差见 §7。
> **部署**:2026-09-13 经 master 发布线(`1cde70e` → merge → 空提交 `c81c6d2`)上生产 lab,migrate 0(无 schema 变更),15 页 + 新端点全 200;回滚镜像保留 lab `piks-tools:rollback-pre-ia`。⚠️ 生产 `ai_model_vision` 为空 → 截图导入暂不可用(自选/交易均然),待网关出视觉模型。
> 契约依据:`frontend/src/components/layout/navItems.ts`(导航单一真源)、`frontend/src/App.tsx`(路由表)、`internal/web/api_v1.go:668`(handleAPIDashboard 聚合先例)、`internal/web/api_write.go:283/368`(截图导入两段式 tradeImportAPI/tradeConfirmAPI)、`internal/web/trades.go:84/108`(importPrompt/buildImportPreview)、`internal/store/{entities,relationships,research_runs,trades,personal_notes,market_snapshots}.go`、`internal/model/model.go`(Entity.Status)、`migrations/0004_entities.sql`。

> ⚠️ **后续变更(issue #87,2026-09-22)**:自选同步**不再只走截图** —— 新增**服务器侧自动同步**
> `cmd/watch-sync`(每日 3 次拉同花顺「我的自选」),与本文 §2.6/§2.7 的截图镜像**并存**:
> 两路写**同一套** `EnsureCompanyEntity` + `SetEntityStatus`,成员资格真源仍是 `entities.status='watch'`
> (**§2.6 结论不变**)。截图路径降级为**兜底**(自动失灵时用)。**新增**:加入价/日入新表
> `watchlist_entries`(迁移 `0020`)—— 因 `entities.detail` 每轮被 entity-build 覆盖(即 §2.6.1 同一地雷的
> 另一面);截图路径**不含**价/日。设计见 `docs/phase11/design/watchlist-sync.md`。

---

## 1. 背景与现状

### 1.1 问题

用户(2026-09-13)原话:「我现在这个页面没有头绪,不知道怎么下手看这个项目,按照市场上常见的个人投研项目重新组织、设计,我希望这个项目真的有用。」

**现状诊断**(诊断依据:`navItems.ts` 当前结构 + 各页面路由):

- **导航按后端产物类型分组**(数据/复盘/交易/系统),14 个入口平铺,**没有主轴**。用户打开不知道该先看哪个。
- **缺个股纵轴**:同一个个股的信息**散落五个页面**——深研 `/research`、相关事件 `/events`、笔记 `/notes`、持仓 `/trades`、快讯 `/flashes`。看一只票要来回跳。
- **管线内部产物被当一等公民**:`实体库`/`图谱` 是数据加工的中间产物(实体/关系),用户日常并不查,却占据主导航。
- **无「我关注的票」概念**:`entities.status='watch'` 在 `migrations/0004_entities.sql` 已定义、`frontend/src/lib/types.ts:23` 已声明为 `"active"|"watch"|"archived"`、`pages/entities.tsx:139,175` 已渲染「关注/观察」徽标,但**全站无任何地方能设置、无 UI、无写接口**。
- **首页是市场看板**:看的是「今天市场如何」,而非「我关注的票如何」——对个人投研平台而言重心错了。

### 1.2 与既定方向的关系

用户 memory 已定:PIKS 从「工具集合」收敛为**个人投研平台**——以个股为主轴,串 研究→决策→持仓→复盘→沉淀 闭环(epic [glacierzzz26/PIKS#1](https://github.com/glacierzzz26/PIKS/issues/1))。本设计是该方向的前端落地,并覆盖 issue #1 候选工作项「个股中心统一视图」。

### 1.3 用户使用习惯(设计前提)

用户日常在**同花顺手机端**做买卖与维护自选。因此:

- PIKS 的自选**不是**一份需要手动维护的第二列表,而是**同花顺自选的镜像**。
- 同步走**截图识别**(与交易功能同一条已验证路径),零外部依赖,数据只落本地 PG。
- 手动星标**不做**(用户明确):自选唯一由截图镜像驱动,避免双份状态。

---

## 2. 方案

### 2.1 目标 IA(导航骨架)

导航单一真源仍为 `frontend/src/components/layout/navItems.ts`(`SideNav` 渲染、`CommandPalette` 枚举 `NAV_ITEMS`)。保持既定约束:**分组仅视觉分隔、不可折叠**(保证任意页面一键直达)。

```
自选        /watchlist              ← 新首页(Star)
研究        报告库   /research       (Microscope)
            研报详情 /research/:runId (参数页,不进侧栏)
发现        市场看板 /market         (LayoutDashboard,原 / 看板迁址)
            快讯流   /flashes        (Zap)
            涨停梯队 /ladder        (TrendingUp)
            事件流   /events        (Newspaper)
            图谱     /graph         (Share2)
            实体库   /entities      (Boxes)
复盘        复盘     /reviews        (ClipboardCheck)
            周报     /weekly         (FileText)
            笔记     /notes          (NotebookPen)
交易        交易     /trades         (Wallet)
系统        AI 对话  /chat           (MessageSquare)
            设置     /settings       (Settings)
            (对账 /recon 降为设置页子入口,侧栏不再占位)
```

- **`/` 不再是看板**:`/` → `<Navigate to="/watchlist" replace />`;看板搬到 `/market`。`SideNav.tsx` 的 `isActive` 特判(`href === "/" ? pathname === "/" : pathname.startsWith(href)`)与 `CommandPalette` 无需改逻辑,只改 `navItems.ts` 数据源。
- **`/stock/:code` 不进侧栏**(参数化页无法一键直达),但**进 `CommandPalette`**:输入股票代码/名称直接跳个股中心。
- **对账 `/recon` 降级**(用户确认):对账 = 每日核对「东财快讯 → AI 事件」链路的完整性(数据健康自检),日常不使用。移出侧栏,作为设置页下的子入口保留(路由不变)。

### 2.2 现有页面 → 新位置映射

| 现有页面 | 文件 | 新位置 | 变化 |
|---|---|---|---|
| 看板 | `pages/dashboard.tsx` | `/market`(发现组) | 仅路由改名;不再承担首页职责 |
| 个股分析师 | `pages/analyst.tsx` | `/research`(研究组) | 语义从「入口」降为「报告库/全量触发台」;个股入口改由 `/stock/:code` 承担 |
| 研报详情 | `pages/research.tsx` | `/research/:runId` | 不变 |
| 事件流 | `pages/events.tsx` | `/events`(发现组) | 表内 affected 实体名 → 链接 `/stock/:code` |
| 实体库 | `pages/entities.tsx` | `/entities`(发现组) | 有 code 的卡片主点击进 `/stock/:code`;`watch` 徽标复用 |
| 图谱 | `pages/graph.tsx` | `/graph`(发现组) | 公司节点点选面板加「打开个股中心」 |
| 涨停梯队 | `pages/ladder.tsx` | `/ladder`(发现组) | code chip → `/stock/:code` |
| 快讯流 | `pages/flashes.tsx` | `/flashes`(发现组) | 不变(时间轴视角,不挂个股轴) |
| 复盘 | `pages/reviews.tsx` | `/reviews`(复盘组) | 不变 |
| 周报 | `pages/weekly.tsx` | `/weekly`(复盘组) | 不变 |
| 笔记 | `pages/notes.tsx` + `note/**` | `/notes`(复盘组) | 笔记详情 refs 中实体 → `/stock/:code` |
| 交易 | `pages/trades.tsx` | `/trades`(交易组) | **截图导入加第三类「自选股」**;表内 code chip → `/stock/:code` |
| 对账 | `pages/recon.tsx` | `/recon`(设置子入口) | 移出侧栏 |
| AI 对话/设置 | `chat.tsx`/`settings.tsx` | 系统组 | 不变 |

**新增页面/路由**

| 路由 | 文件 | 说明 |
|---|---|---|
| `/watchlist` | `pages/watchlist.tsx` | 新首页:自选列表(分「持仓中 / 仅自选」两组)+ 顶部紧凑市场概览条 |
| `/market` | 复用 `pages/dashboard.tsx` | 看板迁址 |
| `/stock/:code` | `pages/stock/[code].tsx` | 个股中心(Hub) |
| `/` | `App.tsx` 内 `<Navigate replace>` | 重定向到 `/watchlist` |

### 2.3 个股中心(`/stock/:code`)信息架构

**核心原则:以 code 为主键,entity 为可选富化。** 深研只需 code(`POST /research-runs {code}`),交易/持仓只带 code;公司实体只是「相关事件/我的笔记/行业」三节的前置条件。因此 `entities.detail->>'code'` 查不到公司实体时,持仓/深研/涨停照常渲染,事件/笔记/行业如实空态 + 一行「未建实体,暂无事件/笔记关联」。**不做 GET 触发的懒创建**(`EnsureCompanyEntity` 是写操作,GET 触发会污染数据)。

| # | 区块 | 内容 | 数据来源 | 现状 |
|---|---|---|---|---|
| 0 | 页头 | 名称/代码/最新报告入口/深研按钮 | 聚合响应 `entity` + 报告列表 | 复用 `components/research/DeepResearchButton.tsx`(已「无报告→深研/有报告→查看」双态) |
| 1 | **我的持仓与交易** | 最新持仓快照行 + 该股全部成交(时间倒序,分页) | `LatestPositionByCode`(**新写**)、`ListTradesByCode`(**新写**,走 `idx_trades_code`) | 无持仓/无成交 → `EmptyState` + 「去交易页录入」;这是 hub 与「看盘工具」的分水岭,放第一屏 |
| 2 | **深研** | 该股历史报告表 + 触发按钮 | `ListResearchRuns(ctx,code,limit)`(`research_runs.go:132`,**现成**)+ `RunHistoryTable.tsx` | 零新代码 |
| 3 | **我的笔记** | 引用了该公司实体的笔记 | `ListNotesReferencingEntity`(**新写**,`ListNoteRefs` 反向) | 需公司实体存在 |
| 4 | **相关事件** | affects 到该公司实体的事件(去重,时间倒序) | `ListEventsAffectingEntities`(`relationships.go:99`,**现成**) | 精确到公司级;行业/概念事件走另一跳,不混入 |
| 5 | **涨停记录** | 该 code 在涨停池出现的交易日期 | `ListZTAppearances(ctx,code)`(`market_snapshots.go:61`,**现成**) | store 层已有,未在 `/api/v1` 暴露过 |
| 6 | **行业/概念归位** | 公司→行业关系 | `ListEntityRelationships`(`relationships.go:34`,**现成**) | 纯展示 |

**顺序理由**:**我持有的(1)→ 我判断的(2、3)→ 外部信息(4)→ 结构(6)→ 历史(5)**。与「我的」无关的市场数据(快讯/图谱)不进 hub,它们在「发现」组里靠 code 反查。

### 2.4 数据获取:新增聚合端点 `GET /api/v1/stock/:code`

新文件 `internal/web/api_stock.go`,注册一行(`server.go` Routes():`mux.HandleFunc("/api/v1/stock/", s.handleAPIStock)`)。**照抄 `handleAPIDashboard`(`api_v1.go:668`)的形状**:顺序调 store 方法 → 组装一个 wrapper struct → 一次 `s.writeJSON`;错误统一 `s.apiErr(w,"stock",err)`。响应:

```json
{
  "code": "600519", "symbol": "sh600519",
  "entity": { "id": "...", "name": "贵州茅台", "status": "watch", "description": "..." },
  "industry": { "id": "...", "name": "白酒" },
  "position": { "snapshot_date": "...", "qty": 100, "cost_price": 1500.0, "pl": 1200.0 },
  "trades": [ ... ], "research": [ ... ], "events": [ ... ],
  "notes": [ ... ], "limit_ups": ["2026-08-14", "..."], "relations": [ ... ]
}
```

- **code 归一**:入口即 `store.NormalizeCode(code)`(`research_runs.go:44`),`/stock/sh600519` 与 `/stock/600519` 等价;前端统一用 6 位。
- **为什么用聚合端点而非前端组合请求**:需 6 次调用,其中 `ListAllEntities`/`ListAllRelationships` 是全量拉取(`api_v1.go:176` 无 status/code 过滤),前端过滤 = 把全库搬进浏览器。聚合端点 = 1 次请求 + 6 个 store 调用,是 dashboard 已验证的模式。
- **为什么不做 6 个细粒度端点**:hub 一次性整屏渲染,无局部刷新需求(深研轮询走已有 `/research-runs/:runId`)。

**需新写的 Go 代码(恰好 4 个小 getter,无 schema 变更)**:

| 方法 | 文件 | 说明 |
|---|---|---|
| `GetCompanyEntityByCode(ctx, code) (*model.Entity, error)` | `internal/store/entities.go` | `SELECT entityCols FROM entities WHERE type='company' AND detail->>'code'=$1 ORDER BY created_at LIMIT 1`。当前只有 `EnsureCompanyEntity`(读+写,`:24` 的 SELECT 是模板但私有);同 code 多实体时取最早。返回 nil = 未建。 |
| `ListTradesByCode(ctx, code string, limit int) ([]model.Trade, error)` | `internal/store/trades.go` | `WHERE code=$1 ORDER BY trade_date DESC, created_at DESC`,命中 `idx_trades_code`(`migrations/0010_trades.sql:21`) |
| `LatestPositionByCode(ctx, code) (*model.Position, error)` | `internal/store/trades.go` | `WHERE code=$1 ORDER BY snapshot_date DESC, created_at DESC LIMIT 1`。**注意**:`LatestPositions` 是「全组合最近快照日」,语义不同,不可复用 |
| `ListNotesReferencingEntity(ctx, entityID) ([]NoteRefDetail, error)` | `internal/store/personal_notes.go` | `ListNoteRefs`(:144)的**反向**:`JOIN personal_notes p ON p.id=r.from_id WHERE r.from_type='personal_note' AND r.to_type='entity' AND r.to_id=$1 AND r.rel_type='references'` |

### 2.5 前端组件拆分(遵守 <150 行硬规则)

```
frontend/src/pages/stock/[code].tsx          ← 编排 + useData<StockHub>,< 60 行
frontend/src/components/stock/StockHeader.tsx      (名称/代码/深研)
frontend/src/components/stock/StockPosition.tsx    (持仓 + 成交)
frontend/src/components/stock/StockEvents.tsx      (相关事件)
frontend/src/components/stock/StockNotes.tsx       (我的笔记)
frontend/src/components/stock/StockResearch.tsx    (深研报告区,内含 RunHistoryTable)
frontend/src/components/stock/StockLimitUps.tsx    (涨停记录小卡)
```

复用现成:`ui/{Num,States,Pagination}`、`hooks/{useData,useUrlState,usePagedQuery}`、`research/{DeepResearchButton,RunHistoryTable}`、`lib/{api,format,types}`。**不新增 UI 库、不新增 CSS 变量**,全部用现有 `.panel/.panel-pad/.page-head/.psub/.section/.section-head/.two-col/.kpi-grid/.ent-grid/.table/.num/.st-*` 词表。

### 2.6 自选数据模型:复用 `entities.status`(零迁移)

`entities.status` 已定义为自由 TEXT,`model.go:127` 与 `types.ts:23` 已声明三态,`entities.tsx` 已渲染徽标。**复用,不建新表。**

| status | 含义 | 触发 |
|---|---|---|
| `watch` | 在自选 | 自动同步命中(issue #87)或截图镜像命中 |
| `active` | 库中有、不在自选 | entity-build 默认态 |
| `archived` | 曾在自选、已移出(**不删**,历史/深研/笔记保留) | 两路任一判定缺失 |

> **issue #87 起的补充**:自动同步(常驻 `watch-sync`)与截图镜像**写同一套状态机**,
> 二者判定结果一致时互不影响。**加入价/日**另存 `watchlist_entries`(迁移 `0020`),
> **不进 `entities.detail`**(原因见 §2.6.1 —— 同一地雷)。

**理由**:
1. **身份唯一**:自选必须是「那只股票」,而非「一份并行列表里的一行」。深研 join(`ListResearchRunsByEntity`,`research_runs.go:153`)已以 `entities.detail->>'code'` 为桥,事件/笔记关联全挂在 entity id 上。新表会立刻产生「自选表 code ↔ entities code」双主键对账问题,违背「单一 Source of Truth」的项目哲学。
2. **镜像语义天然映射**:同花顺快照「有→无」正好是 watch→archived,无需额外软删列。
3. 前端类型与徽标已就绪。

#### ⚠️ 2.6.1 前置必修地雷:`UpsertEntity` 会每天清掉自选

**已验证的缺陷链**:
- `cmd/entity-build/main.go:389` 构造 `model.Entity` 时**不赋 Status**(字段缺省 `""`)。
- `internal/store/entities.go:54` 的 `UpsertEntity` 做 `status := defaultStr(e.Status, "active")` → 空 Status 被当作 `"active"`。
- churn 判断 `existing.Status == status` → `"watch" != "active"` 为假 → 落入 UPDATE 分支,把 `status` 写回 `'active'`。

**后果**:用户今天标的星(或截图同步的 watch),**明天收盘管线下一次跑 `entity-build` 就被清成 active**。自选功能在管线日跑下不可用。

**修法**(Phase 2 一并做):
- `UpsertEntity` 语义改为「**空 Status = 保持既有**」:UPDATE 分支在 `e.Status == ""` 时**不下发 status 列**(SQL `status = COALESCE($5, status)` 或 Go 侧按 `e.Status == ""` 决定是否包含该列);INSERT 分支仍用 `'active'`。
- churn 判断相应改为「仅当显式传了 status 才比较 status」。
- **影响面收敛**:`UpsertEntity` 唯一调用方是 `cmd/entity-build/main.go`(已 grep 确认)。`EnsureCompanyEntity`(`entities.go:19`)只在 INSERT 分支写 `'active'`,命中已存在实体时直接 return id 不写 status,**无需改**。
- **硬验收**:dev 库手工把某公司实体置 `watch` → 跑 `./bin/entity-build` → 断言仍为 `watch`。

#### 2.6.2 API 写/读接口

- **`GET /api/v1/entities?status=watch`**:`handleAPIEntities`(`api_v1.go:176`)加一个 query 参数;store 加 `ListEntitiesByStatus(ctx, status, limit)`(`ListEntitiesByType` 同款)。复用为自选列表数据源,`CommandPalette` 的实体拉取不受影响。
- **`GET /api/v1/watchlist`**:自选实体 + 持仓标记 + 一行摘要(最近深研日期、是否持仓),**避免前端 N 次 hub 请求**。复用 `ListEntitiesByStatus('watch')` + `LatestPositions` + 按需 `ListZTAppearances`(自选量小)。
- **只允许 `type='company'` 的实体进自选**:行业/概念进自选无意义。

### 2.7 自选截图镜像同步(复用交易截图两段式)

现有两段式:`tradeImportAPI`(`api_write.go:283`)→ 视觉抽取 → `buildImportPreview`(`trades.go:108`)→ 预览 JSON(**不落库**)→ `tradeConfirmAPI`(`:368`)→ 落库。自选只**替换两处 + 加 confirm 分支,不新增端点**。

**(1) `importPrompt(kind)`(`trades.go:84`)新增第三分支**:

```
system: 你是 PIKS 的自选股截图识别助手。识别同花顺 App「自选股列表」截图,抽取股票列表。
规则:
- 只抽取截图中明确出现的条目;字段缺失标 null,禁止推断或补全;
- code 必须为 6 位代码;只有名称没有代码的行跳过;
- 忽略指数/基金/板块行(若截图含此类行,不输出);
- 若图片不是自选股列表截图,返回空数组 {"watchlist":[]},不要编造;
- 仅输出 JSON。
schema: {"type":"object","properties":{"watchlist":{"type":"array","items":
         {"type":"object","properties":{"code":{"type":"string"},"name":{"type":"string"}}}}}}
```

**(2) `buildImportPreview`(`trades.go:108`)加 `kind == "watchlist"` 分支 —— 服务端 diff**

与 trade/position 的关键区别:预览不是「识别的行」,而是「识别的行 × 现有 `status='watch'` 的公司实体」的差集。新增 store getter `ListWatchEntities(ctx)`(`SELECT ... WHERE type='company' AND status='watch' ORDER BY name`)。

预览数据结构:

```go
type PreviewWatch struct {
    Include bool   // 默认 true
    Code    string
    Name    string
    Action  string // "add" | "keep" | "remove"
}
type ImportPreview struct {
    Kind         string
    AttachmentID string
    Trades       []PreviewTrade
    Positions    []PreviewPosition
    Watchlist    []PreviewWatch   // 新增
}
```

- `add`:截图有、PIKS 无(或 status != watch)→ 置 `watch`
- `keep`:两边都有 → 无操作(预览标「已在」)
- `remove`:PIKS `watch` 有、截图无 → 置 `archived`(不删,历史全保留)

**(3) `apiImportPreview`(`api_write.go:258`)** 加 `Watchlist []apiPreviewWatch`;`toAPIImportPreview`(`:265`)同步映射。`tradeImportAPI` 里「未识别到…」的 422 判断(`:358`)扩成三类任一非空即通过,文案改「未识别到交易/持仓/自选股」。

**(4) `tradeConfirmAPI`(`:368`)加 `case p.Kind == "watchlist"` 分支 —— 幂等应用**:
- `add`/`keep` 行(`include=true`):`EnsureCompanyEntity(ctx, code, name)` 拿/建 entity id → `SetEntityStatus(id, "watch")`
- `remove` 行(`include=true`):按 code 查 `GetCompanyEntityByCode` → `SetEntityStatus(id, "archived")`
- 全空 → 400「没有勾选任何自选股行」
- 返回 `{ok:true, watch:N, archived:M}`(比既有 `{ok:true}` 多带计数)

**关键设计决定:confirm 的请求体就是「用户编辑过的 diff」**,不在 confirm 时重算。理由:用户可能在预览表取消某行(不想移出某只票),重算会覆盖其编辑;且 confirm 不读截图、不读 AI,天然幂等(重复提交只是把同批 status 再写一遍)。

**(5) 新增 store 方法**:`SetEntityStatus(ctx, id, status) error`(`UPDATE entities SET status=$2, updated_at=now() WHERE id=$1`)、`ListWatchEntities(ctx)`。`EnsureCompanyEntity` 的 `detail.source` 参数化,自选来源记 `watchlist-import`(留血缘)。

### 2.8 前端:入口合并 + 组件拆分

- `ImportFlow.tsx` 的 kind chip 从两项变三项:`今日交易 / 持仓 / 自选股`。保持显式选择(现有链路用 `r.FormValue("type")`;vision 对三类截图结构差异大,明确传 type 比让模型猜更稳,识别失败的代价是整份截图识别为空)。
- `ImportFlow.tsx` 现 **251 行已超 150 行硬规则**,本次顺手拆:抽出 `components/trades/ImportPreviewTables.tsx`(trade/position/watch 三张表各自子组件),`ImportFlow.tsx` 只留上传/确认编排。
- 自选预览按 `Action` 分两组渲染:**「将加入自选」**(add + keep,keep 带灰色「已在」徽标)与**「将移出自选」**(remove,默认勾选,整组可一键取消),顶部提示「历史/深研/笔记保留」。
- 文案:panel 副标题改「同花顺今日交易 / 持仓 / 自选股截图,AI 视觉识别后预览确认」。

### 2.9 前端类型与常量

- `lib/types.ts`:加 `PreviewWatch`、`ImportPreview.watchlist`、`StockHub`(个股中心聚合响应)、`WatchlistRow`。
- `lib/api.ts` ENDPOINTS 加 `stock: "/stock/:code"`、`watchlist: "/watchlist"`;`entities` 注释补 `?status=`。
- `lib/constants.ts`:加 `IMPORT_KINDS`(避免三处硬编码 label)。

---

## 3. 边界与诚实降级

| 情况 | 行为 |
|---|---|
| 截图含指数/基金/板块行 | prompt 要求忽略;若仍输出,`buildImportPreview` 按 `^\d{6}$` 过滤,**不**直接建实体 |
| 截图只覆盖部分自选(分组/分页) | 镜像语义最大操作风险 → 移出默认勾选 + 整组一键取消 + 顶部明示「本次将移出 M 只」 |
| 重复上传同一张 | diff 全 `keep` → confirm 是幂等重放,零新增行 |
| 视觉模型未配置/预算耗尽 | 完全沿用现有护栏(`api_write.go:318-329`):400/429 如实报错,不降级 |
| 某 code 在 PIKS 无实体 | `add` 分支 `EnsureCompanyEntity` 补建(与交易导入同款,`source=watchlist-import`) |
| `archived` 实体可见性 | `ListAllEntities` 不过滤 status,移出自选 ≠ 从库中消失;实体库加「已移出自选」灰色徽标(与 `watch` 徽标对称) |
| 长截图/多屏 | Phase 3 先只支持单图,**UI 明示「请一屏内截全」**;多图作为后续(第二次上传时缺失行会误判 remove) |

---

## 4. 分阶段实施(后续轮次,每阶段独立可验收)

### Phase 1 — 个股中心(只读、零写入、可回滚)★ 先做

**Scope**:任何地方看到一个代码 → 一页看完这只票。

- Go(全新增,不改既有行为):`internal/web/api_stock.go`(`handleAPIStock`,聚合 6 节)+ `server.go` 注册一行 + 4 个 store getter(`GetCompanyEntityByCode`/`ListTradesByCode`/`LatestPositionByCode`/`ListNotesReferencingEntity`)。
- 前端:`App.tsx` 加 `/stock/:code`;`pages/stock/[code].tsx` + `components/stock/*.tsx`(6 区块);`lib/{types,api}.ts` 加 `StockHub`/`ENDPOINTS.stock`;**导流改造**——`pages/ladder.tsx`、`components/trades/TradeTable.tsx`、`PositionTable.tsx`、`components/research/ReportHeader.tsx`、`pages/events.tsx` 的 affected、`pages/entities.tsx` 有 code 的卡片 → 指向 `/stock/:code`。
- **验收**:`go build ./... && go vet ./... && go test ./...` 过;`tsc --noEmit && npm run build` 过;`curl /api/v1/stock/600519` 形状对齐 `StockHub`;从涨停梯队点代码 → hub 六区块渲染;无持仓/无笔记的股票如实空态;深研按钮从 hub 触发 → 跳 `/research/:runId`。**无写入、无 schema、可随时回滚(删路由 + 删导航一行)。**

### Phase 2 — 自选(模型 + 首页 + 市场概览条)

**Scope**:把 `watch` 真正用起来,首页换成自选。**依赖 Phase 1 的 `/stock/:code`。**

- Go:🔴 `UpsertEntity` 空 Status 保持既有(§2.6.1)+ `ListEntitiesByStatus` + `ListWatchEntities`;`handleAPIEntities` 支持 `?status=`;新增 `GET /api/v1/watchlist`。
- 前端:`navItems.ts` 最终版(PRIMARY=`自选 /watchlist`,`/market`)+ `App.tsx` `/`→`/watchlist` 重定向 + `/market`;`pages/watchlist.tsx`(自选列表,分「持仓中/仅自选」,顶部市场概览条复用 `GET /market/snapshot` 的 `Gauge`/`MarketPanel` widgets);`CommandPalette` 有 code 的实体项跳 `/stock/:code`。
- **硬验收**:置某实体 `watch` → 刷新仍在 → **dev 跑 `./bin/entity-build` → 断言仍为 `watch`**;`/watchlist` 空时显示「去截图导入自选」引导;`/` 落到自选;旧看板在 `/market` 完整可用;URL query 承载筛选(规范 7)。

### Phase 3 — 自选截图镜像同步

**Scope**:一处上传口,三类截图,自选自动镜像。**依赖 Phase 2。**

- Go:`importPrompt` 第三分支;`buildImportPreview` watchlist 分支;`ImportPreview.Watchlist`/`PreviewWatch`;`apiImportPreview` 映射;`tradeImportAPI` 422 判断扩展;`tradeConfirmAPI` watchlist 分支;`SetEntityStatus`、`ListWatchEntities`、`EnsureCompanyEntity` source 参数化。
- 前端:`ImportFlow.tsx` 拆分(150 行规则)+ 第三类 chip;`ImportPreviewTables.tsx`(含 `WatchPreviewTable`);`lib/{types,constants,api}.ts` 扩展。
- **验收**:上传自选截图 → 预览分「将加入 N/将移出 M」;取消勾选某移除行 → confirm 后该股仍为 watch;重复上传同一张 → 全 keep、零变化;上传非自选截图 → 422 如实报错;模型未配/超预算 → 400/429 不变。**「移出」可撤销性**:confirm 后实体变 `archived`(非删除),实体库可见、可星标回来。

### Phase 4 — 收口与一致性

- `navItems.ts` 收尾、`App.tsx` 路由注释更新(顶部注释仍在讲旧故事)。
- 事件流 affected、图谱点选面板、笔记详情 refs、周报里的 code → 全部指向 `/stock/:code`。
- `CLAUDE.md`「页面模块」清单重写为个股轴心描述;`docs/进度总表.md` 登记 phase5;`docs/phase5/design/frontend-ia.md` 冻结。
- **已知债登记(本轮不改)**:CLAUDE.md 圆角 6px vs `globals.css` 实际 16px;`ForceGraph(281)/ImportFlow(251)/NoteForm(200)/weekly Sections(175)/dashboard widgets(173)` 超 150 行。

---

## 5. 待确认的开放问题

1. **`UpsertEntity` 状态保持改动**(§2.6.1)——必须批准,否则自选每天被管线清一次。这是全部 4 个 Phase 里唯一触碰非前端既有逻辑的改动。
2. **`entities.detail->>'code'` 无函数索引**。个股中心每次查询全表扫 `type='company'`(当前 52 家量级,无感)。CLAUDE.md 明令「禁止直接改 PG schema(前端重构不涉及)」。**建议**:本轮不加索引,若自选量级或 hub 延迟出问题,作为独立迁移 `0013` 单独批准(partial index:`ON entities ((detail->>'code')) WHERE type='company'`)。
3. **同 code 多实体**(历史脏数据:`EnsureCompanyEntity` 按 name 或 code 两路查,可能同名不同码产生两行)。`GetCompanyEntityByCode` 取最早一行。是否需一次性清理脚本?建议登记为独立任务,不在本轮。
4. **镜像语义的「移出」默认勾选**——已定:**(b)** 移出默认勾选 + 整组一键取消 + 顶部显示「本次将移出 M 只,历史/深研/笔记保留」。
5. **首页市场概览条**——已定:要,复用 `GET /market/snapshot`。
6. **长截图/多屏**——已定:Phase 3 先只支持单图 + UI 明示「一屏内截全」;多图后续。
7. **`archived` 的可见性**——移出自选 ≠ 从库消失;建议实体库加「已移出自选」灰色徽标。
8. **`/events` 是否加「只看我的自选相关事件」筛选**——需新聚合查询,建议 Phase 4 候选,先不做。

---

## 6. 验收方式

- **文档层(本轮)**:本设计经用户过目 → 定稿门确认 → 落 `docs/phase5/design/frontend-ia.md` + 登记 `docs/进度总表.md`。
- **实现层(后续)**:
  - Go:`go build ./... && go vet ./... && go test ./...`;`curl /api/v1/stock/600519` 各字段非空。
  - 前端:`npm run lint && npm run build`;`npm run dev` 浏览器验证导航与个股中心三态。

---

## 7. 实现记录(2026-09-13,dev → 上生产)

四阶段全部落地于 dev 分支;遵循「API key 不进 git」「禁改 PG schema」「禁破坏性 API 变更」红线。

**部署记录(2026-09-13)**:dev `1cde70e`(含 b5baca4/efcfa9c research 两修)→ merge dev→master `82f3176` → 空提交 marker `c81c6d2`;`./scripts/deploy.sh` 上线。migrate 0。验收:15 页全 200(含新 `/market`、`/stock/600519`、`/stock/300750` 深链);`/api/v1/watchlist`(`{items:[]}`)、`/api/v1/stock/:code`、`/api/v1/entities?status=watch` 全 200;`/api/v1/stock/600371`(万向德农)entity+industry(种植业)+limit_ups 6 渲染正常(实体分支);生产 bundle `index-CtxiwP8_.js` 含新 IA 文案。回滚镜像 `piks-tools:rollback-pre-ia`(旧 `511f951`)留存 lab。

**遗留(部署后)**:生产 `ai_model_vision` 为空 → **自选/交易截图导入暂不可用**(需先补视觉模型,`app_config` 改后须 `docker compose restart web` 生效)。

### 7.1 后端

| 文件 | 改动 |
|---|---|
| `internal/store/entities.go` | 新增 `GetCompanyEntityByCode`、`ListEntitiesByStatus`、`SetEntityStatus`;修 `UpsertEntity` 空 status=保持既有(§2.6.1 地雷) |
| `internal/store/trades.go` | 新增 `ListTradesByCode`、`LatestPositionByCode` |
| `internal/store/personal_notes.go` | 新增 `ListNotesReferencingEntity`(`ListNoteRefs` 反向) |
| `internal/store/events.go` | 新增 `ListEventsByIDs` |
| `internal/web/api_stock.go`(新) | `GET /api/v1/stock/:code` 聚合端点 + DTO(股票代码规范化 `NormalizeCode`) |
| `internal/web/api_watchlist.go`(新) | `GET /api/v1/watchlist` 聚合端点(自选 + 持仓标记) |
| `internal/web/api_v1.go` | `handleAPIEntities` 开 `?status=` 参数;抽 `toAPITrade`/`toAPIPosition`;`apiAffected` 加 `code`;`nameRef` 加 code |
| `internal/web/server.go` | 注册 `/api/v1/stock/`、`/api/v1/watchlist` |
| `internal/web/trades.go` | `importPrompt` 加 `watchlist` 分支;`buildWatchPreview` 服务端镜像 diff(add/keep/remove) |
| `internal/web/api_write.go` | 导入预览 DTO 加 `watch`;`tradeConfirmAPI` 加 `watchlist` 幂等分支(`tradeConfirmWatchlist`) |

测试:`internal/store/entities_watch_test.go`(地雷回归:watch 经空-status upsert 仍 watch)、`internal/web/watchlist_preview_test.go`(镜像 diff 语义 + DTO 契约)。均集成测试,`PIKS_TEST_INTEGRATION=1` 开。

### 7.2 前端

- 路由:`/`(自选首页)→ 原看板迁 `/market`;新增 `/stock/:code`;`/recon` 保留(设置页子入口)。
- 导航:`navItems.ts` 重排为 自选 / 研究 / 发现 / 复盘 / 交易 / 系统(对账降为设置页)。
- 新增页/组件:`pages/watchlist.tsx`、`pages/stock/[code].tsx`、`components/stock/*`、`components/watch/{WatchOverview,WatchTable}.tsx`。
- 导流改造:涨停梯队、交易表、持仓表、实体库卡、事件详情 affected、报告头 code、⌘K 面板 → `/stock/:code`。
- ImportFlow 拆分(原 251→149 行):`ImportControls`、`TradePreviewTable`、`PositionPreviewTable`、`WatchPreviewTable`(移除组默认勾选 + 整组一键取消)。

### 7.3 验收结果

- `go build ./... && go vet ./... && go test ./...` 全过;`npm run lint && npm run build` 全过。
- curl:`/api/v1/stock/603392` 形状对齐 StockHub;`sh603392` 规范化正确;`300750` 如实降级(entity=null 但 trades/position 有值)。
- **Phase 2 硬验收**:置某实体 `watch` → 跑真实 `./cmd/entity-build` → 断言仍为 `watch` ✓(修复前会被清成 active)。
- Phase 3:confirm `add` → status=watch;`remove` → status=archived 且实体保留;非法 kind → 400。
- 测试后 DB 已还原(无残留 watch/archived)。

### 7.4 与设计的偏差

1. **§2.3 自选数据模型表**:设计写「`archived` = 截图镜像缺失」;实现中 `archived` 由 confirm 显式写入(用户可整组取消移出),语义一致。
2. **首页市场概览条**:设计提「复用 `GET /market/snapshot`」;实现复用 `GET /dashboard` 的 `market` 字段(单请求,且与看板同源),避免二次请求。
3. **未知路径** `*` → 重定向 `/`(自选),设计未明说,合理默认。

### 7.5 已知债(未改)

- `ForceGraph(281)`、`NoteForm(200)`、weekly Sections(175)、dashboard widgets(173)超 150 行。
- CLAUDE.md 圆角 6px vs `globals.css` 实际 16px 不一致。
- `entities.detail->>'code'` 无函数索引(暂不加,量级小)。
- 同 code 多实体历史脏数据未清理。
- `archived` 在实体库暂无「已移出自选」灰标(§5.7 候选)。
  - 端到端:`frontend/scripts/e2e_check.mjs`(如可用)+ 各页面 200。
  - **Phase 2 硬验收**:置 `watch` → 跑 `entity-build` → 断言仍 `watch`。
