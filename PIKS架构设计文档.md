# PIKS — Personal Investment Knowledge System 架构设计文档

| 项目 | 内容 |
|---|---|
| **Version** | 1.0 |
| **Status** | Architecture Design |
| **Target Lifecycle** | 2–3 年+ |
| **Primary Market** | A 股 |
| **Primary User** | 个人投资者 |
| **Knowledge UI** | Obsidian |
| **Server** | 独立服务器 |
| **Core Database** | PostgreSQL |
| **Backend** | Go |
| **Sync** | Git |
| **AI** | Provider-independent |

---

> ### 现状偏差(2026-08-29 更新)
> 本文档为 v1.0 权威蓝图(已定稿,冻结不改),以下为落地演进与现状,细节见 `docs/项目详解.md` 与 `docs/进度总表.md`:
> - **知识界面**:Obsidian → **Web 平台(PG 直渲)**。迭代 5 建成 `cmd/web`(看板/事件/实体/图谱/复盘/对账/笔记/周报/交易/AI 对话);5-2 下线 vault/GitHub 界面层,`PIKS-Vault/` 仅存档。设计见 `docs/phase2/design/web-app.md`。
> - **大模型配置**:环境变量 → **`app_config` 库表**(权威源,`/settings` 页面可编辑,密钥只存库)。`Markdown = Knowledge Projection` 一章所指投影层已随 vault 下线。
> - **新增个人交易闭环**(2026-08-28,dev-only 未部署 lab):每日自交易截图识别录入 + AI 带引用解读 + 持仓 AI 诊断 + 交易/持仓进周报。设计见 `docs/phase2/design/trades.md`、`docs/phase2/design/trade-loop.md`。
> - **周报 AI 综述**(2026-08-28,dev-only):Web 手动触发 + `weekly_summaries` 按 ISO 周缓存(GET 零 LLM)。设计见 `docs/phase2/design/weekly-ai-summary.md`。
> - 数据层 schema 已演进至 11 个前向迁移(`migrations/0001~0011`,含 app_config/personal_notes/chat_sessions/weekly_summaries/trades/positions/position_reviews)。

---

## 目录

