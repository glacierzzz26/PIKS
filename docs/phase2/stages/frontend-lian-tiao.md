# 产品化 1:前端 React 联调真实后端数据(2026-08-29)

## 背景

`frontend/`(Next.js 14 + React 18 + TS + Tailwind 设计令牌,参照「符合 frontend」自身设计复刻)此前全部回落到内置演示数据——Go 后端没有 `/api/v1/*` 列表 REST。本次目标:把 React 前端接到真实后端数据,`mock` 仅作降级兜底;**零破坏性 API 变更**(现有 HTML 页面路由不动)。

## 后端新增(只读投影,全部复用 store)

新文件 `internal/web/api_v1.go` + 挂载 `internal/web/server.go`:

| 端点 | 数据源 |
|---|---|
| `GET /api/v1/events?type&status&q` | `ListEventsForAPI`(join sources/raw_documents,取来源名与 URL) |
| `GET /api/v1/entities?type&q` | `ListAllEntities` / `ListEntitiesByType` |
| `GET /api/v1/relationships` | `ListAllRelationships`(全量) |
| `GET /api/v1/market/snapshot?date` | `GetMarketSnapshotByDate`(无 date 取最新) |
| `GET /api/v1/flashes?q&source` | `ListRawDocumentsWithSource`(join 事件链,important≈有 event_id) |
| `GET /api/v1/notes` / `/api/v1/notes/:id` | `ListPersonalNotes` / `GetPersonalNote`(id 非 UUID 或笔记不存在 → 回落 `weekly_summaries`) |
| `GET /api/v1/dashboard` | `Counts`+`NoteTradeCounts`+`ListMarketSnapshots(6)`+事件 Top6+`ListTaskRuns(6)`,复盘 Markdown 由最新快照真实数字模板化合成 |
| `GET /api/v1/recon` | `ListReconDaily`(CTE:raw/events/snapshots 按日归并) |
| `GET /api/v1/reviews` | `ListPositionReviews`(risks→state 启发:positive/negative/neutral) |
| `GET /api/v1/trades` | `ListTrades`+`LatestPositions`(pnl_pct=(last-cost)/cost×100,PL 是绝对盈利非百分比) |
| `GET /api/v1/chat` | `ListChatMessages`(引用展平为「事件:…」/「实体:…」字符串) |
| `GET /api/v1/settings` | `ListAppConfig` 分组(extract/reasoning/vision)+ `maskSecret` 掩码密钥 |
| `GET /api/v1/weekly` | `ListWeeklySummaries` |

映射要点:**affected 由 `[]string` 经 name-index(name+aliases)解析为 `{word,entity_id,entity_name}`**;事件状态 extracted→pending / verified·published→confirmed;zt_pool 缺失字段如实补 0/空;index_json/成交额本地 NULL 如实空数组/0。**新增 CORS 中间件**(`cors()`,scope `/api/v1/`):浏览器跨域(:3100→:8090)可读。

## 前端接线

- `frontend/src/lib/api.ts` 增补 7 端点;`useData<P>` 三态 hook 维持不变(mock 降级兜底)。
- **14 页全部接 `useData`**:看板 / 事件 / 实体 / 图谱 / 涨停梯队 / 快讯 / 笔记 / 笔记详情 / 对账 / 复盘 / 交易 / 对话 / 设置 / 周报。
- **「演示数据」徽章改条件显示**:`lib/demoSignal.ts` 全局计数,任一 useData 降级才显示;接真实数据后隐藏(诚实标注)。

## 验证

- ✅ 前端 `npm run build`:16 路由全绿,TS 类型检查通过。
- ✅ 后端 13 端点 curl 全 200;抽查 events(affected 解析)/trades(持仓 pnl%)/notes/reviews/chat 形状对齐 `frontend/src/lib/types.ts`。
- ✅ `go build ./...`、`go vet ./internal/web/` 通过。
- ⏳ 浏览器端到端:`frontend/scripts/e2e_check.mjs`(逐页断言真实数据、无「演示数据」徽章、无 console 错误)。需 lab/dev 机器装 chromium 系统依赖(`sudo npx playwright install-deps chromium`)。

## 部署说明

- Go 后端 `/api/v1` 随镜像重建生效(`./scripts/deploy.sh`),**无迁移**(只读查询)。
- React 前端为本仓库**首次提交**;生产仍由 Go web(:8090)服务,前端是否上生产另行决策。
