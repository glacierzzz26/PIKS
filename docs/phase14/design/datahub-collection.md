# PIKS 采集收口：全部数据交给 datahub（phase14 · ✅ 定稿 v1.0）

> **状态：✅ 定稿 v1.0（2026-10-13 经人工确认 D1–D13）。** 定稿后按本文执行，改动须走变更（涉及两仓时两仓 PR 联动）。
> 本文回答一个问题：把 PIKS 现在自己干的**全部采集**迁入独立取数服务 **datahub**、PIKS 退化为**纯消费方**（需要数据一律调 datahub 的 HTTP/MCP），应该长什么样、分几步走。
>
> **一句话**：datahub = 生态里**唯一的数据采集 + 存储 + 供数**方（服务 steady **与** PIKS）；PIKS = **派生 + 展示 + 用户自有数据**方，不再直连任何外部数据源。
>
> **v1.0 定稿（2026-10-13）**：D1–D13 全部**经人工确认**（草案 v0.2 → 定稿 v1.0）；**实现自此解锁**，按 §6 分期推进。§9 尾注的三项「仍需定」为**分期决定项**（随相应期落地时定），非阻塞。
> **v0.2 变更（2026-10-13）**：§9 的 7 个未决问题**逐个敲定**（D7–D13）：原文存亡走 **A**（可丢弃镜像）、游标用 **seq**、拓扑 **同宿主无 TLS**、研报取数 **全迁分期**、保留期 **R3 归档式**、phase13 **P14-1 兼任 P13-1**、存量 **回填**。

---

## 0. 决策记录（✅ 2026-10-13 定稿，D1–D13 经人工确认）

| # | 决策 | 选择 | 备注 |
|---|---|---|---|
| D1 | **接管范围** | **全交** | 快讯 6 源 / 公告 / 热榜 / 涨停·指数 / 研报取数 → datahub；**datahub 负责维护所有数据** |
| D2 | **事件原文权威** | **datahub 完全权威** | PIKS 不作为原文的独立真源（子决策见 §4.3） |
| D3 | **自选（凭据类）** | **留 PIKS** | `cmd/watch-sync` 与 `ths_*` 凭据**不迁**；凭据类源不进 datahub |
| D4 | 出口形态 | **HTTP 为主契约 + MCP 薄门面** | 沿用 datahub 现状（两出口共享 `service.get_dataset`） |
| D5 | 迁移形态 | **拒绝 big-bang** | 逐数据集**闸门灰度 + 对账 + 可单步回滚**（对齐 steady datahub Phase 2/3） |
| D6 | 交付 | **先设计后实现** | ✅ **定稿门已过**（2026-10-13）；实现按 §6 分期解锁 |

**子决策（v0.2 逐个敲定，对应 §9）**

| # | 决策 | 选择 | 落点 |
|---|---|---|---|
| D7 | **`raw_documents` 存亡** | **A：datahub 权威 + PIKS「可丢弃工作镜像」** | §4.3、§4.4 |
| D8 | **`stream` 游标形态** | **`seq BIGSERIAL`**（提交序单调；拒时间窗） | §3.1 |
| D9 | **部署拓扑 / TLS** | **同宿主 + docker 共享网络，无 TLS** | §7 |
| D10 | **研报取数** | **目标全迁、分期**（先迁 datahub 原生更强的行情/财务/估值） | §4.5、§6 |
| D11 | **保留期清理** | **R3 归档式**（datahub 侧保守归档；精细判据留 PIKS 镜像） | §3.5、§8 |
| D12 | **与 phase13 合流** | **P14-1 兼任 P13-1**（合流是构造出来的） | §2.3 |
| D13 | **存量原文回填** | **回填**（跨实例 `pg_dump\|psql`，幂等） | §6 |

D7 的实质：**"权威"= 谁拥有/维护原文**，非"本地不能有副本"。A 下 datahub 拥有采集/存储/保留期；PIKS 镜像是**纯派生、可 `TRUNCATE` 重建**的工作集，不承担维护责任——与 steady §5.3「本地只读缓存非权威副本」同理念，只是升级为"可重建镜像"。

