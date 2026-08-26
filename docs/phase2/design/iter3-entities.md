# 迭代 3 — 实体补全 设计文档

> 状态:**已定稿冻结(2026-08-26,D18~D21 用户确认)**。此后按它执行,改动需走变更。
> 日期:2026-08-26。契约依据:`PIKS架构设计文档.md` §9.1~9.4 / §9.9 / §33.2 Phase 4(Entity Extraction);`docs/进度总表.md` 迭代 3 行;迭代 2 D15(热点 topic 关联延到实体补全)+ `stages/iter2.md` §5 待续;`internal/publish/publish.go`(事件卡 affected 已留 `[[%s]]` 未解析链接,注释"迭代 3 建实体后自动可跳")。
> 前置:迭代 0/1/2 已闭环。**本迭代少量 AI 调用**(实体分类一次,便宜档),不同于迭代 2 的零 AI。

## 1. 目标与范围

### 目标

把「事件 + 行情」升级为「事件 + **实体** + 行情」:从已有事实层(events.affected / 东财涨停池)沉淀出**可链接、可查询**的实体对象(Company / Industry / Concept / Topic),闭环三件事:

1. **事件卡 affected 变可跳转 wikilink**(iter1 遗留:`[[银行]]` 现在指不到任何卡片 → 建实体后 `[[entity-xxx|银行]]`)。
2. **hot_topics 补 event_ids**(iter2 D15 遗留:迭代 2 只做涨停行业派生,事件↔行业关联因无实体层留空)。
3. **实体知识卡**(行业/公司/概念第一个可复盘形态:涨停日期计数、相关事件、相关实体)。

### 范围(按来源×自动化)

| # | 实体来源 | 类型 | 自动化 | 说明 |
|---|---|---|---|---|
| 1 | 东财涨停池 `zt_pool`(近 N 交易日合并) | Company / Industry | ✅ 零 AI | `{code,name,hybk}` → Company + Industry + belongs_to 关系,真实上市公司 |
| 2 | `observations.industry_dist`(涨停行业) | Industry | ✅ 零 AI | 确认行业集合 + 涨停计数 |
| 3 | `events.affected` 字符串(已 AI 抽取的实体词) | 混合 | ✅ 匹配 / 🔶 分类 | 名称匹配(含后缀剥离)→ 命中即关联;未命中进批量分类 |
| 4 | 事件卡/行情页展示 | — | ✅ 零 AI | 实体卡渲染 + affected wikilink 解析 + hot_topics 补链 |

### 边界(明确不做)

- ❌ 不做行业产业链(upstream/midstream/downstream 全量)—— 只建行业实体 + 计数,parent 链迭代后补(D20)
- ❌ 不做 Topic 生命周期(驱动/阶段/风险全字段)—— Topic 只作分类标签,detail 留空
- ❌ 不做实体属性自动补全(Company 的 business_model/valuation 等留 `Unknown`,架构 §9.3 允许)
- ❌ G3 巨潮公告 **不并入本迭代**(独立数据源 + 事件抽取链路,范围独立;一次不做完,D21 待确认)
- ❌ G4 财务数据 / G5 宏观政策 不在本迭代(进度总表契约缺口表后移,见 §6 D24)

## 2. 数据模型变更(迁移 `0004_entities.sql`)

### 2.1 `entities` — 统一实体表(架构 §9.1 继承模型落地)

单表 + `type` 判别 + `detail` JSONB 存类型专属字段。**与 relationships 的多态 from_type/from_id 天然契合**(迭代 0 已建)。`aliases` 支持"银行板块"↔"银行"这类措辞方差(迭代 1 聚类教训)。

```sql
CREATE TABLE entities (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type        TEXT NOT NULL,          -- company/industry/concept/topic/unknown
  name        TEXT NOT NULL,          -- 规范名(如"银行")
  aliases     JSONB NOT NULL DEFAULT '[]'::jsonb,  -- ["银行板块","银行股"] 措辞变体
  description TEXT,
  detail      JSONB NOT NULL DEFAULT '{}'::jsonb,  -- company:{code,exchange} industry:{source:eastmoney}
  status      TEXT NOT NULL DEFAULT 'active',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (type, name)
);
CREATE INDEX idx_entities_type ON entities(type);
CREATE INDEX idx_entities_name ON entities(name);
```

