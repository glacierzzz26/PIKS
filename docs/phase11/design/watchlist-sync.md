# 同花顺自选自动同步(服务器侧拉取,替代截图)

> 阶段:数据源扩展 / 「减轻手工录入」议题(issue **#87**)。
> 姊妹实现:[hot-topic.md](./hot-topic.md)(常驻服务骨架)、
> [flash-cadence.md](./flash-cadence.md)(常驻 + 反封禁护栏)。
>
> **本文范围**:同花顺「我的自选」的服务器侧自动同步 —— 协议、新表、常驻服务、部署接线、凭据门控。
> **本文不覆盖**:持仓 / 成交的自动化(仍走截图;见 §5 边界)。

---

## 0. 一句话结论

把「自选靠手机截图导入」改为**服务器每日 3 次自动拉取**同花顺「我的自选」:

- **成员资格真源不变** —— 仍是 `entities.status='watch'`,**不迁表**(前端/接口/实体页全以它为真源)。
- **新增表 `watchlist_entries`**(迁移 `0020`)**只补「加入价 + 加入日」** —— 因
  `entities.detail` 每轮被 entity-build **无条件覆盖**(§3.1,本表存在的唯一理由)。
- **新增常驻服务 `watch-sync`**(复用 `piks-tools` 镜像,仍四镜像),北京时间 **09:00 / 12:55 / 18:00**
  各同步一次;重启去重靠 DB(`task_runs.meta.slot`)。
- **只读镜像**:绝不向同花顺写(add/del),只拉「我的自选」这一个分组。

---

## 1. 源选型与协议(探针实测,勿凭想象改)

### 1.1 两个无签名 GET

| # | 端点 | 作用 | 返回 |
|---|---|---|---|
| 1 | `GET https://t.10jqka.com.cn/newcircle/group/getSelfStockWithMarket/` | 名单 | `{errorCode:0, result:[{code:"601091",marketid:"17"}]}` |
| 2 | `GET https://ugc.10jqka.com.cn/selfstock_detail?reqtype=download&app_flag=0E&userid=<uid>` | 加入价/日 | XML `<ret code="0"><item version=.. selfstock_detail="<base64 JSON>">`,JSON = `[{C,M,P,T}]` |

- 请求头用移动端 UA(`Hexin_Gphone/11.28.03 (Royal Flush) … okhttp/3.14.9`),见 `internal/ths/client.go:19`。
- 端点 2 需 `userid` 头(同值)。
- 均**无签名**、无验证码、无设备指纹。

### 1.2 实测名单构成(2026-09-22,43 项)

marketid 分布:`17`×15(沪)、`33`×23(深)、`65`×1(期货 `RU9999`)、
`UCXF`/`218`/`97`/`219` 各 1(`SI0W`/`AUUSDO`/`USDIND`/`BRN0W`)。
→ **38 只 A 股 + 5 只非股票**。

**marketid A 股白名单**(`internal/ths/market.go:33`,单一真源):

```
17 = 沪(SH) · 33 = 深(SZ) · 18 = 科创(KC) · 38 = 创业(CYB) · 71/151 = 北交(BJ)
```

其余(48/88 指数、55 港、61 美、65 期货、20/36 ETF…)一律**丢弃**(记 `meta.dropped[]` 带原因)。

> 🔴 **只同步「我的自选」这一个虚拟分组**。同花顺别的手工分组(如「日韩指数」含 N225/KS11)
> **必须过滤** —— 那正是把指数灌进 `entities` 的入口。

### 1.3 🔴 code → name 缺口(本协议不给股票名)

两个端点**都不返回股票名**(list item keys = `{code, marketid}`;detail keys = `{M,C,P,T}`)。
而 `EnsureCompanyEntity(code, name)` 需要 name,且 `entities` 有 `UNIQUE(type, name)`。

**三级回退链(宁缺毋假,`internal/watchsync/apply.go` 的 `resolveNames`)**:

1. **本地**(零外呼):`store.CompanyNamesByCodes` —— entity-build 每日从涨停池建的 company 已覆盖大量票;
2. **同花顺 realhead**(同一上游、无签名、不引第二外部源):
   `GET https://d.10jqka.com.cn/v6/realhead/hs_<code>/last.js` → `items.name`(需 `Referer: https://stockpage.10jqka.com.cn/`);
3. **仍缺 → deferred**:本轮**不建实体**(绝不拿 code 当 name 建行,会污染 `(type,name)` 唯一键与 ⌘K),
   记 `meta.deferred_codes[]`,**下轮自然重试**。成员资格晚 3 小时可见 ≫ 建一个名叫 `600519` 的实体。

> 未采纳方案:东财 `ulist` 批量取名。理由:`realhead` 与自选**同源**(一个上游出问题只需修一处)、
> 零新依赖、沪深实测均可用;东财留作后续可选批量优化。

### 1.4 登录(账号密码自动登录)

`auth.10jqka.com.cn/verify2` 四步(`do_rsa` 取公钥 → `unified_login` RSA PKCS#1v1.5 加密账密 →
`mainverify` 取 `signvalid`)→ `upass.10jqka.com.cn/docookie2.php` 换 cookie。
**无滑块、无设备指纹**,Go 标准库(`encoding/xml` + `crypto/rsa` + `net/http/cookiejar`)即可。
cookie 里 `sess_tk` 是 JWT,`exp` ≈ **7 天**。

> ⚠️ **登录路径尚未端到端实测**(见 §7 偏差登记)。**实测可用的是 cookie 注入**;
> 故实现上**注入 cookie 优先于账密**(`internal/ths/client.go` 的 `CookieSource()` = `inject` / `login`)。
> 两者可并存:注入的 cookie 过期后仍可用账密自动续。

---

## 2. 架构决策(爆炸半径最小)

**自选「成员资格」权威源仍是 `entities.status='watch'`,不迁到新表。**
`/api/v1/watchlist`、`/stock/:code` 的 decisions、`/entities?status=watch`、术语表全部以它为真源;
迁表 = 前端 + 接口 + 实体页**三处**改动。新表 `watchlist_entries` **只**补「按 code 的加入价/加入日」。

自动同步与截图路径**写的是同一套 entities 状态机**(`EnsureCompanyEntity` + `SetEntityStatus`),
两路并存、互不冲突 —— 截图导入仍是兜底通路(§5)。

**整轮 apply = 一次 pgx 事务**(`store.Begin`)。取名走网络,**刻意在事务外**;
事务内只做 [建实体 → 置 watch/archived → 落价/日]。任一步失败整轮回滚,幂等可重来(下轮重跑收敛)。

---

## 3. 红线

### 3.1 🔴 加入价/日**不得**进 `entities.detail`(新表存在的唯一理由)

`entities.detail` 每轮被 entity-build 的 `UpsertEntity` **无条件覆盖**
(`internal/store/entities.go` 的 `UPDATE … detail=$4`)。写进 detail 的加入价/日**当天就被抹掉**。
→ 新表 `watchlist_entries`。**任何把加入价挪回 `entities.detail` 的改动都是错的。**

集成测试 `TestWatchlistEntrySurvivesEntityBuild` 钉死此行为:写 entry → 调 `s.UpsertEntity`
模拟 entity-build → 断言价/日**逐字不变**,且**断言 `entities.detail` 确实被覆盖了**
(若哪天 entity-build 不再覆盖 detail,该断言会失败,提醒复核本红线是否还成立)。

### 3.2 🔴 移出不删行

`watchlist_entries.removed_at` 置位,与 `entities.status='archived'` **同构**(迁移 0004)。
re-add = 同一行 UPSERT,清 `removed_at`,并用**上游新值**覆盖价/日(同花顺会重新给);
在选期间重复同步(keep)则保留原值(`COALESCE`),避免上游 detail 抖动改写历史。

### 3.3 🔴 失败不静默(#64 教训)

| 情形 | 处置 |
|---|---|
| 上游名单为空 | **failed**,提示「cookie 可能已失效」;**为防误清空,本轮不改任何状态** |
| 上游有数据但过滤后 0 条 | **failed**(A 股白名单/代码格式可能已失效) |
| `selfstock_detail` 单独失败 | **降级**:名单照常同步,`meta.detail_failed=true` **必须可见**(不是静默 success) |
| 无凭据 | **failed** + 指路 `/settings`(配置缺失是运维事件,不该像「非交易时段」那样沉默) |
| 登录失败 | **failed** + `meta.login_failed=true`;退避 5m→15m→30m→1h;**硬上限 6 次/日** |

### 3.4 🔴 只读

**绝不向同花顺写**(不 add / del 自选)。PIKS 侧也不提供手动增删自选的 API —— 仍是同花顺**单向镜像**。

### 3.5 ⚠️ NULL ≠ 0

`added_price = NULL` 表示「上游未给 / ≤0 / 不可解析」,**绝不填 0**(0 不是有效 A 股价格);
`added_on` 同理。原文留在 `extra` 便于审计;缺失计数进 `meta.stats.price_missing`(常态,非错误)。
前端显示 `—`。

---

## 4. 实现

### 4.1 `internal/ths`(协议层,不认识 PIKS 业务)

| 文件 | 职责 |
|---|---|
| `client.go` | cookie 装载、UA、限频(`minGap=1s`)、退避重试、`EnsureSession`、`CookieSource()` |
| `auth.go` | 登录四步 + **纯解析函数**(`parseRet`/`parseItemAttrs`/`parsePassport`/`parseCookieString`/`parseSetCookieHeader`/`encryptPKCS1v15`/`jwtExpiry`,全 stdlib) |
| `selfstock.go` | 两个 GET → `SelfStock{Code,MarketID}` / `Detail{Code,MarketID,Price,AddedOn}` |
| `market.go` | `MarketAbbr(id)` / `IsAShare(id)`(白名单) |
| `name.go` | realhead 取名 → `StockName` / `StockNames` |
| `parse.go` | detail base64 解码 + `ParseAddedOn`/`ParseAddedPrice`(纯函数,NULL 语义) |

**红线**:⚫ 不得为「多分组同步」引入 `multiStorage`/`blockstock` 协议(§1.2)。

### 4.2 `internal/watchsync`(策略层,不 import `internal/web`)

| 文件 | 职责 |
|---|---|
| `watchsync.go` | 纯策略:`Filter`(规范化码 → `IsStockCode` → `IsAShare`,去重)/ `Merge`(按 code 挂价/日)/ `Diff`(镜像语义:本地有上游无 = remove)/ `DueSlot` / `ParseSlots` / `SlotKey` / `BeijingDay` |
| `apply.go` | 编排:`SelfStocks` → `Filter` → **fail-closed 守卫** → `SelfStockDetails`(失败降级)→ `Merge` → `ListEntitiesByStatus("watch")` → `Diff` → 取名(事务外)→ **单事务**落库 → `Stats` |

- **不 import `internal/web`**:web 带 HTTP/AI 依赖,会把整包拖进 tools 镜像(同 `hot-topic` 的取舍)。
  `entityCode` 在包内**本地小实现**(与 web 同名函数同口径)。
- **`Diff` 顺序**:add/keep 先按 code 升序,remove 在后。

### 4.3 迁移 `0020` + store

新表 `watchlist_entries`(`migrations/0020_watchlist_entries.sql`):

```sql
code TEXT PK · entity_id UUID NULL · market TEXT · market_id TEXT
  · added_price NUMERIC(12,4) NULL · added_on DATE NULL · removed_at TIMESTAMPTZ NULL
  · first_seen_at · last_synced_at · extra JSONB · created_at · updated_at
INDEX idx_watchlist_entries_live   (code)      WHERE removed_at IS NULL
INDEX idx_watchlist_entries_entity (entity_id) WHERE entity_id IS NOT NULL
INDEX idx_task_runs_command_started (command, started_at DESC)
```

- **主键 = code 而非 entity_id**:① 上游给的就是 code;② `entity_id` 可能为 NULL(名称未解析);
  ③ 读路径本以 code 聚合。
- store(`internal/store/watchlist.go`):`UpsertWatchlistEntry[Tx]` / `MarkWatchlistRemoved[Tx]` /
  `EntityIDByCode` / `ListWatchlistEntries` / `TaskRunSlotDone` / `CountTaskRunsSince`。
- **`DBTX` 接缝**(`internal/store/dbtx.go`):同一段 SQL 既能在连接池、也能在事务上跑
  (`*pgxpool.Pool` 与 `*store.Tx` 都实现 `DBTX`),这是「整轮原子」的实现基础。

### 4.4 常驻服务 `cmd/watch-sync`(骨架同 `cmd/hot-topic`)

- flags:`-interval 1m`(时钟检查粒度)/ `-slots "09:00,12:55,18:00"` / `-grace 90m` /
  `-tail-grace 4h` / `-once` / `-dry-run`。
- **重启去重靠 DB**:`TaskRunSlotDone(command, slot, since)` —— 同一 slot 当日**成功**跑过则跳过;
  `failed` 必须重试。`SlotKey = "2026-09-22#09:00"` 写进 `meta.slot`。
- **宽容窗**(超窗不补陈旧快照):09:00 / 12:55 = 90m;末个 18:00 = 4h。
  即 14:00 不会去补 09:00 的名单(= 陈旧快照),如实记 missed。
- **周末跳过**(同 hot-topic)。
- **凭据**:直接 `store.ListAppConfig` 读 `ths_cookie` / `ths_account` / `ths_password`
  (`config.ApplyAppConfig` 只认 5 个 `ai_*` 键,**不合并新键**)。**注入 cookie 优先于账密**。
- **记账**:`StartTaskRun("watch-sync")` + `FinishTaskRun`,
  `meta = {slot, trigger, cookie_source, stats{upstream,kept,dropped[],add,keep,remove,deferred_codes[],price_missing,detail_failed}, [dry_run]}`。
- **请求量**:每天 3 轮 × 2 host + 偶发登录 ≈ 10 请求/日,远低于风控阈值;
  请求间 `minGap=1s` + 指数退避 3 次。

### 4.5 接口与前端

- `GET /api/v1/watchlist`(`internal/web/api_watchlist.go`):`apiWatchItem` 加
  `added_price: number|null` + `added_on: string`;handler 内 `ListWatchlistEntries` 建 code→entry 索引合并。
- 前端:`WatchItem` 类型加两字段;`WatchTable.Badges` 加「加入 {date} · {price}」灰字徽标。
  ⚠️ 文案叫**「加入价」不叫「成本价」**(`trades.positions.cost_price` 才是成本价),
  `glossary.ts` 新增 `added_price` 词条说明是同花顺自选口径。
- **用户可见文案去「截图」化**:`glossary.ts`「自选」词条、`WatchGroups` 底部说明、
  `home.tsx`/`QuickStart`/`help.tsx` 的引导文案一律改为「服务器自动同步」,
  截图降级为「自动失灵时的兜底」。`glossary.ts` 是**术语单一真源**(CLAUDE.md 硬规定),
  文案撒谎 = 文档不同步。

### 4.6 凭据写入门控(issue #78 未修前的止血)

`/settings/form` 只回**掩码**(复用 `web.maskSecret`),**绝不回填明文**。
`POST /settings` 的 `ths_account` / `ths_cookie` / `ths_password` 三键
**仅当 `PIKS_ALLOW_THS_CRED_UI=1` 时接受写入**,否则 400
「公网暴露未鉴权期间,同花顺凭据暂不允许从页面写入(issue #78)。请在 lab 直接写 `app_config` 表…」。

- 前端 `ths_cred_ui=false` 时**只显示说明、不渲染输入框**(填了才被 400 拒绝 = 撒谎的 UI)。
- 动机:PIKS 公网可访问且**零鉴权**(#78),若允许任意人写凭据,**攻击者可覆盖凭据把你锁在门外**。
- lab 内网部署可设 `PIKS_ALLOW_THS_CRED_UI=1` 打开页面写入(`configs/.env.prod.example` 已登记)。

---

## 5. 边界(明确不做)

- ❌ **不同步持仓 / 成交**(只做自选)—— 仍走同花顺截图(`/trades` 页)。
- ❌ **不做「多个同花顺分组」** —— 只「我的自选」;不为多分组引入 `multiStorage`/`blockstock`。
- ❌ 不改 web 现有**截图导入路径**(保留兜底;抽取复用留后续)。
- ❌ **不在 PIKS 侧提供手动增删自选的 API** —— 仍是同花顺单向镜像。
- ❌ **不做同花顺自选的写操作**(只读)。

---

## 6. 验收(dev 侧)

| 项 | 结果 |
|---|---|
| `go build ./...` / `go vet ./...` | ✅ |
| DB-free 纯函数单测(`internal/ths` + `internal/watchsync`,8+ 表驱动) | ✅ |
| 迁移 `0020` 干净应用(scratch 库) | ✅ |
| 真库集成测(10 个,双开关 `PIKS_TEST_INTEGRATION` + `PIKS_DATABASE_URL`) | ✅ 全过 |
| 🔴 **加入价/日不被 entity-build 覆盖**(红线回归 `TestWatchlistEntrySurvivesEntityBuild`) | ✅ |
| NULL 往返仍为 NULL(不写成 0) | ✅ `TestWatchlistEntryNullRoundTrip` |
| 幂等(同 UPSERT 两次 → 1 行) | ✅ `TestWatchlistEntryIdempotent` |
| 移出不删行 / re-add 清 `removed_at` 且取新值 | ✅ |
| 名称缺失 → deferred,不建实体;下轮补 `entity_id` | ✅ |
| 上游为空 → fail-closed(不误清空) | ✅ `TestApplyEmptyUpstreamFailsClosed` |
| `selfstock_detail` 失败 → 降级(名单仍同步) | ✅ `TestApplyDetailFailureDegrades` |
| `TaskRunSlotDone`:success 才去重、failed 必须重试 | ✅ |
| 守卫 `check-image-topology.sh`(tools **11** 命令) | ✅ |
| `tsc --noEmit` / `vite build` | ✅ |

**未验(须人工/lab)**:

- ⚠️ **账密登录端到端**(§1.4 / §7);
- **生产端到端**:lab `docker compose logs piks-watch-sync` 确认定点执行;
  `task_runs` 有 `meta.slot` 正确的 success 行;同 slot 重启后不重复;
- **反向验收**:同花顺侧移出一只 → 下轮 `entities.status='archived'` + `removed_at` 非空 + **行未删**;
  移回 → 恢复 `watch`;
- **降级可见性**:改坏 detail 请求 → `meta.detail_failed=true` 且名单仍同步成功(**不是静默 success**)。

---

## 7. 偏差登记(如实)

| 计划 | 实际 | 理由 |
|---|---|---|
| Phase A「账密登录跑通」为硬前置,通过前不写生产代码 | **登录路径未端到端实测**,cookie 注入已实测 | 登录探针须由用户在本地用 `!` 自行运行(凭据不进 AI 上下文)。**兜底已就位**:注入 cookie 优先于账密,功能不依赖登录;登录作为增量。**残余风险**:若账密登录不通,cookie 需人工 ~每周换一次 |
| 计划用 `TaskRunSlotDone(ctx,command,slot,dayStart)` | 实现签名 `TaskRunSlotDone(ctx, command, slot string, since time.Time)` | 同义(去重窗口用 `since`) |
| 计划 `Stats{...,Archived,...}` | 实现 `Stats{Upstream,Kept,Dropped,Add,Keep,Remove,Deferred,PriceMissing,DetailFailed}` | `Archived` 与 `Remove` 同义,合并 |

---

## 8. 风险(登记)

| 严重度 | 风险 | 处置 |
|---|---|---|
| 🔴 高 | **登录未实测**(§1.4) | cookie 注入兜底;账密作增量;失败**不静默** |
| 🔴 高 | **#78 公网零鉴权** —— 账密将落在 `piks.5home.online` 背后 | 表单只回掩码;写入加 `PIKS_ALLOW_THS_CRED_UI` 门控(§4.6);**未修 #78 前,公网侧凭据写入保持关闭** |
| 🟡 中 | 协议非官方,同花顺改版即失效 | 结构漂移一律 failed;被动检测(空名单)优先于主动 exp 判断;失败**不静默** |
| 🟡 中 | 登录频繁触发风控(锁号) | 常驻复用会话;登录 ≤6 次/日 + 退避;**不做重试风暴**;稳态 ~1 次/周 |
| 🟢 低 | 加入价/日为上游 Fact,缺失时为 NULL | 严守 NULL ≠ 0(§3.5),前端显示 `—` |

---

## 9. 红线汇总

- ⚠️ **加入价/日不得进 `entities.detail`** —— 每轮被 entity-build 覆盖(§3.1)
- ⚠️ **成员资格真源仍是 `entities.status`** —— 不迁表(§2)
- ⚠️ **移出不删行** —— `removed_at` 置位,与 `archived` 同构(§3.2)
- ⚠️ **失败不静默** —— 空名单/过滤归零/无凭据/登录失败一律 failed,detail 失败降级须可见(§3.3)
- ⚠️ **只读** —— 绝不向同花顺写自选;PIKS 不提供手动增删 API(§3.4)
- ⚠️ **NULL ≠ 0** —— 上游没给就 NULL,不填 0(§3.5)
- ⚠️ **只「我的自选」一个分组** —— 不引入 `multiStorage`/`blockstock`(§1.2)
- ⚠️ **宁缺毋假取名** —— 不拿 code 当 name 建实体(§1.3)
