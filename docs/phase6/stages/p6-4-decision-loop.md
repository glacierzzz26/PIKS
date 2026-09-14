# P6-4 决策记录闭环(研究→决策→持仓)—— 实现与验收

> 阶段:前端决绝重构 P6 第 4 阶段。设计依据 `docs/phase6/design/ux-ia.md` §3.1。
> 目标:回答史诗闭环 #1「我为什么买它」—— 交易可回溯到买入当时在看的研究 / 消息 / 笔记。**零 schema。**

## 交付

### 1. 后端:决策边(复用多态 `relationships`,零新表)
- 边形:`(from_type='trade', from_id=<trades.id UUID>, to_type='research_run'|'event'|'personal_note', to_id=<UUID>, rel_type='based_on')`。
  **用 `research_runs.id`(UUID),绝不用 `run_id`(TEXT)** —— `relationships.to_id` 无 FK,接错会静默悬空。
- `POST /api/v1/trades` 收 `based_on:{run_ids[],event_ids[],note_ids[]}` → 逐条 `CreateRelationship`;
  返回 `{ok, linked, trade_id}`。建边失败不回滚交易,但**如实报错**(「交易已入库,但决策关联失败」),不假装全成功。
- `store` 新增:
  - `InsertTradeReturningID`(单条入库返回 id,建边需 from_id);`InsertTrades` 抽出 `insertTradeSQL` 共用。
  - `ListRelationshipsFrom` / `ListRelationshipsFromIDs`(出边;后者批量喂个股中心,免 N 次往返)。
  - `ListResearchRunsByIDs`(按 id 取 **status='done'** 报告)、`ListPersonalNotesByIDs`。
- 读回:`apiTrade` 加 `based_on[]`;`/trades` 列表批量解析;`/stock/:code` 加顶层 `decisions: {trade_id:[...]}`。
  悬空边(目标已删/非 done)**诚实跳过**,不渲染无名空节点。
- `apiResearchRunSummary` 加 `id`(UUID,决策关联用)—— 前端此前只有 `run_id`(业务键),接边会悬空。

### 2. 图谱隔离(⚠️ 同 commit,风险所在)
- 决策边 `from_type='trade'` 会经 `ListAllRelationships`(无过滤)漏进 `/graph` 成**无名节点**。
  加 `graphRelFilter = rel_type NOT IN ('decided_by','based_on')` 到 `ListAllRelationships`。
- `ListGraphEdges` 只查 `affects`,`/graph` 投影本身无 trade 节点 → 已天然隔离,无需改。

### 3. 顺修:CreateRelationship 的 properties NOT NULL 地雷(首跑即炸)
- `relationships.properties` 是 **NOT NULL**,而 `CreateRelationship` 透传 nil `Properties` → NULL → 23502。
- 修:调用方未设时默认落 `'{}'`(与 `ReplaceNoteRefs` 手写 INSERT 的省略列默认行为一致)。
- 回归锁:`internal/store/relationships_integration_test.go`(默认 skip,`PIKS_TEST_INTEGRATION=1` 时跑;测后清边)。

### 4. 前端
- 新组件:
  | 组件 | 作用 |
  |---|---|
  | `trades/DecisionRefs.tsx` | 决策引用 chip(研报/消息/笔记 → 深链)+ 列表,空时如实「未记录当时依据」 |
  | `trades/TradeBasedOnPicker.tsx` | 「当时在看什么(可选)」选择器,数据源 = 该 code 的 `/stock/:code`;代码非 6 位/无关联 → 折叠如实说明 |
  | `stock/StockDecisions.tsx` | 个股中心首屏「当时在看什么」区块,逐笔列买入决策 + 关联 |
- 复用既有 `note/RefPicker`(三组:研报/消息/笔记)。
- `TradeAddForm` 内嵌选择器并 POST `based_on`;`TradeTable` 展开行显示「当时在看什么」;
  `/stock/:code` 新增区块(置于「我的持仓与交易」之前,首屏回答「我为什么买它」)。

## 验收

| 项 | 结果 |
|---|---|
| `go build ./...` / `go vet ./...` / `go test ./...` | ✅ 全过 |
| `tsc --noEmit`（`npm run lint`） | ✅ |
| `npm run build` | ✅（JS 964.53 kB / gzip 309.95）|
| `POST /trades {based_on:{event_ids,note_ids}}` → `GET /stock/:code` 返回该边 | ✅（`linked:2`;`decisions[trade_id]` 含 event+note,标题/日期/深链正确）|
| `POST /trades {based_on:{run_ids:[UUID]}}` → 边 to_id 落 `research_runs.id` | ✅（临时插 done run 实测;URL 用业务键 `run_id`;测后删除）|
| **`/graph` 节点/边数前后不变(回归断言)** | ✅ **9 节点 / 4 边不变**;`/relationships` **64 不变**(DB 内 3 条决策边存在但被过滤)|
| 无关联交易如实空态 | ✅（`based_on:[]`;个股页显示「还没有记录过买入依据」+ 引导）|
| P6-4 首跑暴露 `properties` NOT NULL 缺陷 | ✅（已修 + 集成回归测试锁定）|
| `DecisionRefChip` prop 名 `ref`(React 保留)致报错 | ✅（e2e 浏览器检查捕获 → 改 `item`;console 0 错误）|
| e2e 冒烟 28 通过 / 0 失败 | ✅（本地 PG:5433 + Go:8090 + vite:3100;改动后复跑确认）|
| 拉取式 UI 实测(Playwright) | ✅ 选择器三组渲染 + 事件选项可勾选(已选 1);个股页 chip 渲染;空态文案正确;0 console 错误 |

## 偏差与遗留
- **`based_on` 仅手录入径**:截图导入不采集依据(截图无决策语义);后续若要从导入补关联,需另做 UI。
- **交易行不可事后编辑关联**:当前只能在录入时关联;补关联/改关联留作后续(需 `PUT /trades/:id/based_on`)。
- **决断边一律 `rel_type='based_on'`**:设计提到 `decided_by` 备选,实现统一用 `based_on`(语义足够,少一处分叉);
  `graphRelFilter` 同时排除两者以防未来启用 `decided_by` 再漏。
- 生产 `research_runs` 可能有数据,但本地为空 → run 路径以临时插入实测覆盖(已删)。