1. [项目背景](#1-项目背景)
2. [项目目标](#2-项目目标)
3. [核心设计原则](#3-核心设计原则)
4. [总体架构](#4-总体架构)
5. [系统分层](#5-系统分层)
6. [数据源（Data Source）](#6-数据源data-source)
7. [数据采集（Data Ingestion）](#7-数据采集data-ingestion)
8. [知识核心（Knowledge Core）](#8-知识核心knowledge-core)
9. [核心对象模型](#9-核心对象模型)
   - 9.1 [Entity（实体）](#91-entity实体)
   - 9.2 [Concept（概念）](#92-concept概念)
   - 9.3 [Company（公司）](#93-company公司)
   - 9.4 [Industry（行业）](#94-industry行业)
   - 9.5 [Event（事件）](#95-event事件)
   - 9.6 [Observation（观测）](#96-observation观测)
   - 9.7 [Market（市场状态）](#97-market市场状态)
   - 9.8 [Emotion（情绪）](#98-emotion情绪)
   - 9.9 [Topic（热点/题材）](#99-topictopic热点题材)
   - 9.10 [Evidence（证据）](#910-evidence证据)
   - 9.11 [Inference（推理）](#911-inference推理)
   - 9.12 [Belief（个人认知）](#912-belief个人认知)
   - 9.13 [Case（历史案例）](#913-case历史案例)
   - 9.14 [Relationship（关系）](#914-relationship关系)
10. [因果链与三层知识模型](#10-因果链与三层知识模型)
    - 10.1 [核心因果链](#101-核心因果链)
    - 10.2 [Fact / Inference / Belief 三层模型](#102-fact--inference--belief-三层模型)
11. [AI 架构](#11-ai-架构)
    - 11.1 [AI 处理流程](#111-ai-处理流程)
    - 11.2 [AI Provider 抽象](#112-ai-provider-抽象)
12. [知识处理流水线](#12-知识处理流水线)
13. [数据去重](#13-数据去重)
14. [PostgreSQL 数据模型](#14-postgresql-数据模型)
15. [Knowledge DB 与 Obsidian 的关系](#15-knowledge-db-与-obsidian-的关系)
16. [Obsidian Vault 目录结构](#16-obsidian-vault-目录结构)
17. [Server / Local 边界](#17-server--local-边界)
18. [Generated / Personal 隔离](#18-generated--personal-隔离)
19. [Git 同步模型](#19-git-同步模型)
20. [Knowledge Publishing（知识发布）](#20-knowledge-publishing知识发布)
21. [个人认知流程](#21-个人认知流程)
22. [分析框架](#22-分析框架)
    - 22.1 [市场分析框架（Market Daily）](#221-市场分析框架market-daily)
    - 22.2 [事件分析框架](#222-事件分析框架)
    - 22.3 [股票分析框架](#223-股票分析框架)
    - 22.4 [热点分析框架](#224-热点分析框架)
    - 22.5 [情绪模型](#225-情绪模型)
    - 22.6 [学习系统](#226-学习系统)
    - 22.7 [错误系统](#227-错误系统)
23. [知识生命周期](#23-知识生命周期)
24. [Confidence（置信度）](#24-confidence置信度)
25. [数据版本](#25-数据版本)
26. [Backend 架构与项目结构](#26-backend-架构与项目结构)
27. [任务架构](#27-任务架构)
28. [可观测性](#28-可观测性)
29. [安全（Security）](#29-安全security)
30. [备份（Backup）](#30-备份backup)
31. [技术选型总结](#31-技术选型总结)
32. [架构可持续性说明](#32-架构可持续性说明)
33. [V1 范围与阶段规划](#33-v1-范围与阶段规划)
34. [架构冻结项（Architecture Freeze）](#34-架构冻结项architecture-freeze)

---

## 1. 项目背景

### 1.1 当前问题

个人投资者普遍存在以下典型问题：

1. 金融知识基础薄弱
2. 无法解释股票上涨/下跌原因
3. 无法理解新闻、政策、宏观事件对市场的影响
4. 无法识别行业热点和市场主线
5. 无法判断市场整体情绪
6. 买卖行为容易受到评论区、低价、大跌等因素影响
7. 缺少稳定的分析框架
8. 缺少系统性的复盘机制
9. 学到的知识容易碎片化

当前交易决策模式可抽象为：

```
信息 → 直觉 → 买卖 → 结果
```

该模式缺失了中间的认知加工环节：

```
事实 → 概念 → 关系 → 因果 → 推理 → 判断 → 验证
```

### 1.2 设计动机

本系统旨在补齐"信息→直觉"之间的结构化认知链路，使投资者逐步形成可解释、可复盘、可迭代的市场理解能力。

---

## 2. 项目目标

### 2.1 核心目标

建立一个长期运行的个人投资认知系统，使用户逐步形成：

> **看懂市场 → 理解市场 → 分析市场 → 形成判断 → 验证判断 → 修正认知**

### 2.2 非目标（Non-Goals）

第一阶段（V1）明确不做：

- 自动交易
- 股票预测
- 自动买卖信号
- 自动荐股
- 高频交易
- 自动管理真实资金
- 与 `steady` 系统耦合

**定位声明：**

> PIKS 是 **Knowledge System**，而不是 **Trading System**。
> 未来是否与交易系统结合，另行决定。

---

## 3. 核心设计原则

### 3.1 Knowledge First（知识优先）

系统首先解决：

> "我为什么这样理解市场？"

而非：

> "今天买什么？"

### 3.2 Fact 与 Opinion 分离

系统必须严格区分事实、推理与个人认知三层：

```
Fact（事实） → Inference（推理） → Belief（个人认知）
```

**禁止**将 AI 推测直接写成事实。

### 3.3 Evidence First（证据优先）

任何重要判断都应尽量具备可追溯的证据链：

```
Claim（主张） → Evidence（证据） → Source（来源）
```

每条证据至少包含：`Source / Evidence / Claim / Confidence` 四个要素。

### 3.4 Time-aware（时间感知）

金融知识具有强时间属性。例如：

- 2026-01：市场认为某行业景气度上升
- 2026-06：行业进入过热
- 2026-09：行业开始下降

不能用今天的知识覆盖过去的认知。核心实体须支持时间字段：

```
created_at / updated_at / valid_from / valid_to
```

### 3.5 AI 可替换

核心架构不得绑定某一家模型厂商。统一经由 LLM Adapter 输出结构化结果：

```
AI Provider → LLM Adapter → Structured Output
```

未来可在不修改 Knowledge Core 的前提下替换 Provider A / B / C 或本地模型。

---

## 4. 总体架构

```
                           External World
                                │
              ┌─────────────────┼─────────────────┐
              ↓                 ↓                 ↓
            News              Market            Reports
            Policy            Data              Filings
              │                 │                 │
              └─────────────────┼─────────────────┘
                                ↓
                     ┌───────────────────┐
                     │   Data Ingestion  │
                     │  Fetch/Parse/     │
                     │  Normalize/Dedup/ │
                     │  Archive          │
                     └─────────┬─────────┘
                               ↓
                     ┌───────────────────┐
                     │   Knowledge Core   │
                     │ Entity/Event/     │
                     │ Observation/      │
                     │ Concept/Industry/ │
                     │ Company/Topic/    │
                     │ Emotion/Case      │
                     └─────────┬─────────┘
                               ↓
                     ┌───────────────────┐
                     │  Knowledge Graph  │
                     │ Relationship/     │
                     │ Causality/        │
                     │ Evidence/         │
                     │ Confidence        │
                     └─────────┬─────────┘
                               ↓
                     ┌───────────────────┐
                     │  Reasoning Engine │
                     │ Extract/Classify/ │
                     │ Explain/Link/     │
                     │ Summarize         │
                     └─────────┬─────────┘
                               ↓
                     ┌───────────────────┐
                     │ Knowledge Publish │
                     │ Markdown/YAML/    │
                     │ Obsidian          │
                     └─────────┬─────────┘
                               ↓
                              Git
                               ↓
                     ┌───────────────────┐
                     │    Local PC       │
                     │     Obsidian      │
                     │ Read/Learn/Review│
                     │     /Edit        │
                     └───────────────────┘
```

---

## 5. 系统分层

系统划分为 6 层：

| 层级 | 名称 | 职责 |
|---|---|---|
| Layer 1 | Data Source | 外部数据源接入 |
| Layer 2 | Data Ingestion | 采集、解析、标准化、去重、存档 |
| Layer 3 | Knowledge Core | 实体/事件/关系等核心对象管理 |
| Layer 4 | Reasoning / AI | 抽取、分类、解释、链接、摘要 |
| Layer 5 | Knowledge Publishing | 生成 Markdown/YAML，发布到 Obsidian |
| Layer 6 | Obsidian | 本地知识阅读、学习与个人认知界面 |

---

## 6. 数据源（Data Source）

**V1 接入数据源：**

- News（新闻）
- Policy（政策）
- Market Data（市场数据）
- Company Reports（公司报告）
- Industry Reports（行业报告）
- Macro Data（宏观数据）
- Public Announcements（公开公告）
- Historical Data（历史数据）

**未来可扩展数据源：**

- Social Media、Research Reports、Forums、RSS、User Input

---

## 7. 数据采集（Data Ingestion）

采集流程：

```
采集 → 解析 → 标准化 → 去重 → 存档
```

**关键约束：** 数据采集模块**不得直接修改**知识实体，必须经过：

```
Raw Data → Processing → Knowledge Extraction
```

---

## 8. 知识核心（Knowledge Core）

Knowledge Core 是系统最核心的模块。V1 定义以下核心对象：

```
Entity / Concept / Company / Industry / Event / Observation /
Market / Topic / Emotion / Evidence / Inference / Belief /
Case / Relationship
```

---

## 9. 核心对象模型

### 9.1 Entity（实体）

Entity 是系统统一实体基础，其他实体类型均继承自 Entity。

```
Entity
├── Concept
├── Company
├── Industry
├── Topic
├── Indicator
├── Market
└── Organization
```

**核心字段：**

| 字段 | 说明 |
|---|---|
| id | 实体唯一标识 |
| type | 实体类型 |
| name | 名称 |
| aliases | 别名列表 |
| description | 描述 |
| status | 状态 |
| created_at / updated_at | 时间戳 |

### 9.2 Concept（概念）

用于描述金融知识概念，如：PE、PB、利率、通胀、供需、现金流、市场预期、风险溢价、行业周期等。

**Concept 模板：**

```
id / type: concept
name            - 概念名称
definition      - 定义
core_question   - 核心问题
explanation     - 解释
examples        - 示例
related_concepts- 相关概念
evidence        - 证据
status          - 状态
```

每个 Concept 必须能回答四个问题：

> 它是什么？／为什么重要？／如何观察？／与什么有关？

### 9.3 Company（公司）

描述上市公司，核心字段：

```
name / ticker / exchange / industry / business_model /
products / revenue_sources / profit_drivers /
competitive_advantage / risks / valuation /
related_events / related_topics / evidence
```

> 第一阶段不要求自动填齐所有字段，允许值为 `Unknown`。

### 9.4 Industry（行业）

用于理解行业，如半导体、机器人、银行、房地产、新能源、AI 等。

**核心字段：** `name / parent_industry / industry_chain / upstream / midstream / downstream / supply / demand / competition / cycle / growth / key_drivers / key_risks / companies / events / topics`

**核心问题：**

> 行业为什么上涨？／行业为什么下跌？／行业当前处于什么阶段？

### 9.5 Event（事件）

Event 是系统中非常重要的对象，如降息、政策发布、公司财报、行业事故、国际事件、重大技术突破等。

```
id / type: event
title / event_type / occurred_at / source / summary /
facts / affected_entities / potential_impacts /
market_reaction / expectation_change / evidence / inference / confidence
```

### 9.6 Observation（观测）

Observation 与 Event 必须严格区分：

| 维度 | Event | Observation |
|---|---|---|
| 含义 | 发生了什么？ | 市场实际发生了什么？ |
| 示例 | 某政策发布 | 机器人指数 +5.2%；相关股票涨停 15 家；成交额增加 38% |

```
id / type: observation
timestamp / market / indicator / value /
previous_value / change / source
```

### 9.7 Market（市场状态）

描述市场整体状态：

```
date / index / breadth / volume / turnover / limit_up / limit_down /
max_board / market_style / risk_appetite / sentiment / trend / hot_topics
```

### 9.8 Emotion（情绪）

情绪**必须有数据支持**，禁止仅写"今天市场情绪不好"。

```
date / breadth / limit_up / limit_down / broken_limit_up / max_board /
yesterday_limit_up_return / hot_topic_continuity / risk_appetite /
profit_effect / state / evidence
```

**情绪状态枚举：** `Extreme_Fear / Fear / Weak / Neutral / Warm / Strong / Extreme_Greed`

> Emotion 是市场状态描述，**不是交易信号**。

### 9.9 Topic（热点/题材）

描述市场热点/题材，如 AI、机器人、低空经济等。

```
name / start_time / current_stage / drivers / related_events /
industries / companies / capital / emotion / continuity / risk / historical_cases
```

### 9.10 Evidence（证据）

Evidence 是整个系统可信度的基础。

```
source_id / source_type / title / url / published_at / author /
content / related_claim / reliability / retrieved_at
```

**来源类型（source_type）：** `Official / Company / Exchange / Government / News / Research / Social / AI / User`

### 9.11 Inference（推理）

Inference 表示"基于事实得到的推理"。

示例：`Fact: 央行降息 → Inference: 企业融资成本可能下降`

```
statement / premises / reasoning / conclusion / confidence /
created_by / evidence / status
```

### 9.12 Belief（个人认知）

Belief 是**用户自己的认知**，是系统最终最重要的资产之一。

示例：`"低价股并不代表便宜。"`

```
statement / reason / supporting_cases / contradicting_cases /
confidence / created_at / updated_at / status
```

**状态枚举：** `Hypothesis / Active / Confirmed / Questioned / Rejected`

### 9.13 Case（历史案例）

Case 用于把"知识"与"现实"连接起来。

```
title / date / market_context / event / industry / companies /
initial_hypothesis / market_reaction / actual_result / lesson /
related_concepts / related_beliefs
```

### 9.14 Relationship（关系）

所有对象之间通过 Relationship 连接。核心关系类型：

```
related_to / affects / affected_by / belongs_to / contains /
causes / caused_by / supports / contradicts / derived_from /
validated_by / similar_to / part_of
```

示例：

```
降息
  ├── affects → 地产
  ├── affects → 银行
  └── affects → 估值
地产
  └── contains → 某公司
```

---

## 10. 因果链与三层知识模型

### 10.1 核心因果链

系统重点支持如下因果传导链（PIKS 核心认知框架）：

```
Event → Factor → Industry → Company → Earnings → Expectation
      → Valuation → Capital → Emotion → Price
```

### 10.2 Fact / Inference / Belief 三层模型

任何知识都必须尽可能归属于三层模型之一：

```
┌──────────────┐
│     Fact     │  事实
└──────┬───────┘
       ↓
┌──────────────┐
│   Inference  │  推理
└──────┬───────┘
       ↓
┌──────────────┐
│    Belief    │  我的认知
└──────────────┘
```

**禁止：** `AI 推测 → 直接写成事实`

---

## 11. AI 架构

### 11.1 AI 处理流程

AI **不直接控制数据库**，标准流程为：

```
Raw Data → LLM → Structured Output → Schema Validation
         → Business Validation → Knowledge Core
```

结构化输出示例：

```json
{
  "entities": [],
  "events": [],
  "claims": [],
  "relationships": [],
  "confidence": 0.82
}
```

### 11.2 AI Provider 抽象

定义统一接口 `AIProvider`：

```
AIProvider
├── Chat()
├── StructuredOutput()
├── Embedding()
└── HealthCheck()
```

具体实现：`OpenAIProvider / ClaudeProvider / LocalProvider / OtherProvider`

> 业务层**不允许**直接调用具体厂商 SDK。

---

## 12. 知识处理流水线

标准知识处理流水线（Knowledge Pipeline）：

```
Collect → Normalize → Deduplicate → Extract Entity → Extract Event
       → Extract Fact → Extract Relationship → Generate Inference
       → Validate → Persist → Publish
```

---

## 13. 数据去重

金融信息重复严重，必须基于以下字段去重：

```
content_hash / source_id / published_at / entity_id
```

避免"同一新闻 → 生成 10 个 Event"的问题。

---

## 14. PostgreSQL 数据模型

建议核心表（与核心对象一一对应）：

```
entities / concepts / companies / industries / events /
observations / markets / topics / emotions / evidences /
inferences / beliefs / cases / relationships /
sources / documents / processing_tasks
```

> **PostgreSQL = Source of Truth（结构化知识唯一可信源）**

---

## 15. Knowledge DB 与 Obsidian 的关系

关键原则：

> **PostgreSQL 是结构化知识 Source of Truth。**
> **Obsidian 是知识阅读、学习和个人认知界面。**

```
PostgreSQL → Markdown Generator → Git → Obsidian
```

---

## 16. Obsidian Vault 目录结构

```
PIKS/
├── 00-System/       系统配置与说明
├── 01-Concepts/     概念知识（Generated）
├── 02-Market/       市场状态（Generated）
├── 03-Industries/   行业知识（Generated）
├── 04-Companies/    公司知识（Generated）
├── 05-Events/       事件知识（Generated）
├── 06-Topics/       热点题材（Generated）
├── 07-Emotion/      情绪记录（Generated）
├── 08-Cases/        历史案例（Generated）
├── 09-Personal/     个人认知（User Knowledge）
└── 99-Archive/      归档
```

---

## 17. Server / Local 边界

| 侧 | 职责 |
|---|---|
| **Server** | 数据采集、数据处理、知识抽取、AI 分析、实体关系、事件、市场状态、行业、公司、热点、**自动生成 Markdown** |
| **Local / Obsidian** | 阅读、学习、人工修改、个人笔记、个人判断、交易复盘、**Belief** |

---

## 18. Generated / Personal 隔离

强烈建议将两类知识隔离，避免 Git 冲突：

- `01-08/*`：Generated Knowledge（服务器生成，默认**不修改** Personal）
- `09-Personal/`：User Knowledge（本地维护）

---

## 19. Git 同步模型

```
Server ──git push──→ Git Repository ──git pull──→ Local Obsidian
```

- 服务器生成：`generated/`
- 本地维护：`personal/`

---

## 20. Knowledge Publishing（知识发布）

数据库中的对象（如 `Event #123`）被发布为带 YAML Front Matter 的 Markdown 文件，供 Obsidian 渲染与双链关联：

```markdown
---
id: event-123
type: event
date: 2026-08-26
status: published
---

# 某政策事件

## 发生了什么
...

## 事实
...

## 影响
### 行业
[[机器人]]
### 公司
[[XXX]]

## 因果链
[[政策]] → [[需求]] → [[行业景气度]] → [[公司利润]] → [[市场预期]]

## Evidence
...

## AI 分析
...

## 我的理解
> 等待填写
```

---

## 21. 个人认知流程

用户在 Obsidian 中的认知闭环：

```
Event → 阅读 → 理解 → 提出问题 → 写 Belief
      → 关联 Case → 实际观察 → 验证 → 更新 Belief
```

---

## 22. 分析框架

### 22.1 市场分析框架（Market Daily）

每日市场复盘建议包含 12 项：

```
1. 指数  2. 成交额  3. 涨跌家数  4. 涨停/跌停
5. 连板高度  6. 昨日强势股表现  7. 行业表现  8. 热点
9. 市场情绪  10. 资金  11. 重要事件  12. 我的判断
```

### 22.2 事件分析框架

对任何重要事件按以下链路分析：

```
发生了什么？ → 事实是什么？ → 影响什么变量？ → 影响哪些行业？
→ 影响哪些公司？ → 影响利润吗？ → 改变市场预期吗？
→ 当前价格是否已经反映？ → 资金如何反应？ → 情绪如何变化？
→ 我的判断是什么？
```

### 22.3 股票分析框架

```
公司 → 做什么？ → 怎么赚钱？ → 行业如何？ → 竞争如何？
→ 利润驱动因素 → 未来增长 → 风险 → 估值
→ 市场预期 → 资金 → 价格
```

### 22.4 热点分析框架

```
Topic → 为什么出现？ → 驱动因素（政策/产业/业绩/技术/资金）
→ 核心行业 → 核心公司 → 扩散 → 情绪
→ 持续性 → 分歧 → 退潮
```

### 22.5 情绪模型

情绪由多个 Observation 聚合得到：

```
Breadth + Limit Up + Limit Down + Broken Boards + Max Board
+ Strong Stock Performance + Topic Continuity + Risk Appetite
+ Profit Effect  →  Market Emotion State
```

> **Emotion State ≠ Buy/Sell Signal**

### 22.6 学习系统

知识库不是单纯的资料库，学习路径为：

```
Concept → Example → Case → Market Observation
        → Personal Understanding → Belief
```

示例：`PE → 学习定义 → 看公司案例 → 比较同行 → 观察市场 → 形成认知`

### 22.7 错误系统

必须在 `09-Personal/Mistakes/` 下记录每次错误：

```
我的判断 / 为什么这么判断 / 依据 / 结果 / 哪里错了
/ 正确解释 / 新的认知 / 是否更新 Belief
```

---

## 23. 知识生命周期

统一生命周期状态：

```
Discovered → Processed → Extracted → Verified
          → Published → Updated → Archived
```

---

## 24. Confidence（置信度）

所有 AI 推理必须带置信度：

- 等级制：`High / Medium / Low`
- 或连续值：`0.0 ~ 1.0`

> Confidence 表示模型对推理的信心，**不代表事实正确的概率**。

---

## 25. 数据版本

重要数据必须保留以下时间字段：

```
source / retrieved_at / published_at / created_at / updated_at
```

知识更新**不覆盖**历史结论（保留历史版本可追溯）。

---

## 26. Backend 架构与项目结构

### Backend 模块划分（Go）

```
Go
├── API
├── Worker
├── Collector
├── Processor
├── Knowledge Engine
├── Reasoning Engine
├── Publisher
└── Scheduler
```

### 推荐项目结构

```
piks/
├── cmd/
│   ├── api/
│   ├── worker/
│   ├── collector/
│   └── publisher/
├── internal/
│   ├── entity/  ├── concept/  ├── company/  ├── industry/
│   ├── event/   ├── observation/ ├── market/ ├── topic/
│   ├── emotion/ ├── evidence/ ├── inference/ ├── belief/
│   ├── case/    ├── relationship/
│   ├── ingestion/ ├── reasoning/ ├── knowledge/
│   ├── publishing/ └── sync/
├── migrations/
├── templates/
├── prompts/
├── configs/
└── docs/
```

---

## 27. 任务架构

系统长期运行依赖异步任务队列：

```
Collector → Task Queue → Processor → Knowledge Engine → Publisher
```

任务状态：`Pending / Running / Success / Failed / Retrying / Dead`

---

## 28. 可观测性

必须记录以下运行指标，以判断系统是否正常运行：

```
任务执行时间 / 成功率 / 失败率 / AI 调用次数 / AI Token
/ 数据源状态 / 知识生成数量 / 重复数量 / 异常数量
```

---

## 29. 安全（Security）

服务器敏感信息：

```
Secrets / API Keys / Database Password / Git Credentials
```

**不得写入 Git**，使用：

```
Environment Variables / Secret Manager
```

（部署规模扩大后再引入专门 Secret Management）

---

## 30. 备份（Backup）

| 资产 | 备份方式 |
|---|---|
| PostgreSQL | Daily Backup + Weekly Full Backup + Retention |
| Markdown | Git History |
| 原始数据 | Object Storage（S3-compatible） |

整体形成：`Database Backup + Git + Raw Data Archive` 三重保障。

---

## 31. 技术选型总结

| 模块 | 技术 |
|---|---|
| Backend | Go |
| Database | PostgreSQL |
| Vector | pgvector |
| Cache | Redis（按需） |
| AI | Provider Adapter |
| Storage | S3-compatible |
| Knowledge Format | Markdown + YAML |
| Knowledge UI | Obsidian |
| Version Control | Git |
| Deployment | Docker |
| Monitoring | Prometheus / Grafana（后期） |

---

## 32. 架构可持续性说明

架构能支撑 2～3 年的核心原因**不是**"用了很多技术"，而是：

### 核心模型稳定

```
Entity / Event / Observation / Relationship /
Evidence / Inference / Belief / Case
```

### 外围可替换

`AI / Data Source / Search / Storage / Obsidian` 等外围组件均可替换，不影响 Knowledge Core。

---

## 33. V1 范围与阶段规划

### 33.1 V1 明确不做（禁止清单）

```
❌ 自动交易  ❌ 自动荐股  ❌ 高频行情  ❌ 实时交易系统
❌ 复杂前端  ❌ Neo4j  ❌ Elasticsearch  ❌ Kubernetes
❌ 微服务  ❌ 多 AI Agent  ❌ 十几个数据源
```

V1 技术栈保持精简：**Go + PostgreSQL + AI + Markdown + Git + Obsidian**。

### 33.2 分阶段开发顺序

**Phase 0 — Model：** 定义 Entity / Event / Relationship / Evidence / Belief

**Phase 1 — Knowledge Core：** PostgreSQL + Go Domain Model + CRUD + Relationship

**Phase 2 — Obsidian Publisher：** DB → Markdown → Git → Obsidian

**Phase 3 — 第一批数据：** 仅接入市场数据 / 新闻 / 公告

**Phase 4 — AI：** Event Extraction / Entity Extraction / Relationship Extraction / Summary

**Phase 5 — Market Intelligence：** 市场 / 行业 / 热点 / 情绪

**Phase 6 — Personal Learning：** Case / Belief / Review / Mistake

### 33.3 V1 实际运行链路

```
新闻/公告 → Collector → PostgreSQL → Processor → AI
         → Structured Output → Validator → Knowledge Core
         → PostgreSQL → Markdown Generator → Git → Obsidian
         → 你 → Personal → Belief → Case
```

> 这条链跑通，**PIKS V1 即成立**。

---

## 34. 架构冻结项（Architecture Freeze）

为满足"2～3 年稳定"要求，以下项目作为 **Architecture Freeze**：

**核心对象（冻结）：**

```
Entity / Event / Observation / Relationship /
Evidence / Inference / Belief / Case
```

**核心原则（冻结）：**

```
Fact ≠ Inference ≠ Belief
```

**核心存储（冻结）：**

```
PostgreSQL = Source of Truth
Markdown   = Knowledge Projection
Obsidian   = Knowledge Interface
```

**核心同步（冻结）：**

```
Server → Git → Local Obsidian
```

**核心 AI 原则（冻结）：**

```
AI → Structured Output → Validation → Knowledge Core
```

**核心因果链（冻结）：**

```
Event → Factor → Industry → Company → Earnings
      → Expectation → Valuation → Capital → Emotion → Price
```

---

## 附录：系统核心价值

最终系统的价值**不是**"AI 告诉我明天买什么"，而是：

> **"我终于能解释为什么市场发生了这件事情。"**

示例：

```
今天机器人上涨 7%
  → 不是简单记录"机器人涨了"
  → 政策事件 → 影响产业预期 → 机器人行业预期变化
  → 核心公司盈利预期变化 → 资金进入 → 板块情绪升温
  → 股票价格上涨 → 我记录自己的判断 → 未来验证 → 形成新的认知
```

几年后，你得到的不是一堆新闻，而是一套**真正属于你自己的市场认知体系**。