**非目标（明确不做）**
- ❌ 不迁**自选**（`cmd/watch-sync` / `ths_*` 凭据 / `entities.status` 写入链路）。
- ❌ 不迁**派生**（`worker` 抽取 / `cluster` 聚类 / `entity-build` / `market-state` / `daily-review` / `reconcile` / 研报生成）——这些是 PIKS 的**计算职责**，留在 PIKS。
- ❌ 不改前端业务逻辑（仅**数据来源**改，呈现不变）。
- ❌ 不动用户自有数据（交易 / 持仓 / 笔记 / 周报 / 决策记录）。

---

## 1. 现状：PIKS 采什么、存哪、谁消费

**以代码为准**（`docs/数据源总览.md`，dev `06c4cfd`）。PIKS 全部外网采集 = **4 个 Go 入口 + 1 套 Python 按需取数**：

| 链路 | 入口 | 源 | 落库 | 消费者 |
|---|---|---|---|---|
| **快讯** | `cmd/collector -driver news/all`（常驻 + 早晚档） | 6 机构源（东财/金十/财联社/新浪/同花顺/富途） | `sources` + `raw_documents`(`status=raw`) | `worker`(LLM 抽取) → `cluster` → 前端消息页 |
| **公告** | `cmd/collector -driver cninfo-announce` | 巨潮资讯网 | `raw_documents`(`status=collected`, `grade`) | 前端公告 tab（**不进 LLM**） |
| **热榜** | `cmd/hot-topic`（常驻） | 2 源（同花顺话题榜 / 财联社热文） | `hot_topic_items`（**独立表/进程**） | 前端热榜页（**不进事件链路**） |
| **涨停/指数** | `cmd/quote-collector` | 东财 push2ex / push2 | `observations`（12 指标） | `market-state` / `daily-review` |
| **研报取数** | `python3 -m src.cli`（按需，UI/CLI） | akshare（腾讯/申万/NBS/东财） | `research_runs.metrics`(JSONB) | 研报页 / 个股中心速评 |
| **自选** | `cmd/watch-sync`（常驻，**留 PIKS**） | 同花顺「我的自选」（**需凭据**） | `entities.status` / `watchlist_entries` | 自选首页 |

**关键事实**（决定改造难度）：
1. **事件原文 `raw_documents` 是血缘锚点**：`events.raw_document_id UUID REFERENCES raw_documents(id)`（迁移 0001），且**去重键（0016）/ `grade`（0017）/ `origin_kind`（0021）/ `canonical_id`（0022）** 都挂在这张表上；前端事件抽屉、`cluster_sources`、`reconcile`、`cleanup` 全读它。
2. **反封禁不变量是生产事故换来的**（C 层）：per-host 令牌桶 / 空响应哨兵 / 熔断、财联社签名、巨潮 POST+`pageSize≤30`+`column=szse`、财联社热文 SSR 解析**报错不空成功**（#64）—— 迁采集 = **这些必须一并搬且逐条重验**（§4.2、§8）。
3. **PG 里没有全市场行情表**（`架构总览` §2.5）：个股 K 线只在研报运行时临时取 → 这正是 datahub（承载 steady 的 `daily_price` 等）能补的硬缺口。
4. **datahub 已就位**：独立仓、Python（FastAPI+FastMCP）、**HTTP `/v1/datasets/{id}` + MCP `/mcp` 双出口**、Bearer 鉴权、自有库 `datahub`（与 steady **同 PG 实例** `quant-postgres` 独立库）、`external`/`raw` 两类数据集、**双闸门默认全关**。**2026-10-13 起已是 steady 的唯一采集方**。

---

## 2. 目标架构

### 2.1 终态

