# 迭代 0 — 最小闭环 设计文档

| 项 | 内容 |
|---|---|
| 状态 | 🔶 草稿(待定稿门) |
| 关联 | `PIKS架构设计文档.md` v1.0 · `docs/项目详解.md` · `docs/进度总表.md` |
| 定稿日期 | 待确认 |

> 本设计遵守三条冻结骨架:`PostgreSQL=Source of Truth / Markdown=Projection / Obsidian=Interface`;`Fact ≠ Inference ≠ Belief`;`AI → Structured Output → Schema Validation → Business Validation → Knowledge Core`。

---

## 1. 目标与范围

### 目标
跑通一条真实可验收的最小闭环:

```
新闻 → 采集 → 去重存档 → AI 结构化抽取(仅 Fact)→ 校验 → Knowledge Core
     → Markdown 发布 → Git → Obsidian 可见(Event 卡片)
```

### 范围
- 7 张 PostgreSQL 表(6 张领域表 + 1 张运维表)
- 3 个 Go 命令:`collector`(采集)/ `worker`(抽取)/ `publisher`(发布)
- OpenAI 兼容 AI 适配器(覆盖 DeepSeek 官方 + 各类 OpenAI 兼容订阅源)+ 模型分层配置
- 简单任务记录与可观测性最小形态

### 边界(明确不做,留给后续迭代)
| 不做 | 归属 |
|---|---|
| 事件语义去重/聚类 | 迭代 1 |
| 行情数据、市场状态 12 项、情绪模型、每日复盘页 | 迭代 2 |
| Company / Industry / Concept / Topic 实体表与抽取 | 迭代 3 |
| Belief / Case / Mistake / 回写 harvest / 周报 | 迭代 4 |
| 任务队列框架、ES、Neo4j、K8s、多 AI Agent | 长期不做 |

---

## 2. 链路与处理阶段

```
Collect → Normalize → Dedup(content_hash) → Extract(Event+Fact)
       → Schema Validation → Business Validation → Persist
       → Publish(Markdown + Git)
```

映射到架构文档 §12 流水线:本迭代裁剪为 **Fact 层只取 Event 抽取**;
- 不做 Entity 抽取建表(迭代 3 补);
- 不生成 Inference(AI 推测不进库,严格 Fact≠Inference;Inference/Belief 由用户在 Obsidian 产生);
- 语义去重(事件聚类)不在本迭代(迭代 1 补)。

---

## 3. 数据模型(7 张表)

> 迁移文件:`migrations/0001_init.sql`。`gen_random_uuid()` 需 `pgcrypto` 扩展。

### 3.0 数据库与 steady 的关系(定稿决策 D7)

**背景**:steady 后续将上实盘(真实资金),要求在线稳定。→ 隔离优先。

- **决策:DB 级完全隔离** —— PIKS 使用**独立 Postgres 实例**(自带 postgres 容器;与 steady 容器同 host 亦可,DB 级零耦合)。不共享实例、不共享表、不互相调用。
- 原因:实盘系统不可被 PIKS 的迁移/重启/维护打扰;同样,PIKS 也不受 steady 的升级影响。负载无关(见下),隔离的是**可用性与运维耦合**(blast radius)。
- **负载评估**:PIKS 日均 DB 操作约几十次插入 + 发布时几十次查询(数据量 <1MB/天);AI 抽取为外部 HTTP 调用,不占本地 DB/CPU。对任何实例的"压力"均可忽略。
- 备份:两实例各自备份,但**备份策略统一规划**(每日晚间窗口执行,与用户"晚上做迭代"的维护窗口一致)。
- 实现配置化:`PIKS_DATABASE_URL` 指向 PIKS 自身实例;即使未来想合并,仅改该连接串即可,代码与迁移不变(但决策保持独立,该项仅作应急手段)。

