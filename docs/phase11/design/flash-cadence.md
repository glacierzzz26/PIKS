# C 层:快讯提频(盘中 3 分钟)+ 反封禁护栏 ✅ 已实现(dev-only)

> 阶段:数据源分层(issue #68,epic #43 续篇)的 **C 层 = 快讯改用法**。2026-09-21。
> 前置:[source-tiering.md](./source-tiering.md)(A/B/C/D 分层总设计,§5 即本层)、
> [announcement-grading.md](./announcement-grading.md)(S1 公告分级)。
>
> ⚠️ **本文是 C 层的实现记录**。总设计 §5 定的是「做什么、为什么」,本文落「怎么做的 +
> 实测数据」。分层总设计(`source-tiering.md`)与 S1 记录随 PR #69 合入 dev;C 层的实现
> 独立开在 `dev` 上(本 PR),故 S2 的实测结论与 S3 的护栏/常驻设计**在此独立成篇**。

## 1. 目标与结论

C 层两件事(设计 §5):

| 子项 | 问题 | 定夺 |
|---|---|---|
| **S2** 新浪去留 | 担心新浪是聚合平台、多转载金十 → 污染「独立信源数」 | **保留新浪**(见 §2);「转载≠独立源」修在根因层(#45 P2 剥转载) |
| **S3** 提频 + 反封禁护栏 | 现状每交易日**只采 1 次**(16:10 后),盘中不采 | 提频到**盘中每 3 分钟**轮询,**硬前置** per-host 三条护栏(见 §3/§4) |

**产出**:交易时段(工作日 09:15–15:05)每 3 分钟采一轮 6 个快讯源;采集侧有 per-host
护栏保证不触发上游封禁。**零 schema 变更**。

## 2. S2 结论:保留新浪(用生产数据测定,非拍脑袋)

**方法**:用**与生产聚类同一把尺子**(`cluster.NormalizeTitle` + `Bigrams`/`Jaccard`,
**阈值 0.7 不降**)比对 lab 生产 `raw_documents` 里新浪与金十的条目。

| 方向 | 近同对 | 占比 |
|---|---|---|
| 新浪 → 金十 | 9/80 | **11.2%** |
| 金十 → 新浪 | 9/78 | **11.5%** |

**结论**:**不支持**「新浪多数转载金十」—— 重合仅 ~11%,且**双向对称**(若新浪单向转载金十,
两个方向应显著不对称)。窗口内多数新浪条目为**新浪独有**(卡塔尔能源 / 花旗 CEO / 白宫哈塞特
等发布会速记只在新浪出现)。

→ 采纳设计**选项 A:保留新浪**。「转载≠独立源」是**根因问题**,由 **#45 P2 剥转载**在
   §2 层解决(剥掉转载前缀后,转载条目会与原文**聚成同一条**,自然不会各记一源)。

> ⚠️ **不得**以「数据证明无用」为由删源(设计 §10 红线)。若日后仍要删,须记明是
> **策略性删除**并写清理由,不得伪装成「数据结论」。

> **顺带纠正**:设计 §5.2 写去重键为 `UNIQUE(source_id, content_hash)`,**已过时** ——
> 迁移 `0016`(issue #50)已按「源是否带 `external_id`」分派,快讯各源走
> `UNIQUE(source_id, external_id, content_hash)`。见 `docs/数据源总览.md` §2.1 落库说明。

## 3. 🔴 为什么护栏是提频的硬前置(实测佐证)

免费源**无 SLA**,提频前必须先有护栏,否则就是主动触发封禁:

| 实测 | 结果 |
|---|---|
| 东财 push2 频繁请求(issue #43 记录) | **SSL RST**(IP 级限流) |
| 东财正文接口 300 条连发 | **209/300 失败**;连接复用 60/60 `Connection reset` |
| 申万 官网 频繁请求 | HTTP **508** |

现状只有 **per-source `minGap=2s`**(`internal/collector/http.go`)—— 它约束的是
「**同一源相邻请求**」,对「**同一 host 的突发**」(分页 + 退避重试叠加)无约束。提频后
每 host 的请求密度上升,必须补 **per-host** 约束。

## 4. S3 护栏实现(`internal/collector/limiter.go`)

三条护栏**都是 per-host、进程内共享**(同一 host 的多个驱动/分页共用一份):

| 护栏 | 触发 | 动作 | 参数 |
|---|---|---|---|
| **令牌桶** | 平均速率 + 突发 | 请求前 `wait(ctx)` 阻塞到有令牌 | 补充 **2 req/s**、突发 **3** |
| **空响应哨兵** | **连续 3 次**「成功但 0 条」 | 该 host 有效速率 **×1/2**(降速一档),封顶 **×1/8**;出现非空即复位 | `emptyStreakToPenalize=3`、`penaltyCap=8` |
| **熔断** | **连续 4 次**失败 | 开路 **60s**,期间**直接返回错误、不发网络 I/O**;cd 后半开探一次,成功闭合、失败续开 | `failStreakToOpen=4`、`breakerCooldown=60s` |

**信号来源**:驱动层喂入,因为 `httpSource` 看不到条目数 —— 各 `Fetch` 出口调
`observeFetch(<endpoint>, len(items))`,**成功但 0 条**即记一次空响应(静默限流的信号)。

**确定性**:`reserve` 是**纯决策**(给定 `now` → 需等待多久),睡眠留给调用方 → 单测可
注入假时钟、**零真睡**(`limiter_test.go` 用 `httptest.Server` + 原子计数,断言熔断开路期间
**一个请求都不发出**)。

**`dongcai` 顺带归队**:该驱动此前**自建 `http.Client` 绕过 `httpSource`**,故拿不到限频/
退避/护栏。本次改为走 `httpSource`,与其他 5 源自证同一套约束。

> ⚠️ **状态在内存、不落盘** —— 故哨兵与熔断要**跨轮生效,采集必须常驻进程**(见 §5)。

## 5. 提频实现:常驻采集循环(而非 cron 每 3 分钟)

**关键事实**:`tools` 容器**无可写卷**(只挂 `./certs:ro`)→ 跨轮状态无处落盘。
3 条护栏里**两条(哨兵、熔断)靠跨轮累积**(连续 N 次才触发);若每 3 分钟 `cron` 拉起
**一次性**进程,每轮从零开始、状态永远归零 → **两条护栏形同虚设**,且一天产生 ~4000 个容器。

→ 方案:**常驻循环**(仿 `cmd/research-worker`:常驻 + `signal.NotifyContext` + 优雅退出)。

- `cmd/collector` 新增 `-interval`(默认 `0` = 保持既有一次性行为,日管线不变)与
  `-session`(交易时段闸,默认 `09:15-15:05` 北京时间,工作日)。
- `-interval>0` 时进入 ticker 循环,**每 tick 跑全部 sourceSpecs**,护栏状态**留内存跨 tick 生效**;
  启动即采一轮(不等首个 tick);时段外/周末**静默跳过、不产生请求**。
- **不落「今日已跑」stamp** —— 那是日管线的**单一日锁**,盘中轮询本不该被它阻断
  (该锁正是现状「盘中不跑」的根因)。
- **新常驻服务 `collector`**:`configs/docker-compose.prod.yml`,**复用 `piks-tools:latest` 镜像**
  (故仍是**四镜像**),`command: ["./bin/collector","-driver","news","-interval","3m"]`,
  `restart: unless-stopped`;`deploy.sh` rollout 段纳入(`up -d web research collector`)。
- **只跑快讯源**(`-driver news`,6 家):公告(巨潮)单日 ~1200 条/40 页翻页重,留日管线
  16:10 采一次,**不进盘中高频循环**(否则每 3 分钟重复翻 40 页)。

**日管线保留不变**:`pipeline.sh` 的 `collector -driver all`(收盘后补齐)+
`collector -driver cninfo-announce`(公告)**不动**。二者幂等 + 迁移 `0016` 去重,重复无害。

### 5.1 连带必改(否则提频会积压/误停源)

| 改动 | 原状 | 问题 | 现方案 |
|---|---|---|---|
| `worker -limit` | **300**(按 ~180 条/日 定) | 盘中 3 分钟 × ~5.5h ≈ 110 轮,日入库量翻倍 → raw 积压、事件滞后 | 抬高到 **800**;`ai_daily_token_budget` 仍是唯一 token 护栏 |
| 3 连败自动 `PauseSource` | 单轮内 3 连败即暂停该源 | 3 分钟轮询下**3 分钟就停一个源**;且 `PauseSource` 后**无人读取**(collector 从不看 `status`)→ 暂停后照旧轮询,**坏到两头** | **去掉**该自动路径,瞬时故障改由 **per-host 熔断**承担(时间盒 + 自愈);**让 collector 真正尊重 `sources.status='paused'`**(跳过 + 记 `task_runs` status=`skipped` + 打日志) |

> **与 #45 P5 的口径协同**:#45 P5 已定稿三档(early 09:15 / late 18:30 / realtime 盘中每 10min)。
> 本 PR 取 **3 分钟**,**一次定死**并在此登记 —— 不再让两套调度并存;P5 的 early/late 两档
> (窗口 + 落库策略)不在本 PR 范围。

**不做**:交易日历(`DOW>=6` + 时段闸兜底,与 #45 P5「交易日历暂不做」一致)。**非交易日会空转**
(工作日闸放行但无新数据),如实登记。

## 6. 测试

- **`internal/collector/limiter_test.go`**(repo **首次**用 `httptest.Server`):令牌桶节流 /
  哨兵降速与复位 / 熔断开路期间**零请求** + cd 后半开/闭合;全部注入假时钟,**零真睡**。
- **`cmd/collector/main_test.go`**:`inSession`(周一 09:14/09:15/15:05/15:06 + 周六)、
  `parseSession` 非法输入、`resolveSpecs` 的 `news`(=6,不含公告)vs `all`(=7)。
- 既有 `go build/vet/test` 全绿。

## 7. 文件落点

| 文件 | 改动 |
|---|---|
| `internal/collector/limiter.go` | **新增**:per-host 三护栏 + 进程内注册表 |
| `internal/collector/limiter_test.go` | **新增**:护栏确定性单测 |
| `internal/collector/http.go` | `doJSON` 请求前后接入 `limiterFor` 的 `wait`/`record` |
| `internal/collector/{sina,jin10,cls,ths,futu,dongcai,cninfo_announce}.go` | `Fetch` 出口 `observeFetch(endpoint, len(out))` |
| `internal/collector/dongcai.go` | 自建 client → 改走 `httpSource`(归队护栏) |
| `cmd/collector/main.go` | `-interval`/`-session`/`-driver news`;常驻循环;尊重 `paused`;去掉自动暂停 |
| `cmd/collector/main_test.go` | **新增**:时段闸/解析/源分组单测 |
| `configs/docker-compose.prod.yml` | **新增 `collector` 服务**(复用 tools 镜像) |
| `scripts/deploy.sh` | rollout 纳入 `collector` |
| `scripts/pipeline.sh` | `worker -limit 300` → **800** |