```
        外部源（东财/金十/财联社/新浪/同花顺/富途/巨潮/BaoStock/腾讯/申万/NBS…）
             │  采集（唯一出口 = datahub）
             ▼
  ┌──────────────── datahub（独立仓 / 独立部署 / 自有 DB）────────────────┐
  │  采集层  provider 注册表 + 每数据集源链 + 限流/缓存/降级/黑名单         │
  │  存储层  原始数据 —— **单一真源**                                      │
  │          raw：行情/估值/财务/日历/股票                     （steady 已有）│
  │          external：热点/行业                              （steady 已有）│
  │          新增：事件原文(快讯/公告) / 热榜 / 涨停·指数观测 / 研报数值域     │
  │  出口层  MCP 门面(Streamable HTTP+token) │ 原生 HTTP API(+token)        │
  └───────┬─────────────────────────┬───────────────────────┬─────────────┘
          │ 原始数据（HTTP/MCP）      │                       │
          ▼                          ▼                       ▼
   steady（只算因子/信号）      PIKS（只派生/展示）       其他/AI 客户端
                                ├ worker/cluster/entity/market-state（派生，读 datahub）
                                ├ 前端（经 web 读 datahub 或读本地派生投影）
                                └ cmd/watch-sync（自选，唯一保留的 PIKS 采集，D3）
```

### 2.2 边界总表

| 维度 | datahub | PIKS |
|---|---|---|
| 职责 | 采集 + 存储 + 供应**原始数据** | **派生**（事件/聚类/实体/市场/研报）+ **展示** + **用户自有数据** |
| DB | 自有库 `datahub`，含**全部原始表** | `piks` 库：**派生表 + 用户数据**（原文不再作独立真源，见 §4.3） |
| 对外 | HTTP + MCP（供 PIKS/steady/外部） | 前端 REST `/api/v1`（现状） |
| 依赖方向 | datahub ⇢ 外部源；**不依赖 PIKS/steady** | PIKS → datahub（**单向**） |
| 外网 | **唯一出口** | **零外网**（除 LLM 网关 + 自选同花顺） |

### 2.3 与 phase13 的关系（顺带交付 · D12）

phase13「STEADY 经 MCP 接入（唯一）」的 `McpProvider`，其**服务端就是 datahub 的 MCP 出口**。本方案把 datahub 的**数据集目录扩到覆盖研究所需**（行情/财务/**估值分位**/资金流/行业/宏观，§3.4）后，phase13 的数据底座（P13-1）**即由 datahub 提供**——两阶段在此合流，不再重复造接入层。

🔴 **D12：不专门"一次做全"，让 **P14-1 兼任 P13-1**。** P14-1（研报数值域）首批补的**行情/财务/估值**正是 phase13 的核心缺口（估值历史分位，akshare 拿不到）；做完 P14-1，phase13 的 `McpProvider` 直接有数。资金流/宏观等**随 phase13 各分析师按需增量加**，不提前造用不上的数据集。datahub 数据集**机械同时出 HTTP + MCP**，`McpProvider` 复用而不另造。

---

## 3. datahub 侧改造

### 3.1 新增数据集 kind：`stream`（追加流水）

datahub 现有两种 kind 都不适配**事件原文**：
- `external` = 抓外部源 + **TTL 缓存**（"读当前形状"）；
- `raw` = 读自有库（"按 key/区间读表"）。

而**快讯/公告是追加日志**——按**到达顺序增量读**，不是"读当前 800 行"。新增第三种 kind：

```python
DatasetKind = Literal["external", "raw", "stream"]   # 新增 stream
```

**`stream` 语义**（新）：
- **游标增量**：`since` = **`seq`（单调到达序）** → 只返回 `seq > since` 的**新到达**行；**幂等**（每行 id/seq 稳定，消费方按 `datahub_id` 去重）。
- **不缓存**（本地库权威，同 `raw`）。
- **空结果合法**（无新行 = 正常，不转 503，同 `raw`）。
- **冻结列契约**：`id, seq, source, external_id, url, title, content, content_hash, published_at, retrieved_at, extra(jsonb), grade`。
- **发布语义进契约**：`status` 由 PIKS 派生侧维护（§4.3），datahub 只保证"原文 + 到达时刻"。

> 🔴 **D8：游标 = `seq BIGSERIAL`（决定）**，拒用 `retrieved_at` 时间窗。理由：时间窗有**并发乱序漏读**——两行可能 `retrieved_at=T` 与 `T+ε`，但 `T` 行**晚提交**，时间窗一旦跨过就再捞不回；`seq` 由 DB 序列在**提交序**上单调，天然无此洞，且**免时钟依赖**。初始水位 = 回填时取 `MAX(seq)`；清理删行留 **seq 洞无害**（游标只前进）；`limit` 分页。