### 3.1 `sources` — 数据源注册
```sql
CREATE TABLE sources (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL,
  source_type TEXT NOT NULL,              -- news/policy/market/report/announcement/macro/history
  config      JSONB NOT NULL DEFAULT '{}',-- 采集配置(feed url、分页参数等)
  status      TEXT NOT NULL DEFAULT 'active', -- active/paused/dead
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.2 `raw_documents` — 原始文档(去重与血缘锚点)
```sql
CREATE TABLE raw_documents (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id        UUID NOT NULL REFERENCES sources(id),
  external_id      TEXT,                  -- 来源侧主键(如有)
  url              TEXT,
  title            TEXT,
  content          TEXT NOT NULL,
  content_hash     TEXT NOT NULL,         -- sha256(归一化 content), 去重键
  published_at     TIMESTAMPTZ,
  retrieved_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  status           TEXT NOT NULL DEFAULT 'raw', -- raw/processed/failed
  pipeline_version TEXT,                  -- 处理时的抽取 pipeline 版本
  error            TEXT,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (source_id, content_hash)
);
```

### 3.3 `events` — 事件(Fact 层)
```sql
CREATE TABLE events (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  raw_document_id  UUID REFERENCES raw_documents(id),
  title            TEXT NOT NULL,
  event_type       TEXT NOT NULL,   -- policy/earnings/industry/accident/international/tech/macro/company/other
  summary          TEXT,
  facts            JSONB NOT NULL DEFAULT '[]', -- 事实性陈述数组(禁推测词)
  affected         JSONB NOT NULL DEFAULT '[]', -- 受影响实体名(行业/公司/概念),原文为准
  occurred_at      TIMESTAMPTZ,
  confidence       NUMERIC(3,2) NOT NULL DEFAULT 0, -- 0.00~1.00,模型置信度
  status           TEXT NOT NULL DEFAULT 'extracted', -- discovered/processed/extracted/verified/published/archived
  pipeline_version TEXT,
  source_id        UUID REFERENCES sources(id),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  valid_from       TIMESTAMPTZ,
  valid_to         TIMESTAMPTZ
);
CREATE INDEX idx_events_raw_doc     ON events(raw_document_id);
CREATE INDEX idx_events_occurred_at ON events(occurred_at);
```

### 3.4 `evidences` — 证据(Claim→Evidence→Source,V1 形态:事件即主张,raw doc 即证据)
```sql
CREATE TABLE evidences (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id     UUID REFERENCES events(id),
  claim        TEXT NOT NULL,
  source_id    UUID REFERENCES sources(id),
  source_type  TEXT,               -- official/company/exchange/government/news/research/social/ai/user
  url          TEXT,
  title        TEXT,
  content      TEXT,
  published_at TIMESTAMPTZ,
  retrieved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  reliability  TEXT,               -- high/medium/low
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.5 `observations` — 观测(V1 建表预留,迭代 2 开始填充)
```sql
CREATE TABLE observations (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id       UUID REFERENCES events(id),
  market         TEXT NOT NULL,      -- 指数/板块名
  indicator      TEXT NOT NULL,      -- 涨跌幅/成交额/涨停数...
  value          TEXT NOT NULL,
  previous_value TEXT,
  change         TEXT,
  observed_at    TIMESTAMPTZ NOT NULL,
  source         TEXT,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.6 `relationships` — 关系(通用有向边;因果链=模板+视图,非约束)
```sql
CREATE TABLE relationships (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  from_type   TEXT NOT NULL,      -- 多态:event/company/industry/concept/topic/...
  from_id     UUID NOT NULL,
  to_type     TEXT NOT NULL,
  to_id       UUID NOT NULL,
  rel_type    TEXT NOT NULL,      -- causes/affects/supports/contradicts/part_of/belongs_to/derived_from/validated_by/similar_to/related_to
  properties  JSONB NOT NULL DEFAULT '{}', -- 如 {chain_step:"Event→Factor"}
  confidence  NUMERIC(3,2),
  source      TEXT,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  valid_from  TIMESTAMPTZ,
  valid_to    TIMESTAMPTZ,
  UNIQUE (from_type, from_id, to_type, to_id, rel_type)
);
```

> 本迭代关系表的 from/to 均为 event 类型(事件间关系);对"未建模实体"的影响先用 `events.affected` 文本承载(渲染为 Obsidian wikilink),迭代 3 建实体表后再转正式 relationship。

### 3.7 `task_runs` — 运维/可观测性(架构文档 §27/§28 最小形态)
```sql
CREATE TABLE task_runs (
  id         BIGSERIAL PRIMARY KEY,
  command    TEXT NOT NULL,          -- collector/worker/publisher
  status     TEXT NOT NULL DEFAULT 'running', -- running/success/failed
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at   TIMESTAMPTZ,
  error      TEXT,
  meta       JSONB NOT NULL DEFAULT '{}', -- counts(采集数/新建事件数) + ai_tokens
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

## 4. 组件与 Go 项目结构

```
piks/
├── cmd/
│   ├── collector/        # 采集:source 适配器 → 归一化 → 去重 → raw_documents
│   ├── worker/           # 抽取:raw(processed 前)→ LLM → 校验 → events/evidences
│   └── publisher/        # 发布:events → Markdown → vault git 仓库
├── internal/
│   ├── config/           # env 配置(base_url/key/模型分层/预算阈值)
│   ├── model/            # 7 张表的 domain struct
│   ├── store/            # pg 访问(每表一个 repository,参数化 SQL)
│   ├── collector/        # 适配器:file(保底) / dongcai(待验证) / 未来扩展
│   ├── ai/               # Provider 接口 + openai_compat 适配器 + 模型分层路由
│   ├── extract/          # 抽取编排 + JSON Schema 校验 + 业务校验
│   ├── publish/          # Markdown 渲染 + git commit/push
│   └── task/             # task_runs 记录
├── migrations/0001_init.sql
├── prompts/extract.md    # 抽取系统提示词
├── configs/docker-compose.yml
├── sample/sample-news.json   # 验收用样例(标记为测试夹具,非真实数据)
└── docs/
```

### 命令语义
- `collector`:`-driver file|dongcai`,`-input`(file 驱动);产出 raw_documents,写 task_runs;**记录每源成功/失败计数到 `task_runs.meta`,单源连续失败 ≥3 次自动暂停该源(`sources.status='paused'`)**。

### 源健康监控(应对非官方源不稳定)
"快讯"品类(东财/新浪/同花顺)均为网页内部接口,无官方公开 API、无 SLA、随时可能改格式或加反爬。对策:
1. **健康监控**:每源成功/失败计数入 `task_runs.meta`,连续失败自动 pause(见上);
2. **可换可补**:采集走适配器,补源/换源零成本;稳定替代源列入 G 缺口——巨潮公告(官方稳定)、政府/央媒官方 RSS、商业数据源(Tushare 积分/付费);
3. **验收不依赖外部源**:file 驱动保证闭环先跑通。
- `worker`:取 `status='raw'` 的 raw_documents → 逐条抽取 → 校验 → 入 events/evidences → 标记 processed/failed,写 task_runs。
- `publisher`:取 `status in ('extracted','verified')` 且未发布的事件 → 渲染 Markdown → 提交并推送 vault 仓库,写 task_runs。

### 任务与调度
- 不引队列框架。cron 定时跑三个命令(docker 内 cron 或宿主 crontab)。
- 幂等:`content_hash` 唯一约束保证重复采集不重复入库;`UNIQUE(from_type,from_id,to_type,to_id,rel_type)` 保证关系不重复。

---

## 5. AI 设计与成本策略

### 5.1 Provider 接口
```go
type Provider interface {
    Name() string
    StructuredOutput(ctx context.Context, sys, user string, schema json.RawMessage) (json.RawMessage, error)
    HealthCheck(ctx context.Context) error
}
```
> 业务层只依赖该接口,不接触任何厂商 SDK。`Chat()`/`Embedding()` 等接口待真实需求出现再加(接口增量扩展成本低)。

### 5.2 首个适配器:OpenAI-Compatible
覆盖 DeepSeek 官方 API(`https://api.deepseek.com`)、OpenAI、OpenRouter 及任何 OpenAI 兼容订阅源。
- 协议:POST `/chat/completions`,`response_format={"type":"json_object"}` 输出 JSON。
- 配置(env):`PIKS_AI_BASE_URL` / `PIKS_AI_API_KEY` / `PIKS_AI_MODEL_*`。
- 换源只改 base_url + model,不改代码。Claude(Anthropic Messages API)适配器为可选第二实现。

### 5.3 模型分层与路由(省钱核心)
> 原则:**90% 调用(机械抽取)走最便宜档;高智模型只留给每周一两次、真正需要推理深度的节点。**
> 无本地模型:本系统只使用厂商大模型(用户明确无本地 LLM),不配置本地回退。

| 环节 | 频率 | 档位 | 默认模型建议 | 备注 |
|---|---|---|---|---|
| 新闻→事件抽取(JSON) | 高频·每日几十条 | 便宜档 | `deepseek-chat` | 坏输出由校验拦截,不值得用贵的 |
| 抽取失败重试 | 低频 | 便宜档 | 同上 | 重试 ≤2 次 |
| 语义去重/聚类 | 高频·迭代1 | 便宜档→规则优先 | 同上 | 先规则,规则不行再上模型 |
| 每日复盘聚合 | 中频·每日1 | 中档 | 中等模型 | 单次调用 |
| **周报/深度复盘/疑难判断** | **低频·每周1~2** | **高智档** | `deepseek-reasoner` 或 Claude 高智档 | **关键节点,全系统唯一值得花大钱处** |

配置结构(路由按任务键取模型):
```yaml
ai:
  default_provider: openai_compat
  tiers:
    extract:   { model: deepseek-chat }
    reasoning: { model: deepseek-reasoner }
```

### 5.4 成本护栏
1. **去重先于调 LLM**:`content_hash` 命中即跳过,同一内容不二次调用模型(最大的省钱点)。
2. **预算护栏**:`task_runs.meta.ai_tokens` 累计 token;超 `PIKS_AI_DAILY_TOKEN_BUDGET` 阈值 → 当日 worker 暂停(可配置,默认关闭)。
3. 抽取输出仅请求必要字段,`prompt` 明确禁止冗长叙述。

### 5.5 抽取契约(Structured Output)
`prompts/extract.md` 系统提示词 + JSON Schema:
```jsonc
{
  "events": [
    {
      "title":        "string(必填)",
      "event_type":   "policy|earnings|industry|accident|international|tech|macro|company|other",
      "occurred_at":  "ISO8601 或 null",
      "summary":      "string(单句叙述)",
      "facts":        ["string; 只允许事实性陈述,禁止 可能/或许/预计/大概 等推测词"],
      "affected":     ["string; 受影响实体名,以原文为准,禁止编造"],
      "confidence":   0.0~1.0
    }
  ]
}
```
- **Schema 校验**:字段类型/必填/enum/数组长度上限(单文档 events ≤5)。
- **业务校验**:title 非空、event_type 合法、facts 非空、confidence∈[0,1];通过后才允许写库。
- 抽取失败或校验不过 → `raw_documents.status='failed'` + `error` 记录,重试 ≤2 次。
- `pipeline_version` = `extract@<prompt hash>-<model>`;worker 每次处理记录,供血缘与回归。

### 5.6 数据诚实声明
- `facts` 为 AI 抽取产物,发布到 Markdown 时标记为"AI 抽取,需人工复核",**不写入 Inference/Belief 层**。
- 上游(东财快讯)真实接口形状未验证前,采集契约 §6 的"现有接口契约"章节保持待填,不凭想象写字段。

---

## 6. 采集契约

### 6.1 归一化产出(RawNews DTO,采集适配器唯一输出)
```go
type RawNews struct {
    ExternalID  string
    URL         string
    Title       string
    Content     string
    PublishedAt time.Time
}
```
> 归一化在此统一(driver 不同、输出一致),去重(content_hash)在入库层做。

### 6.2 适配器
| 驱动 | 状态 | 说明 |
|---|---|---|
| `file` | ✅ 保底 | 读本地 JSON/txt,迭代 0 验收用样例,保证闭环不依赖外部 API |
| `dongcai` | 🔶 待验证 | 东方财富快讯(网页内部接口,**非官方、无 SLA**);真实 DTO 实现时验证后填 §6.3;不稳定 → 由源健康监控 + 适配器换源兜底(备选:巨潮公告/政府RSS/商业源,G2~G5) |
| 未来 | ⬜ | 政策/行情/公告等(见 `进度总表.md` G2~G5) |

### 6.3 现有接口契约(待验证)
> 实现时核验真实接口形状后填写,遵守"对照真实 DTO 逐字核对,不凭想象"。

---

## 7. Markdown 发布模板(事件卡片)

```
---
id: event-{uuid前8}
type: event
date: {occurred_at 或 created_at}
status: extracted
source: {sources.name}
confidence: {confidence}
pipeline: {pipeline_version}
---

# {title}

## 发生了什么
{summary}

## 事实
- {facts 每条一行}

## 影响
- [[{affected 实体名}]]   ← 未解析链接,迭代3 建实体后自动可跳

## 证据
[{title}]({url})

## AI 分析
> 本卡片的 事实/影响 由 AI 抽取,未经人工复核。推测性判断不在本卡片,请填写到下方"我的理解"。

## 我的理解
> 等待填写(用户在此写 Inference / Belief,遵循 Fact≠Inference≠Belief)
```

> 发布规则:仅"未发布过"的事件生成新文件;已存在则跳过(避免 Git 噪音,迭代 1 补增量更新)。生成的 vault 为独立 git 仓库 `PIKS-Vault/`,与代码仓库分离(Generated 与 Personal 天然隔离)。

---

## 8. 验收标准(smoke test)

1. `docker compose up -d` → Postgres healthy。
2. `go run ./cmd/collector -driver file -input sample/sample-news.json` 执行两次 → `raw_documents` 仅 1 条(去重生效)。
3. `go run ./cmd/worker` → LLM 抽取 → `events` 1 条、`evidences` ≥1 条;`facts` 内无推测词(抽查);`raw_documents.status='processed'`。
4. `go run ./cmd/publisher` → vault 仓库生成 `05-Events/**/event-xxx.md`,含 front matter、事实、wikilink、证据、我的理解占位。
5. 用 Obsidian 打开 vault → Event 卡片可见。
6. `task_runs` 有 3 条 success 记录,含耗时与 ai_tokens。

---

## 9. 定稿拍板点(需用户确认)

| # | 决策 | 推荐 |
|---|---|---|
| D1 | 首个 AI 适配器 | OpenAI 兼容(覆盖 DeepSeek 官方 + 订阅源),Claude 可选项 |
| D2 | 模型分层 | 便宜档抽取(deepseek-chat)+ 高智档周报(deepseek-reasoner);表 5.3 |
| D3 | 表数量 | 6 领域表 + task_runs(超出之前 5 张,原因:血缘/去重/可观测性,§3) |
| D4 | V1 抽取只产 Fact | AI 推测不进库,Inference/Belief 由用户在 Obsidian 产生 |
| D5 | 采集双轨 | file 驱动保底验收,dongcai 适配器待验证,G1 缺口化 |
| D6 | Vault 独立仓库 | 代码仓库与 vault 仓库分离,Generated/Personal 隔离 |
| D7 | 数据库与 steady | steady 将上实盘 → **独立 Postgres 实例**(DB 级零耦合,同 host 双容器亦可);备份策略统一走晚间窗口;DSN 配置化(§3.0) |

> 确认后冻结本设计(`design/README.md` 登记),开始按契约实现。