**类型枚举**:`company / industry / concept / topic / unknown`(unknown = AI 也判不出的诚实落点,不猜类型)。

### 2.2 关系(复用 `relationships`,无新表)

| from_type | to_type | rel_type | 语义 |
|---|---|---|---|
| event | entity | `affects` | 事件涉及实体(从 events.affected 收割) |
| entity(company) | entity(industry) | `belongs_to` | 公司所属行业(东财 hybk) |
| entity(industry) | entity(concept/topic) | `tagged_as` | 行业↔概念/题材标签(可选,iteration 内不强求) |

> relationships 已有 `UNIQUE(from_type,from_id,to_type,to_id,rel_type)`,天然幂等。

### 2.3 实体 ↔ 已有表血缘

- **company.detail.code** ↔ `zt_pool.code`(东财)↔ 未来行情/财务数据锚点(迭代 4+ 财务 G4)。
- **industry.name** ↔ `observations.industry_dist`(涨停计数)↔ `market_snapshots.hot_topics`(补 event_ids)。

## 3. 命令与组件

### 3.1 `cmd/entity-build` — 实体构建(核心)

**步骤(幂等,重跑零变更):**

1. **种子源(零 AI)**:读 `market_snapshots.zt_pool`(近 30 交易日,合并去重)提取 `{code,name,hybk}`。
   - Company 实体(`detail:{code}`);Industry 实体(`detail:{source:eastmoney}`,`aliases` 含 hybk 常见变体)。
   - `belongs_to`:company → industry。
2. **事件 affected 收割**:全量 `events.affected` 字符串去重集合。
   - **匹配**:精确匹配实体 name / aliases;或剥离后缀(`板块`/`概念`/`指数`/`股`/`类`)后再匹配。命中 → 建 `affects` 关系,并把原字符串补进 aliases。
   - **未命中** → 待分类列表(下一步)。
3. **AI 批量分类(便宜档)**:未命中词一次性发分类(§4)。结构化输出:
   ```json
   {"entities":[{"name":"LPR","type":"concept","aliases":[]},{"name":"宁德时代","type":"company","aliases":[]}]}
   ```
   `type ∈ {company,industry,concept,topic}`。建实体 + `affects` 关系。**输出非法/失败 → 建 `type=unknown` 实体并记 task_runs meta**(诚实,不猜类型,§5.7)。
4. **task_runs**:`entity-build` 记 `{seed_companies, seed_industries, affected_terms, matched, classified, unknown, token_count}`。

### 3.2 `internal/entityextract` — 分类提示词 + Schema(复用 internal/extract 模式)

- JSONSchema:实体数组,`name` string / `type` enum / `aliases` array。
- 提示词立场:"你是中国 A 股金融实体分类器。把实体词归类为 company/industry/concept/topic。拿不准用 topic,绝不用 industry 指代公司。" + 已知实体清单上下文(去重,帮模型对齐规范名)。
- 复用 `internal/ai.Provider.StructuredOutput` + `internal/validate` 校验。

### 3.3 `internal/publish` 扩展 — 实体卡 + affected wikilink

- **实体卡**:`03-Entities/{type}/{name}.md`。
  ```markdown
  ---
  id: entity-{short8}
  type: entity
  entity_type: company
  name: 宁德时代
  status: active
  pipeline: entity-build@<git-short>
  ---
  # 宁德时代
  ## 基本信息(code / 行业)
  ## 相关事件(affects 关系)
  ## 相关实体(行业/概念)
  ## 涨停记录(zt_pool 日期计数)
  ```
- **事件卡 affected 解析**:publisher 渲染事件卡时,查询 `entities by name/alias` → 命中则 `[[entity-xxx|原始词]]`,未命中保持纯文本(诚实)。**重跑零提交**:实体不变则内容逐字节相同。
- 发布目录:03-Entities 进 Generated 仓库。

### 3.4 `cmd/market-state` 扩展 — hot_topics 补 event_ids