### 3.2 新增采集器（从 PIKS Go **移植**到 datahub Python）

把 PIKS 的采集驱动**逐条搬进 datahub**（`app/collectors/`）：

| 迁入 | 原落点 | 目标 |
|---|---|---|
| 快讯 6 源 | `internal/collector/{dongcai,jin10,cls,sina,ths,futu}.go` | datahub `app/collectors/news/` |
| 巨潮公告 | `internal/collector/cninfo_announce.go` | datahub `app/collectors/announcement.py` |
| 热榜 2 源 | `internal/collector/hottopic.go` | datahub `app/collectors/hottopic.py` |
| 涨停池/指数 | `internal/collector/quotemarket.go` | datahub `app/collectors/quote_market.py` |
| 反封禁护栏 | `internal/collector/limiter.go`（令牌桶/哨兵/熔断） | datahub `app/ratelimit.py` 扩展 |
| 限频退避 | `internal/collector/http.go` | datahub `providers/net.py` + `ratelimit` |

🔴 **必须一并搬且逐条重验的生产硬化不变量**（漏一条就是重演事故，见 §8）：
- per-host **令牌桶（2req/s·突发 3）/ 空响应哨兵（连续 3× 降速）/ 熔断（连续 4 失败开路 60s）**——且**采集必须常驻**（护栏跨轮累积，一次性进程状态归零）；
- 财联社**签名** = `md5(sha1(参数按 key 升序拼))` + `Referer`；
- 巨潮**必须 POST**、`pageSize≤30`、`column=szse & plate=szsh`、`minGap=1s`、**只存标题+PDF 外链**；
- 财联社热文 **SSR 解析报错不空成功**（#64 教训）；`rank` = 上游下标 + 1、**不得重排**；
- 去重键**按源分派**（`external_id` 非空 → `UNIQUE(source_id,external_id,content_hash)`，迁移 0016 语义）——**搬到 datahub 侧存储时同样保留**。

### 3.3 新增存储（datahub 库）

`deploy/postgres/init.sql` 或迁移新增：
- `news_documents`（快讯原文）/ `announcement_documents`（公告）/ `hot_topic_items`（热榜）/ `market_observations`（涨停·指数）——**列照 PIKS 现有表逐字冻结**（`raw_documents`/`hot_topic_items`/`observations`），带 `seq BIGSERIAL` 供 `stream` 游标。

### 3.4 研报数值域（补 phase13 数据底座）

复用 datahub **已有** raw 数据集（`daily_price`/`daily_valuation`/`financial_indicator`），**新增**：
`capital_flow`（龙虎榜/北向/两融）、`industry_index`（申万指数日线）、`industry_members`（已有）、`macro`（CPI/PPI/M2/GDP）、`news_by_stock`（个股新闻）、`announcement_by_stock`——**列照 research 现用 akshare 输出冻结**，各数据集**自动同时出现在 HTTP 与 MCP**（datahub 机械生成）。

### 3.5 保留期清理（D11 · R3 归档式）

现有判据 `cleanupWhere`（issue #83 P-5）= 到期 **AND** `NOT EXISTS events.raw_document_id` **AND** `NOT EXISTS r2.canonical_id=rd.id`——**后两个否定条件都是 PIKS 派生事实**（events、转载组代表），**datahub 都不知道**。硬塞给 datahub 就要新增回写通道，破 datahub「对外只读」护栏（§8#5）。

🔴 **D11 决定：走 R3「归档式」**——
- **datahub 侧**：原文**保守归档**（默认长期/永久保留；如需上限，用一个**很长的安全窗口**，如 180 天，覆盖任何可能的派生引用窗口）——**不搬运精细判据**。
- **PIKS 侧**：现有 `cmd/cleanup` + `cleanupWhere` **原样作用于镜像**（`datahub_id` 关联），红线「已抽取永久保留」**不动**。
- 理由：① 判据天然属于 PIKS；② 原文归档是资产（event 溯源/重抽取/印证都要历史），存储 0.23GB/年（公告）+ 快讯同量级，数年不过 GB；③ issue #83 P-5 的「28 天」本意是**给盘中高频采集止血**，那个"血"其实在 **PIKS 镜像**上。

