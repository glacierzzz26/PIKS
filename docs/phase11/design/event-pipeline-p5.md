# 事件管线 P-5:实时层读接口 + 三档调度 + 保留期(issue #83 分期 P-5)

> 状态:**已实现(dev-only)** · 日期:2026-09-23 · 落地:issue **#83** 分期 **P-5(收口)**
> 前置:P-1(剥转载,PR #92)、P-2(迁移 `0021`,PR #93)、P-3(窗口/榜单/代表,PR #94)、
> P-4(展示单元/粗筛,迁移 `0022`,PR #96)均已合入 dev。
> 本篇 = issue §四 的最后一件:**实时层 + 三档调度(flock)+ 保留期** + §五 验收 + 生产部署。
> **零 schema、零迁移、零新服务、零 LLM 增量。**

## 0. 范围与三处 issue 事实前提更正

issue §四:`| **P-5** | 实时层 / 三档调度(flock) / 其余收口 | ... |`。

### 0.1 四个已与用户锁定的决定

| # | 决定 | 选择 |
|---|---|---|
| 1 | **实时层怎么落** | **复用 C 层 + 只做读接口** —— 盘中采集(常驻 `collector -interval 3m`)已在生产跑,`origin_kind='realtime'` **保留不启用**(保持 inert 契约) |
| 2 | **三档调度 cron 形状** | **重试窗口** —— `early`/`late` 各给一个 10min-tick 重试窗(保幂等/退避/分型),非 issue 字面的单发 |
| 3 | **榜单窗口锚** | **改锚「原始到达时刻」** `COALESCE(raw.published_at, raw.retrieved_at, e.created_at)`(P-3 §1.2 已预先授权重评) |
| 4 | **`deferred` 是否在快讯流可见** | 见 §3.1(修正**文档**向代码靠拢,不隐藏) |

### 0.2 🔴 三处 issue 事实前提按代码更正

issue 正文写在 P-1 落地**之前**,有三处与今日代码不符,本篇按代码更正(**不照字面实现**):

1. **实时层字面已被 C 层取代**。issue P-5 要求实时落 `origin_kind='realtime'`(只落 raw、不进 events)。
   但 **C 层采集(issue #68 / PR #70)已在生产常驻**(`collector -interval 3m`),其行落 **`'pipeline'`**
   且**会被抽取 + 合并**。`origin_kind='realtime'` 至今**无任何生产写入方**(仅测试构造),
   过滤是 **inert 契约**(P-2 已立)。⇒ 本期**不新增写入方、不改 C 层语义**;realtime 的**展示**
   由 raw 层读接口承担(§1)。
2. **窗口锚 `events.created_at` 使早/晚档名存实亡**。全链在收盘后一次跑,`created_at` 全落**晚**窗;
   09:15 早档跑出的 `created_at` 又在晚窗内(09:15–18:30)⇒ **早榜恒空**、早/晚切分只是装饰。
   改锚「**我们何时拿到它**」(raw `published_at/retrieved_at`)后,隔夜消息才真正落早榜(§2,P-3 §1.2 预先授权)。
3. **三档的 realtime 档不在 cron**。盘中轮询已由常驻 `collector` 承担,且 pipeline 的日锁(`.done`)
   **只锁 pipeline 自身**,从来不妨碍 collector(二者是不同进程)——「盘中轮询不被今日已跑锁阻断」
   **今日已成立**。故 cron 只加 `early`/`late` 两条(§4)。

### 0.3 不做

- **独立 realtime 采集路径**(决定 1)、**交易日历**(issue 既定不做,周末闸为临时)、
  **`hot_topic_items` 保留期**(另一条线,另开 issue 登记)、**存量簇回填**(P-3 已定不回填)、
  **cluster-raw-link 的多源簇组装**(仍只表达转载组,见 P-4 §7)。

---

## 1. 实时层 = raw 层滚动读接口(决定 1)

**定位**:实时层**不是**新的采集路径,而是**同一条 C 层采集数据的滚动读窗口**。
盘中每 3 分钟采集已在生产运行;本接口给它一个「看最近 N 小时」的视图。

### 1.1 接口 `GET /api/v1/flashes?hours=N`

- **ADDITIVE 非破坏**:不传 `hours`(或缺省/非正数)行为**逐字不变**;`hours=3` 即滚动近 3 小时。
- 实现:`store.ListRawDocumentsWithSource` 新增 `since time.Time` 参数(零值 = 不限),
  SQL 加 `AND ($2::timestamptz IS NULL OR COALESCE(rd.published_at, rd.retrieved_at, rd.created_at) >= $2)`。
- **锚与榜单窗口同源**(`COALESCE(published_at, retrieved_at, created_at)`):「我们何时拿到它」。
- ⚠️ **不加 `status` 过滤**(见 §3.1):`deferred` 行**仍在快讯流显示**。
- 前端 `pages/flashes.tsx` 增「近 3 小时 / 全部」切换,**写 URL query**(规范 7)。

### 1.2 🔴 红线:不做同事件合并、不排名、不做榜

滚动读口**只是 raw 层的时间切片**——展示的是**原始快讯**,不是事件、不是簇、不是榜。
issue P3 的红线(不排名)在此同样成立:它**不是** `GET /api/v1/board`(那是事件层、按窗口、有印证度),
也不做跨源合并(合并是事件层 cluster 的职责)。

### 1.3 `origin_kind='realtime'` 保持 inert

P-2 立的隔离契约**不动**:worker 的 `ListRawPendingStatus` 与 reconcile 的全部 raw 层查询
一律加 `origin_kind='pipeline'`。今日**无 realtime 行**,过滤是**契约非优化**。
⚠️ P-2 登记的边界仍在:**去重键(迁移 `0016`)不含 `origin_kind`** —— 若将来真的启用实时写入,
须先补「realtime→pipeline 提升」,否则同一篇稿的实时行会与管线行**去重冲突**(本期不启用,故不做)。

---

## 2. 榜单窗口锚改「原始到达时刻」(决定 3)

### 2.1 变更

`store.ListEventsInWindow` 的 `WHERE` 与 `ORDER BY` 锚:

```sql
-- 旧:P-3 锚抽取时刻
e.created_at >= $1 AND e.created_at < $2
-- 新:P-5 锚原始到达时刻
COALESCE(rd.published_at, rd.retrieved_at, e.created_at) >= $1
AND COALESCE(rd.published_at, rd.retrieved_at, e.created_at) < $2
```

`rd` 别名(`LEFT JOIN raw_documents`)在 P-3 已存在,故改动仅两处表达式 + 注释。
`e.created_at` 保留作 `raw_document_id IS NULL` 的**兜底**(dev 有 1 行)。

### 2.2 🔴 这是对 P-3 已交付语义的**授权变更**(必须显式登记)

- **授权来源**:P-3 §1.2 原文已预告「若将来改实时落库需重新评估锚点」;本篇经**用户当面确认**改锚。
- **理由**:见 §0.2 第 2 点 —— 锚 `created_at` 使早榜**结构性恒空**。
- **后果(必须写明)**:早/晚榜的**成员集合会与 P-3 验收时不同**;同一事件可能换档。
  这是**批准的设计变更**,不是漂移。
- **不改**:`boardWindow`/`boardStageNow`/`boardScore`/`handleAPIBoard` **逐字不变**
  (纯窗口边界与响应形状不变),`internal/web/board.go` 仅更新文件头注释。

---

## 3. 保留期清理(28 天,周日 02:00)

### 3.1 `deferred` 在快讯流的**文档-代码不一致**(P-4 遗留,修正文档)

- **现状(代码为准)**:`ListRawDocumentsWithSource` **无 status 过滤** ⇒ `deferred` 行**会**出现在快讯流。
- **P-4 文档误述**写「`deferred` …**不在快讯流显示**」(见 `进度总表.md`、`event-pipeline-p4.md` §7)。
- **处置:修文档向代码靠拢**(**不改代码**)。理由:`deferred` 是**抽取层**的分流(还没轮到抽),
  它的**内容与 raw 同源**(采集已完成);展示层据此隐藏会违背红线「**不得静默隐藏**」。
  改为:「`deferred` 行**仍在快讯流显示**(内容与 raw 同源;抽取后才链上事件)」。

### 3.2 `ReconProcessedNoEvent` 的 NULL 标题崩溃(既有缺口,顺修)

`internal/store/reconcile.go`:`r.title AS detail` → `COALESCE(r.title,'') AS detail`。
**根因**:`title` 可空(金十等源无独立标题字段,由正文派生、派生失败即 NULL),
NULL 扫进非指针 `Detail string` 会报错 ⇒ 对账接口 **500**。与 P-4 无关的另一条线,顺修。

### 3.3 `cmd/cleanup`(第 **13** 个管线命令)

删除 `raw_documents` 中**同时**满足的行(判据 = `store.cleanupWhere` 常量,候选与实删**共用**):

```sql
FROM raw_documents rd
JOIN sources s ON s.id = rd.source_id
WHERE rd.retrieved_at < now() - $1::interval
  AND NOT EXISTS (SELECT 1 FROM events e WHERE e.raw_document_id = rd.id)   -- ① 无事件引用
  AND NOT EXISTS (SELECT 1 FROM raw_documents r2 WHERE r2.canonical_id = rd.id)  -- ② 非转载组代表
```

- flags:`-days 28`(默认)、`-dry-run`(预览候选,与实删**同判据** ⇒ 候选数 == 实删数)。
- `-days <= 0` **拒绝执行**(否则判据退化成「全删」)。
- `task_runs` 记账(`command='cleanup'`,meta = `days`/`deleted`/`dry_run`/`basis`)。

🔴 **判据是单趟的**:只在「删除发生时」保证不删被引用的行 —— 任何时刻都**不留悬空 `canonical_id`**。
若整组(代表 + 成员)同时到期,代表在本趟仍被**存活**成员护住、下趟成员没了才可清,这是
**渐进清理**不是漏删(见集成测试的「护住」用例)。

🔴 **已抽取行永久保留**(事件溯源:`source_url` / 详情抽屉来源 / `cluster_sources` 仍消费它;
且被 `events.raw_document_id` FK 挡下)。故 **「28 天」不是「全体 raw 的 28 天」,
而是「**未被抽取的** raw 行保留 28 天」** —— 文档与 UI 文案须逐字写清。

### 3.4 `scripts/cleanup.sh`(闸门)

- `flock` 单实例(`logs/cleanup.lock`);
- **周日闸**(crontab 已限 `0 2 * * 7`,脚本内双保险);
- **距上次成功 ≥28 天**(`logs/cleanup.last` 秒级时间戳,**仅在成功后更新**;失败留待下个周日);
- 跑 `docker compose run --rm -T tools ./bin/cleanup`;失败**不**更新时间戳,**如实记日志**(#64 教训)。

### 3.5 接线(三处须同步,漏一处即「新命令不上像」)

- `Dockerfile` tools 段加 `COPY --from=build /out/bin/cleanup /app/bin/cleanup`(**逐条列**,禁整目录);
- `scripts/check-image-topology.sh`:12 → **13** 个管线命令(正向清单加 `cleanup`);
- `scripts/deploy.sh`:`TAG_TOOLS` 的 `go_deps_hash_all` 参数**加 `cleanup`**
  (否则改 `cmd/cleanup` 不动 tag → 生产跑旧像,issue #68 同款地雷)。
  ⚠️ 同批**补上此前漏列的 `cluster-raw-link`**(P-4 新增第 12 命令时漏进 hash 联合 ⇒ 改
  `cmd/cluster-raw-link/` 不动 tag 的静默地雷,实测复核发现,一并修)。

---

## 4. 三档调度(决定 2)

### 4.1 `scripts/pipeline.sh` 加 `STAGE` 参数

- `STAGE="${1:-late}"`(`early|late`);**未知档 `exit 2`**(cron 配错要立刻暴露,不静默跑错档)。
- **闸门按档**:`early` = `HMS∈[0915,1159]`;`late` = `HMS∈[1830,2259]`(重试窗上界,防早档拖到下午)。
- **按档记账**:`.done` / `.done.d/` / `.log` / `.fail.*` 文件名一律嵌 `-$STAGE`。
- **按档 flock**:`logs/pipeline-$STAGE.lock` —— 两档**互不阻塞**(否则早档慢会吞掉晚档);
  同档重入即退。
- **步骤按档分两组**:
  - `COMMON`(两档都跑,采集 + 抽取 + 合并):`migrate` · `collector -driver all` ·
    `cluster-raw-link` · `worker -limit 800` · `cluster`。
  - `EOD`(**仅 late**,当日收盘产物):`collector -driver cninfo-announce` · `hot-topic` ·
    `quote-collector -date` · `entity-build` · `watch-sync -once` · `market-state -date` ·
    `daily-review -date` · `reconcile -date`。
  - 🔴 **理由**:`quote-collector`/`market-state`/`daily-review` 是**当日收盘**产物,09:15 早档跑会
    为「尚未发生的当日」出报告(数据空/错)—— **不能进 COMMON**。

### 4.2 `scripts/setup.sh` crontab(2 条 → 4 条)

```
*/10 9-11  * * 1-5  pipeline.sh early     # 早档(09:15 起,10min 重试窗)
*/10 18-22 * * 1-5  pipeline.sh late      # 晚档(18:30 起,10min 重试窗)
0 2        * * 7    cleanup.sh            # 周日 02:00 保留期(≥28d 才真清)
59 23      * * 1-5  backup.sh             # 原样
```

幂等仍靠 `grep -v -F` **逐条**去除再追加(4 行各自去重)。`cleanup.sh` 同步加入 `setup.sh`
与 `deploy.sh` 的 scp 清单(lab 侧脚本随部署同步)。

🔴 **realtime 档不在 cron**:常驻 `collector`(compose 服务,已上生产)已覆盖;
`setup.sh`/compose **不加新服务**。

### 4.3 🔴 已知边界(如实登记)

- **周一早档缺窗**:周末两档均跳过(DOW≥6);周一早档覆盖不了「周日 18:30 →」的窗
  (collector 周末不跑,`inSession` 周日恒 false)。issue 已定**不做交易日历**,故登记不改。
- **COMMON/EOD 拆分改变行为**:早档**不再**跑 `daily-review`/`market-state` 等当日产物
  —— 与旧「一日一次全链」差异显著。

---

## 5. 测试

| 测试 | 覆盖 |
|---|---|
| `internal/store/pipeline_p5_integration_test.go`(集成,新增) | 清理**安全边界**:到期无引用非代表 ⇒ 删;有事件引用 ⇒ 不删(FK/溯源);**被存活成员指向的代表 ⇒ 不删**(防悬空);未到期 ⇒ 不删;dry-run 候选数 == 实删数;幂等(连跑第二次 0 行) |
| `internal/store/pipeline_p3_integration_test.go`(集成,更新) | `ListEventsInWindow` **锚原始到达**:新增 `anchorProof` 用例(raw 早窗 / `e.created_at` 晚窗 ⇒ 归**早**窗);`docID` 助手改收 anchor 参数并显式设 `retrieved_at`(默认已变 `now()`) |
| `internal/web`(回归) | `handleAPIFlashes` 的 `hours` 解析(缺省 = 不限,逐字不变);`board_test.go` 回归 |
| 脚本干跑(人工实跑) | `pipeline.sh early`/`late` 分别在档内/档外/周末触发,核对**按档**台账文件名、flock 名、COMMON vs EOD 步骤集;**未知档 exit 2** |
| `cmd/cleanup`(人工实跑,dev 5433) | `-dry-run` 看候选 → 真跑 → 重跑 0 行;核对「有事件的 raw 未删」 |

**⚠️ 已知 flaky**:`internal/web` 的 `TestTokenSignVerify`(issue #78/PR #91)~6% 概率误报
(篡改 base64 末位有时解码出**相同字节**)—— **与本期无关**,另开 **issue #95**。

---

## 6. 验证

- `go build ./... && go vet ./...` 全绿;`PIKS_TEST_INTEGRATION=1`(本地 5433)对
  `store`/`web` 实跑全 `ok`;`tsc --noEmit`;`scripts/check-image-topology.sh`(**13** 命令);
  `scripts/check-research-isolation.sh`。
- **实跑(dev 5433)**:
  1. 植入一棵 40 天前、无事件引用的 raw 行 → `cleanup -dry-run` 候选 1 → 真跑删 1 → 重跑 0 → 探针行消失;
     核对 `task_runs` 记账(meta 含 `days/deleted/dry_run/basis`)。
  2. `pipeline.sh early|late` 用桩 `date`/`docker` 干跑:early = 5 步(COMMON)、late = 13 步(COMMON+EOD);
     **按档**台账 `pipeline-<date>-[early|late].done.d/` 与锁 `pipeline-[early|late].lock` 分列;
     档外/周末/未知档均正确退出。
  3. `cleanup.sh` 非周日、距上次 3 天两闸均跳过并记 `cron.log`。
- **生产部署**(人工合并 PR 后):从 dev 跑 `./scripts/deploy.sh`(干净树)→ migrate(累积 `0021`/`0022`)
  → up web/research/collector/hot-topic/watch-sync → gateway;重装 crontab(4 条)。
  **无新迁移、无新服务。**

## 7. §五 验收(逐条)

- 早/晚盘榜各生成、窗口内全集、含「哪些渠道 + 各源链接」。
- 簇合并视图(规范标题 + 成员并集 + 多源链接;无链接源如实标注)。
- 印证度三级正确 + **转载不虚增渠道数**(P-1 构造用例)。
- 命名中性事实型,无「已确认/待确认」残留(§2.1 #4 已由 #80 处置,核对)。
- 实时 10min 刷新、**不污染 events**、worker/reconcile 不误处理(`origin_kind` 门控)。
- 三档调度落地(按档记账 + flock)、盘中轮询不被日锁阻断。
- LLM 成本 ≤ `ai_daily_token_budget`(建议 1,000,000;lab 若为 0 = 护栏关闭,需在 `/settings` 设非 0)。
- UI 如实标注「无热度排序,只有印证度标签」。
- ⚠️ **lab 无多源簇** ⇒ 簇合并/印证度只能验**单源退化路径**,如实标注未验项。

## 8. 风险 / 边界

- 🔴 **保留期只清「无事件引用」的 raw 行** —— 已抽取行作为溯源**永久保留**;
  **28 天不是「全体 raw 的 28 天」**(§3.3)。
- 🔴 **窗口锚变更改变 P-3 已交付语义** —— 已在 P-3 §1.2 显式登记「授权重评 + 理由 + 后果」(§2.2)。
- 🔴 **早档依赖隔夜采集** —— 周末 collector 不跑 ⇒ 周一早榜缺「周日 18:30→」(§4.3)。
- **`cmd/cleanup` 是破坏性 DELETE** —— 默认 28 天 + 只清无引用行 + `-dry-run`;先在 dev 实跑核对候选集。
- **不越界**:`origin_kind='realtime'` 不启用;`hot_topic_items` 保留期不做;交易日历不做。