- ComputeSnapshot 后,对 hot_topics 各行业名:查 Industry 实体 → `affects` 该行业的事件 → 填 `event_ids`。
- `hot_topics` 形状收敛到迭代 2 设计:`[{name, count, event_ids:[...]}]`(迭代 2 只填了 name/count)。
- **无实体关系时保持迭代 2 现状**(event_ids 空),不中断(降级兼容)。

### 3.5 采集契约与源健康

- 实体构建是**纯派生**(读存量表),无新外部源,无 SLA 风险。
- G3 巨潮若并入(见 D21):独立 collector + 探针,同 G1/G2 纪律(真实 DTO / 宁缺毋假 / 源健康连续失败暂停)。

## 4. AI 成本策略

- **唯一 AI 调用**:未命中 affected 词的**一次批量分类**。量级估算:当前 ~9 事件 × 2~3 词 ≈ 20~30 词 → 单次结构化输出 ≈ 数百 token。**用便宜档**(`AIModelExtract`,现 deepseek-v4-flash)。
- 复用 `AIDailyTokenBudget` 护栏(0=关);task_runs 记 token。
- **绝不逐事件调 LLM 做实体抽取**(事件 affected 已在迭代 0/1 由抽取链路产出,本迭代只分类,不重抽)。

## 5. 验收标准(smoke test)

| # | 标准 | 判定 |
|---|---|---|
| §5.1 | 迁移 0004:`entities` 建表 + 索引;relationships 复用 | `information_schema` + `pg_indexes` |
| §5.2 | 种子源实体:zt_pool 建 Company/Industry + belongs_to;重跑幂等零新增 | 实体计数 + 关系计数 + 重跑对比 |
| §5.3 | 事件↔实体:affected 匹配/分类 → affects 关系;未分类如实 unknown/pending | 抽样事件关系 + unknown 计数入 meta |
| §5.4 | hot_topics 补链:market-state 重算 → 行业热点 event_ids 非空 | 快照 hot_topics JSON 抽查 |
| §5.5 | 实体卡发布:03-Entities/ 卡片生成;事件卡 affected 变 `[[entity-xxx|…]]`;重跑零提交 | vault git 两次对比 |
| §5.6 | 回归:迭代 0/1/2 全链不受影响 | worker/cluster/market-state/daily-review/reconcile smoke |
| §5.7 | 诚实:AI 分类失败/非法输出 → 建 unknown,不猜类型;匹配失败不假造关系 | task_runs meta 抽查 |

## 6. 定稿拍板点(已确认,2026-08-26)

- **D18 ✅ 实体表形态 = 单表 `entities`(type 判别 + detail JSONB)**,非每类型一表。依据:relationships 已多态、V1 保持精简、架构 §9.1"Entity 统一基础"。
- **D19 ✅ AI 用量 = 便宜档批量分类未命中 affected 词(一次,数百 token)**:未匹配词一次性发 deepseek-v4-flash 分类;拿不准建 `type=unknown`,诚实不猜。备选(纯规则零 AI,误分类风险高)未采纳。
- **D20 ✅ 行业深度 = 仅东财叶子行业 + 涨停/事件计数**,不建 parent/产业链(一次不做完)。
- **D21 ✅ G3 巨潮公告 = 延后(不并入本迭代)**,独立数据源 + 抽取链路,范围独立。
- **D22 ✅ 实体卡目录 = `03-Entities/{type}/`**(Generated 仓库,与 02-Market/05-Events 平级),事件卡 affected 解析为 wikilink。
- **D23 ✅ 接受项**:G4/G5 财务/宏观不在本迭代;hot_topics event_ids 补链按 §3.4 降级兼容。

## 7. 交付物清单(定稿后按此执行)

| 交付物 | 说明 |
|---|---|
| `0004_entities.sql` | entities 表 + 索引 |
| `internal/entityextract/` + `cmd/entity-build` | 实体构建(种子源 + 收割 + 分类) |
| `internal/publish/` 实体卡 + affected 解析 | 03-Entities 渲染 + 事件卡 wikilink |
| `cmd/market-state` 扩展 | hot_topics 补 event_ids(降级兼容) |
| store: entities / relationships 查询 | ListEntitiesByType / GetEntityByName / ListRelationships 等 |
| 归档 `docs/phase2/stages/iter3.md` + 进度总表更新 | |