> **不采纳** R1（PIKS 回报式：新增回写通道）与 R2（PIKS 驱动 datahub admin 删除：开写端点）——二者都把判据/写权限散到两处，破单一真源纪律。**留作将来升级项**（若 datahub 原文增长需精细清理，再上 R2）。

---

## 4. PIKS 侧改造

### 4.1 datahub 客户端（新）

新增 `internal/datahub`（Go）：Bearer 鉴权、信封 `{code,message,data,meta}` 解析、`timeout(connect,read)`、**只对瞬时故障重试**、进程内短 TTL 缓存、`DATAHUB_READ_DATASETS` **读闸门**（默认空 = 读本地 = **零行为变更**）。两套消费方（`cmd/*` 与 `web`）共用同一份 env。**仿 steady `app/datahub_client.py` + `data_source.py` 的形态**。

### 4.2 采集入口改为「拉取」

- `cmd/collector`：`Fetch(源)` → **调 datahub `stream` 端点 → 落本地**（快讯）/（公告）。
- `cmd/hot-topic` / `cmd/quote-collector`：同理改拉取（或直接由 web 读 datahub）。
- 反封禁护栏**随采集一起搬走**（§3.2），PIKS 侧采集代码删到只剩"拉取 + 落本地"。

### 4.3 🔴 关键子决策：`raw_documents` 在 PIKS 的存亡（D2 = datahub 完全权威）

D2 定了"原文权威归 datahub"，但 PIKS 的**派生链**（`worker` 必须读 `content` 抽取、按 `status` 记账、`events` 外键引用、`reconcile`/`cleanup`/前端抽屉/`cluster_sources` 全读硬连）注定还要**读**原文。两条路线：

**子方案 A（D7 决定）：datahub 权威 + PIKS 保留「可丢弃工作镜像」**
- datahub 是**唯一真源**（采集在此、保留期在此、对外供数在此）；
- PIKS 侧 `raw_documents` **重新定位为"派生工作集"（read-mirror）**——由"拉取步"从 datahub **增量同步**而来，**显式非权威、可整表重建**（`datahub_id` 为业务键）。
- **派生链几乎不动**（worker/cluster/reconcile/抽屉照旧读本地），**改动面最小、风险最低**；代价 = 原文在 datahub 与 PIKS 各存一份（存储翻倍，但原文量级小：~180 快讯/日 + ~1200 公告/日）。
- 与 D2 的关系：**"镜像"不是第二真源**——它可随时 `TRUNCATE` 重建，datahub 侧才是权威；这等价于 datahub 的"读闸门关"本地副本（steady §5.3 同款缓存理念）。

**子方案 B（纯零副本，改动大 · 不采纳）**
- PIKS **完全不留** `raw_documents`：`events.raw_document_id` 改为**逻辑引用** datahub id（**去 FK**，迁移）；`worker` 按"未抽取游标"**按需向 datahub 拉**原文；`reconcile`/抽屉/`cluster_sources` 全部改为**读 datahub**（经 web 代理）。
- 更纯粹、真正单一真源；代价 = **整个派生读路径改走跨系统调用**、`cleanup`/`reconcile` 语义重写、读延迟上升、datahub 成前端展示硬依赖。

> 🔴 **D7 决定：走 A。** 理由：D2 的实质是"**datahub 负责维护**数据"，A 完全满足（镜像无维护责任），且把跨系统改造压到最小——与 D5「拒绝 big-bang」一致。B 留作将来"极致单一真源"的演进项，本阶段不做。

### 4.4 `events.raw_document_id`（D7 = A）

- **D7 决定（A）**：仍指向**本地镜像** `raw_documents(id)`（**FK 不变，迁移 0**）；镜像行携带 `datahub_id` 供回溯。
- （不采纳的 B 会去 FK、逻辑指向 datahub id——见 §4.3。）

### 4.5 research 取数改走 datahub（替 akshare · D10）

