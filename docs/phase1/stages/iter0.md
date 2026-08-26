# 迭代 0 — 最小闭环 归档

> 状态:**已完成并冻结**。本阶段只读,续接请看 `进度总表.md` 与 `../design/iter0-min-loop.md`。
> 日期:2026-08-26。实现以 `design/iter0-min-loop.md` 定稿契约为准。

## 1. 目标与交付物

跑通「新闻 → Event 抽取 → Markdown → Git → Obsidian」最薄闭环,全部落在冻结架构上(PostgreSQL=Source of Truth / Fact≠Inference≠Belief / AI→Structured Output→Validation)。

| 交付物 | 状态 |
|---|---|
| Docker Compose 独立 Postgres(D7:与 steady 完全隔离) | ✅ |
| 7 张表 migrations(幂等迁移器) | ✅ |
| Go 骨架 + store 仓库层(pgx,RowToStructByName) | ✅ |
| 采集器(file 驱动保底 + 东财 stub)content_hash 去重 + 源健康监控 | ✅ |
| AI 结构化抽取管道(OpenAI 兼容 + mock,Schema/业务校验,预算护栏) | ✅ |
| Markdown 发布器 + 独立 vault git 仓库(设计 §7 模板) | ✅ |
| 端到端 smoke test(设计 §8) | ✅ |

## 2. 验收结果(设计 §8)

| # | 标准 | 结果 |
|---|---|---|
| §8.1 | `docker compose up -d` → Postgres healthy | ✅ `Up (healthy)` |
| §8.2 | collector 跑两次,去重生效 | ✅ 第 1 次 `new=2 dup=0`;第 2 次 `new=0 dup=2`;`raw_documents=2` |
| §8.3 | worker 抽取 → events/evidences ≥1;facts 无推测词;raw→processed | ✅ `events=2 evidences=2`;raw `processed`;facts 推测词抽查=0 |
| §8.4 | publisher → vault 生成 `05-Events/**/event-*.md`,含 front matter/事实/wikilink/证据/我的理解占位 | ✅ `05-Events/policy/`、`05-Events/tech/` 各 1 卡 |
| §8.5 | Obsidian 打开 vault → 卡片可见 | 🔶 结构已 Obsidian-ready(wikilink/front matter),需用户本地打开确认 |
| §8.6 | task_runs 有 3 条 success,含耗时与 ai_tokens | ✅ 4 条(2 collector + 1 worker + 1 publisher),均含 started/ended_at + meta |

补充验证:发布器幂等(重跑 `published=0`、vault git 无新提交);预算护栏(日预算 < 单条 token 时拦截、> 时放行);store 集成冒烟 `TestSmoke` 通过。

## 3. 关键实现决策(落地细节)

- **去重**:`ON CONFLICT (source_id, content_hash) DO NOTHING` + `RowsAffected()==1` 判真插入(踩坑:DO NOTHING 冲突无错误码,曾把 dup 当 new)。
- **Source ID 回填**:`CreateSource` 必须 `RETURNING id`,否则 raw_documents 外键空 → 全失败。
- **AI 血缘**:`pipeline_version = "extract@"+提示词sha[:8]+"-"+模型`,落 raw_documents 与 events。
- **预算护栏**:`PIKS_AI_DAILY_TOKEN_BUDGET`(0=关);worker 内累计 + `task_runs.meta->>'ai_tokens'` 日累计,超阈值提前停。
- **发布幂等**:只取 `status IN (extracted,verified)`;文件已存在即视为已发布(置 published),避免重复写与 git 噪音。
- **Vault 独立仓库**:`./PIKS-Vault`,与代码仓库分离;代码仓库 .gitignore 忽略之。推送可选 `PIKS_VAULT_REMOTE`。
- **mock 与事实层**:mock fixture 刻意去除 `预计/计划/将/或` 等推测词,作为提示词规则的可抽查样例。

## 4. 成本记录(模型分层落地)

| 环节 | 模型档 | 实测(mock) |
|---|---|---|
| 抽取(高频) | `deepseek-chat`(便宜档) | 2 文档 / 300 tokens(150/篇,mock 固定值) |
| 深度复盘(低频) | `deepseek-reasoner`(高智档) | 迭代 4 接入,未在本迭代 |

真实 API 需用户提供 `PIKS_AI_API_KEY`(DeepSeek 官方或 OpenAI 兼容订阅源,改 `PIKS_AI_BASE_URL` 即可切换)。

## 5. 遗留缺口(续接迭代 1)

| 缺口 | 说明 |
|---|---|
| **G1 东财快讯** | `dongcai` 驱动为 stub,真实 DTO/稳定性待验证;源健康监控已就位(3 连败自动 pause) |
| **真实 AI 验证** | mock 已验证管道;真实 provider 需 API key 后跑一次,抽查 facts 质量与 token 真实用量 |
| **Obsidian 本地确认** | vault 需在用户机器上打开,确认渲染与双链 |

## 6. 运行手册

```bash
# 1. 起库
docker compose up -d
# 2. 迁移
go run ./cmd/migrate
# 3. 采集(file 驱动;换真实源用 -driver dongcai)
go run ./cmd/collector -driver file -input sample/sample-news.json -source news-flash
# 4. 抽取(mock 验证管道;生产去掉 PIKS_AI_PROVIDER=mock 并设 PIKS_AI_API_KEY)
PIKS_AI_PROVIDER=mock go run ./cmd/worker -limit 50
# 5. 发布到 vault
go run ./cmd/publisher
```

环境变量见 `internal/config/config.go`(DSN 默认指向 compose 的 5433 独立实例)。

## 7. 下一步(迭代 1:可靠性)

Event 语义去重/聚类、raw↔event 对账、任务重试幂等、增量发布(事件更新不重建文件)、真实数据源接入。设计待开。
