# 迭代 1 — 可靠性 归档

> 状态:**已完成并冻结**。本阶段只读,续接请看 `进度总表.md` 与 `../design/iter1-reliability.md`。
> 日期:2026-08-26。实现以 `design/iter1-reliability.md` 定稿契约为准(D8~D11)。

## 1. 目标与交付物

把迭代 0 的「能跑」加固为「可信」:同一真实事件的多条报道只发布一张卡片、异常可对账、失败可重试、更新不产生 git 噪音、真实数据源接入。

| 交付物 | 状态 |
|---|---|
| `0002_reliability.sql`:event_clusters 表 + events 加 cluster_id/published_at | ✅ |
| 事件语义去重聚类 `cmd/cluster`(规则直合 + 便宜档 LLM 批量确认) | ✅ |
| raw↔event 对账 `cmd/reconcile`(6 检查 + vault 报告) | ✅ |
| worker 重试 `-retry` + 任务幂等 | ✅ |
| 增量发布 publisher 改造(内容 hash 跳过、merged 删卡、补证重写) | ✅ |
| G1 东财快讯:探针 `cmd/probe/dongcai` + 真实驱动 `internal/collector/dongcai.go` | ✅ |
| 端到端验收(设计 §5 六项) | ✅ |

## 2. 验收结果(设计 §5)

| # | 标准 | 结果 |
|---|---|---|
| §5.1 | 迁移后 events 有 cluster_id/published_at,event_clusters 建表 | ✅ `information_schema` 两列 + `to_regclass('event_clusters')` 真 |
| §5.2 | 同事件不同措辞 2 条 → cluster 1 簇,仅 canonical 卡片发布,重复卡片删除 | ✅ 4 事件→2 簇(央行/星河)、2 merged;发布后仅 2 张卡片,merged 无卡 |
| §5.3 | 制造 failed raw + 孤儿 event → reconcile 如实列出,meta 有异常计数 | ✅ 3 异常(failed_raw / orphan_event / missing_evidence),meta `{"total":3,"by_category":{...},"conclusion":{"passed":false}}` |
| §5.4 | worker -retry 重跑 failed → 成功,不产生重复事件 | ✅ fixture-dup-001 恢复,其事件恰好 1 条 |
| §5.5 | 补证已发布事件(updated_at 变化)→ publisher 重写该卡片;内容无变化时 git 零提交 | ✅ 补证 `updated=1 commits=1`;touch updated_at 后 `unchanged=1 commits=0` |
| §5.6 | 迭代 0 幂等回归:去重采集 / 抽取 / 发布 / 聚类 | ✅ 采集 2 连 `dup`;worker `processed=0`;publisher `commits=0`;cluster 已聚类不再出新簇 |

**闭环补测**:检测→修复→归零。修复孤儿(FK 恢复)、补证据后重跑 reconcile → `异常=0 结论=通过`。

## 3. 关键实现决策(落地细节)

- **去重分层(§3.1)**:高置信纯规则直合(归一化标题全同+同类型)→ 中等置信 LLM 批量确认(`deepseek-chat`,N 对/条,复用日 token 预算)。`normalizeTitle` 保留汉字+字母数字去标点;Jaccard≥0.7 或(实体交集且 occurred_at≤3d)。
- **canonical 选择**:簇内最早创建(同则高置信);canonical 保留原状态。**聚类 canonical 用 `SetEventClusterNoTouch`(只设 cluster_id,不动 updated_at)** —— 否则已发布 canonical 被触碰即重选、无谓重写+git 噪音。
- **增量发布(§3.4)**:发布候选 = 未发布 ∪ 已发布但 `updated_at>published_at`;**status 恒表知识状态(extracted/verified/merged),发布生命周期由 published_at 承载**(不发 status='published',front matter 稳定 → 内容无变化时渲染逐字节相同 → hash 跳过零提交)。渲染内容 md5 比对,一致则跳过写盘。
- **补证触发**:`CreateEvidence` 插入后 bump 事件 `updated_at`;证据节渲染**全部**证据(不只第一条),补证才能看到变化并触发重写。
- **merged 删卡**:`ListMergedPublished`(merged ∧ published_at 非空)→ 删其旧卡片,git 记录删除。真实场景=发布后才被聚类合并(先发后聚)。
- **对账 6 检查(§3.2)**:stale_raw(>7d)/failed_raw/processed_no_event/orphan_event/missing_evidence/silent_source(24h 无采集)。只读,报告落 `00-System/recon-{date}.md`。
- **孤儿事件不可自然产生**:events.raw_document_id 有 FK 约束,孤儿检查为防御性;验收时临时放开约束制造,验证后恢复。
- **G1 探针(§3.5,数据诚实)**:先抓东财快讯页 JS 反查真实端点,再逐字段核对。**端点** `np-weblist.eastmoney.com/comm/web/getFastNewsZhibo`,参数 `client=web&biz=web_724&sortEnd={游标}&pageSize=N`;DTO=`code/title/showTime/realSort/summary/stockList`;`sortEnd` 时间游标分页;详情页 URL 规律 `finance.eastmoney.com/a/{code}.html`(实测 200)。showTime 为北京时间,固定 +08:00 解析存库。

## 4. 成本记录(模型分层延续)

| 环节 | 模型档 | 实测(mock) |
|---|---|---|
| 事件抽取(高频) | `deepseek-chat` | 150 tokens/文档(固定) |
| 去重确认(中等置信候选) | `deepseek-chat` | 批量提示词;本轮 mock 走纯规则(auto_groups=2, llm_pairs=0),真实对由真实 provider 消耗 |
| 深度复盘/周报 | `deepseek-reasoner` | 迭代 4 接入 |

去重确认与抽取复用 `PIKS_AI_DAILY_TOKEN_BUDGET`(同池有上限)。

## 5. 遗留缺口(续接)

| 缺口 | 说明 |
|---|---|
| **真实 AI provider 验证** | 用户提供 opencode API(base URL+key)后,去 mock 跑一次全链,抽查 facts 质量、去重 LLM 路径(真实 llm_pairs)、token 真实用量 |
| **后到重复** | 已聚类 canonical 不在 cluster 候选池;晚到的重复报道不会与新 canonical 合并(设计已知,后续迭代可放宽候选池) |
| **Obsidian 本地确认** | vault 需用户机器打开确认渲染 |
| **G1 稳定生产化** | 东财接口无 SLA,真实驱动已就位;源健康监控(3 连败 pause)兜底 |

## 6. 运行手册(迭代 1 新增命令)

```bash
export PIKS_DATABASE_URL="postgres://piks:piks_dev_password@localhost:5433/piks?sslmode=disable"

# 聚类去重(生产用真实 provider;mock 验证管道)
PIKS_AI_PROVIDER=mock go run ./cmd/cluster -limit 100 -batch 20

# 对账(只读,报告落 vault 00-System/)
go run ./cmd/reconcile

# 重试失败文档(worker 追加 -retry)
PIKS_AI_PROVIDER=mock go run ./cmd/worker -limit 50 -retry

# 东财快讯真实采集(一次一页≈50 条;调 maxPages 可拉更多)
go run ./cmd/collector -driver dongcai -source news-flash

# 探针(抓真实响应核对字段)
go run ./cmd/probe/dongcai -pages 2 -pageSize 5
```

## 7. 下一步(迭代 2:市场情报)

行情/涨停数据(G2)、observations 落库、市场状态 12 项、情绪模型、每日复盘聚合页。设计待开;真实 AI provider 先行验证迭代 1 管道。