`research/src/providers/*`（唯一 akshare 使用处，§1）逐域改调 datahub：`market→daily_price`、`financial→financial_indicator`、`valuation→daily_valuation`、`capital→capital_flow`、`industry→industry_index`、`macro→macro`。**优先经 MCP**（供 phase13 的 `McpProvider` 复用）。红线不变：**Number Lint / 防未来函数（`as_of` 截断在 datahub 侧或适配层）**。

🔴 **D10 决定：目标全迁（D1 已定"全交"），分期落地、顺序有讲究**：
1. **先迁 datahub 原生更强的那批**（行情/财务/**估值**）：datahub 的 `daily_price`/`financial_indicator`/`daily_valuation` 是**全历史 + 800 只 + 复权因子**，**严格强于** akshare（估值长期 `unavailable`、行情单源）→ 迁过去是**净收益**，且顺带**解锁 phase13 的估值历史分位**（D12）。
2. **再迁需新数据集的那批**（capital 龙虎榜/北向、industry 申万、macro NBS、news、announcement-by-stock）。
3. **不保留 akshare 常驻**——全迁完成后 research 也**零外网**，与 D1 一致。

### 4.6 删采集代码（收口阶段）

`internal/collector/*` 外部源驱动、`internal/announce`（分级规则迁 datahub 或保留为展示规则）、`internal/ths`（自选，**保留**）——**仅删采集驱动，分级/展示规则按归属重定**。CI 守卫：**断言 PIKS 内不得出现外部源直调**（对齐 steady 的 `import akshare` 守卫）。

### 4.7 前端

**呈现零改动**；仅**数据来源**改（经 web 代理 datahub 或读本地派生投影）。事件抽屉/消息页读路径在方案 A 下**完全不变**。

---

## 5. 契约：数据集目录（新增部分）

沿用 datahub 契约原则（**冻结列/单位显式/防未来函数/批量优先/统一信封**）：

| 数据集 id | kind | 参数 | 关键列 | 对应 PIKS 现有 |
|---|---|---|---|---|
| `flash_documents` | **stream** | `since`,`limit`,`source` | id,source,external_id,url,title,content,published_at,retrieved_at,extra | `raw_documents`(快讯) |
| `announcement_documents` | **stream** | `since`,`limit`,`column` | id,external_id,title,url(adjunctUrl),announcement_type,retrieved_at,extra | `raw_documents`(公告) |
| `hot_topics` | raw/stream | `source`,`date`,`limit` | source,rank,title,code,hot_value,url,extra | `hot_topic_items` |
| `ladder` | raw | `date`,`pool`(zt/zb/dt) | code,name,board_days,industry,change_pct,… | `observations`(涨停池) |
| `market_observations` | raw | `date`,`indicator` | indicator,value,change,observed_at | `observations`(指数/家数/成交额) |
| `capital_flow` | raw | `codes`,`start`,`end` | code,trade_date,lhb/北向/两融… | research akshare |
| `industry_index` | raw | `codes`,`start`,`end` | code,trade_date,close,change_pct | research 申万 |
| `macro` | raw | `indicator`,`start`,`end` | indicator,period,value | research NBS |

> **粒度/单位/防未来函数语义随契约冻结**；`stream` 的 `since` 游标 = **`seq`**（D8，见 §3.1）。

---

## 6. 迁移阶段（P14）—— 逐集灰度、每步对账、可单步回滚

> 原则照 steady datahub Phase 2/3：**每数据集一把 env 闸门（默认关 → 部署零行为变更）× 同一时刻单一采集方（防双份）× 切换前逐位对账 × 一键回退**。

- **P14-0 方案定稿**（本文）——✅ **已过定稿门（2026-10-13，D1–D13 人工确认）**。
- **P14-1 研报数值域**（**= phase13 的 P13-1**，D12）：datahub 补 §3.4 数据集 → research `providers` 切读（读闸门）→ **同一只票同一区间，新链路 vs 旧 akshare 逐位一致**为放行门。**先做低风险、且顺带交付 phase13 底座**。
- **P14-2 涨停池/指数**：datahub `quote_market` 采集 + `ladder`/`market_observations` 出口 → `observations` 12 指标对账 → 切采集方。
- **P14-3 事件原文（最大风险）**：datahub 上 `stream` kind + 移植快讯 6 源 + 公告 → **逐源对账**（同窗口抓取，`content_hash`/`external_id` 对齐）→ 切快讯采集方；**反封禁不变量逐条重验**（§3.2）。
- **P14-4 热榜 2 源**：迁入 + 对账（两源逐条一致、`rank` 不重排）。
- **P14-5 收口**：镜像同步定型、**D11 保留期策略落地**、删 PIKS 采集代码、CI 守卫落地。

> 🔴 **D13 存量原文回填（在 P14-3 之前做一次）**：PIKS 历史 `raw_documents`（`piks-postgres`）**跨实例**回灌 datahub（`quant-postgres`）——`pg_dump --data-only | psql`，逐列映射（`source/external_id/content_hash/url/title/content/published_at/retrieved_at/extra`），**去重键（0016 语义）照搬保幂等**（复刻 steady `seed_from_steady.sh` 范式）。回填后取 `MAX(seq)` 作镜像同步初始水位；**避免历史双写**。

> **停止顺序是硬约束**：每个数据集"datahub 采 + PIKS 拉"**并行对账通过**后，才停 PIKS 采集；"datahub 采"与"PIKS 采"**同一数据集不得同时为真**。

---

## 7. 部署 / 鉴权 / 拓扑

✅ **D9 决定：同宿主 + docker 共享网络，无 TLS。** 事实依据：PIKS 在 lab `192.168.0.202`（自有 PG `piks-postgres` + 网络 `piks`）；datahub 与 steady 同处 `quant-postgres`（另一实例）——**三者同一台宿主**。
- **实现**：建一条 external 网络（如 `piks-datahub`），PIKS 的 `web/research/tools` 容器与 datahub 都接；PIKS 按容器名调 `http://datahub:8100`。datahub 无需改绑（凭网络可达）。**流量不出宿主 → 无需 TLS、无 LAN 面**；与 steady「加入 PG 网络按容器名连」同构。
- **TLS：本阶段不做**；仅当将来 datahub 迁独立宿主机才上（届时 token + `DATAHUB_MCP_ALLOWED_HOSTS` 带 `:*` + nginx TLS——后者是 datahub config 里已写死的 421 坑）。
- **鉴权**：`DATAHUB_TOKEN`（Bearer，fail-closed）；若经 nginx 反代须 `proxy_set_header Authorization`（否则 401 假象）。
- **版本/发布**：datahub 独立仓、独立版本（`v0.0.0-<短hash>` + 镜像内烘焙 `APP_VERSION`/`GIT_SHORT`），与 PIKS 发布解耦。
- **DB**：datahub 库（`quant-postgres`）与 PIKS 库（`piks-postgres`）**不同 PG 实例**；回填走 `pg_dump|psql`（D13）。注意 datahub `DB_POOL_SIZE` 已调小（与 steady 共享 `quant-postgres`）。

---

## 8. 风险与取舍

| 风险 | 说明 | 缓解 |
|---|---|---|
| **Go→Python 移植丢不变量**（最大） | 反封禁护栏/签名/SSR 解析/去重键都是**事故换来的**；一次性迁移 = 把踩坑重验一遍 | §6 逐源灰度 + 逐条不变量清单 + 同窗口对账；**禁止 big-bang** |
| **新单点** | datahub 挂 → PIKS 采集/研报取数停（与 steady 同因） | 同宿主降网络风险、datahub 自备 restart/healthcheck、PIKS 侧短 TTL 缓存 + 读闸门一键回退 |
| **跨系统读耦合** | 前端事件抽屉/cluster_sources 若依赖 datahub 在线 | **D7 = A**（本地镜像，读路径不变）——已消解 |
| **存储翻倍**（D7 = A） | 原文在 datahub + PIKS 各一份 | 量级小（~1.4 千行/日）；镜像可整表重建 |
| **契约跨仓漂移** | 列变更需 datahub+PIKS 两仓对齐 | 契约冻结 + 契约测试 + 版本化 + 变更须两仓 PR 联动 |
| **自选游离在外** | D3 留 PIKS，采集不再"全在一个出口" | 如实登记：**凭据类源是唯一例外**（凭据不外流是刻意的安全边界） |
| **保留期归属** | datahub 不知道"某原文已无派生引用" | **D11 = R3**：datahub 保守归档、精细判据留 PIKS 镜像（§3.5）；将来增长需清理再上 R2 |

---

## 9. 子决策记录（v0.2 已逐个敲定）

> 原 v0.1 §9 的 7 个未决问题，本轮（2026-10-13）**全部敲定**：

| # | 问题 | 决定 | 落点 |
|---|---|---|---|
| 1 | 🔴 `raw_documents` 存亡 A vs B | **A**：datahub 权威 + PIKS「可丢弃镜像」（可 `TRUNCATE` 重建） | §4.3、§4.4 |
| 2 | `stream` 游标 | **`seq BIGSERIAL`**（提交序单调；拒时间窗的乱序漏读） | §3.1 |
| 3 | 部署拓扑 / TLS | **同宿主 + docker 共享网络**（`http://datahub:8100`），**无 TLS** | §7 |
| 4 | 研报取数全迁 vs 部分 | **目标全迁**（D1），**分期**：先行情/财务/估值（数据更强），再 capital/industry/macro/news/announcement | §4.5 |
| 5 | 保留期清理判据 | **R3 归档式**：datahub 保守归档；精细判据留 PIKS 镜像（不采纳 R1 回报/R2 admin 删除） | §3.5 |
| 6 | 与 phase13 合流点 | **P14-1 兼任 P13-1**（合流是构造出来的，不专门"一次做全"） | §2.3 |
| 7 | 存量原文回填 | **回填**：跨实例 `pg_dump\|psql`，幂等，取 `MAX(seq)` 作初始水位 | §6 |

**分期决定项（不阻塞定稿）**：datahub 归档窗口的具体天数（默认长期）；保留期判据将来是否升级 R2；phase13 各分析师的补口清单（随其落地增量）。以上均**随相应期落地时定**，不影响本次定稿。

---

## 10. 验证方法（实现阶段用）

1. **契约**：每个新 `DatasetSpec` 列对 datahub 侧 `information_schema` 断言一致；`stream` 幂等/游标推进单测。
2. **采集对账**：每数据集「datahub 采集产物 vs PIKS 采集产物」**同一窗口逐位零偏差**（沿用 datahub `reconcile_table.py` 范式）。
3. **不变量**：单元/集成测覆盖护栏三件套、财联社签名、巨潮 POST 分页、SSR 解析报错、去重键分派。
4. **鉴权负例**：无 token → 401。
5. **无双份采集**：`datahub 请求数 ≈ 采集轮数`，PIKS 侧外部源请求归零（CI 守卫）。
6. **读闸门回退**：闸门关 = 逐字回退本地，生产零行为变更。
7. **end-to-end**：迁移后**一个完整交易日**管线（早晚档 + 盘中常驻）产物与迁移前一致。

---

## 11. 文档同步清单

| 文档 | 改动 |
|---|---|
| `docs/phase14/design/datahub-collection.md` | **新建**（本文） |
| `docs/phase14/design/README.md` | **新建**（索引） |
| `docs/数据源总览.md` | 采集链路由「PIKS 4 入口」→「datahub（PIKS 消费）」，各期落地后改 |
| `docs/架构总览.md` | 数据流骨架加 datahub 分支；§8 采集/调度改写；三运行时 → 加 datahub |
| `docs/项目详解.md` | 加「采集收口 / datahub 作为生态唯一数据方」 |
| `CLAUDE.md` | 采集边界、`DATAHUB_READ_DATASETS` 闸门、容器拓扑变化 |
| `docs/进度总表.md` | 加 phase14 行（⚠️ 待并行的 phase12/phase13 行落定后补登）；各期落地归档 |
| `README.md` | 使用方式/配置若变（env/token）同步 |
| **datahub 仓** | 服务级 README + 数据集契约 + 采集器文档（该仓自身 PR） |
| **steady 仓** | 上位设计 `数据接入层-datahub.md` 补「边界扩到 PIKS」 |
