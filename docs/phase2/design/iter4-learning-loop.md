# 迭代 4 — 个人学习闭环 设计文档

> 状态:**已定稿冻结(2026-08-27,D24~D29 用户确认)**。此后按它执行,改动需走变更。
> 日期:2026-08-27。契约依据:`PIKS架构设计文档.md` §9.12 Belief / §9.13 Case / §21 个人认知流程 / §22.6 学习系统 / §22.7 错误系统 / §33.2 Phase 6(Personal Learning);`docs/项目详解.md` 执行决策(分域双源:个人层 Obsidian 唯一可信源,Phase 3 加单向 harvest 收割器);`docs/进度总表.md` 迭代 4 行 + 契约缺口 G6;iter0 设计 §5 模型分层(D2:周报/深度复盘走**高智档**)与表 5.3「周报/深度复盘/疑难判断 → 高智档(全系统唯一值得花大钱处)」;iter2 设计 §4「周报按需接推理档(迭代 4)」;`PIKS-Vault/09-Personal/PIKS-Obsidian-使用教程.md` §4.2 目录规划。
> 前置:迭代 0~3 + 生产化 P3 已闭环。本迭代 **harvest 零 AI,周报可选一次高智档综述**(符合 iter0 D2 模型分层;不同于迭代 2/3 的纯零 AI)。

## 1. 目标与范围

### 目标

把「系统沉淀事实,你阅读加工」升级为「系统沉淀事实 + **你沉淀认知**,认知反哺系统」:你在 Obsidian `09-Personal` 手写的 **Belief / Case / Mistake**,经**单向 harvest 收割器**回写到 PostgreSQL,再由**周报**聚合「本周市场 × 本周你的沉淀」,闭环架构 §21 个人认知流程:

```
Event → 阅读 → 理解 → 写 Belief / Case / Mistake(09-Personal,你手写)
      → harvest(单向回写 PG)→ 关联 Case → 观察验证 → 更新 Belief
      → 周报(本周行情 × 你的沉淀,聚合复盘)
```

**分域双源是前提**:个人层**权威源 = Obsidian 09-Personal(独立仓库 PIKS-Personal)**;harvest 是**单向只读投影**到 PG(服务端查询/周报用)。服务器不写、不改、不推 09-Personal。

### 范围(按来源×自动化)

| # | 交付 | 数据来源 | 自动化 |
|---|---|---|---|
| 1 | Belief 收割 | 09-Personal 手写(front matter `type: belief`) | ✅ 规则解析,零 AI |
| 2 | Case 收割 | 同上(`type: case`) | ✅ 规则解析,零 AI |
| 3 | Mistake 收割 | 同上(`type: mistake`,目录已存在) | ✅ 规则解析,零 AI |
| 4 | 个人↔知识图谱链接 | 正文 wikilink(`[[event-xxx]]`/`[[entity-xxx]]`/跨 note) | ✅ 规则解析 → relationships |
| 5 | 周报 | market_snapshots + events + 已收割 personal_notes | ✅ 规则聚合 + 🔶 高智档综述(可选,D26) |

### 边界(明确不做)

- ❌ **harvest 不写/不改/不推 09-Personal**——单向只读;服务器对个人层只有读投影(项目详解分域双源)。
- ❌ **不收割自由笔记**(复盘/笔记/,无 `type` 标记)——只收割 front matter `type ∈ {belief,case,mistake}` 的文件,复盘/笔记保持纯 Obsidian(不 lossy 入库)。
- ❌ **AI 转写用户笔记**(帮用户"总结"自己的想法)——有失真风险,违背 Fact≠Inference≠Belief;Belief 是用户自己的认知,harvest 只做结构提取不添油加醋。
- ❌ **Belief 状态不自动迁移**(Hypothesis→Active→…→Rejected 是用户的认知决策,harvest 只镜像 front matter `status`,不推断,D29)。
- ❌ **G3 巨潮公告 / G4 财务 / G5 宏观继续延后**——独立数据源接入,与个人学习闭环无耦合;一次不做完(D28)。
- ❌ 不做 Case 的自动推荐/相似度匹配、不做周报之外的深度复盘综述。

## 2. 数据模型变更(迁移 `0005_personal.sql`)

### 2.1 `personal_notes` — 个人认知投影(架构 §9.12/9.13 + §22.7)

单表 + `type` 判别 + `detail` JSONB(**沿用迭代 3 entities 模式**)。方向与 Generated 层相反:**权威源在 Obsidian,PG 是 harvest 的单向投影**,供服务端查询/周报/未来分析。

```sql
CREATE TABLE personal_notes (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type          TEXT NOT NULL,           -- belief / case / mistake
  slug          TEXT NOT NULL,           -- 文件名基名(稳定键,如 "低价股并不代表便宜")
  title         TEXT,                    -- front matter title → H1 → 文件名
  status        TEXT NOT NULL DEFAULT 'hypothesis',  -- belief:hypothesis/active/confirmed/questioned/rejected;case/mistake:active/archived
  confidence    NUMERIC,                 -- belief 用户自评(可选,0~1)
  content       TEXT,                    -- 收割时全文快照(服务端查询/周报引用)
  detail        JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {"sections":[{"section":"陈述","text":"..."}]}
  source_path   TEXT NOT NULL,           -- 09-Personal 内相对路径(权威定位)
  content_hash  TEXT NOT NULL,           -- md5,幂等跳过(重跑零 churn)
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),  -- 首次收割时间 = "本周新增"基准
  harvested_at  TIMESTAMPTZ NOT NULL DEFAULT now(),  -- 最近收割时间
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (type, slug)
);
CREATE INDEX idx_personal_notes_type   ON personal_notes(type);
CREATE INDEX idx_personal_notes_status ON personal_notes(status);
```

**状态枚举**:belief 用架构 §9.12 `hypothesis / active / confirmed / questioned / rejected`;case/mistake 用 `active / archived`(mistake 如实验记录,不"通过/驳回",只归档)。

### 2.2 关系(复用 `relationships`,无新表)

| from_type | to_type | rel_type | 语义 | 来源 |
|---|---|---|---|---|
| personal_note(belief) | event | `references` | 信念基于的事件 | 正文 `[[event-xxx]]` |
| personal_note(belief) | entity | `references` | 信念涉及的实体 | 正文 `[[entity-xxx]]` |
| personal_note(belief) | personal_note(case) | `supports` / `contradicts` | 支持/反例案例 | `## 支持案例` / `## 反例` 链接 |
| personal_note(belief) | personal_note(belief) | `supports` / `contradicts` | 信念互相印证/矛盾 | front matter `related_beliefs` 或正文链接 |
| personal_note(case) | event / entity | `references` | 案例关联事实 | 正文 wikilink |
| personal_note(mistake) | personal_note(belief) | `updates` | 错误 → 更新某信念 | front matter `updates_belief: <slug>` |
| personal_note(mistake) | event / entity | `references` | 错误发生的场景 | 正文 wikilink |

> relationships `UNIQUE(from_type,from_id,to_type,to_id,rel_type)` 天然幂等,重跑不重复。

### 2.3 血缘

- `personal_notes.*` ↔ 09-Personal 文件(`source_path` + `content_hash`);`status`/`confidence` 镜像 front matter,不推断;`detail` 是章节切分快照。

## 3. 命令与组件

### 3.1 `cmd/harvest` — 单向收割(核心,G6)

**输入**:`PIKS_PERSONAL_VAULT_PATH`(lab:`/srv/vault-personal`,即 PIKS-Personal 私有仓 clone;dev 默认 `./PIKS-Vault/09-Personal`,开箱即用)。

**步骤(幂等,重跑零 churn):**

1. **递归扫 `.md`**,读 front matter:
   - `type ∈ {belief, case, mistake}` → 进入收割(其余跳过,含 `type: journal` 的复盘/笔记)。
   - 缺 type / 非法 type → 记 task_runs meta `skipped_untyped`(诚实,不动用户文件)。
2. **结构提取**(纯规则,零 AI):
   - `title` = front matter `title` → 首个 H1 → 文件名。
   - `slug` = 文件名基名(去 `.md`),唯一键。
   - `status` / `confidence` = front matter 透传(缺失按 type 默认值)。
   - `detail.sections` = 正文按 `## <标题>` 切段,只收**已知章节**:belief 的 `陈述/理由/支持案例/反例`;case 的 `事件/初始假设/市场反应/结果/教训`;mistake 的 `我的判断/为什么这么判断/依据/结果/哪里错了/正确解释/新的认知/是否更新 Belief`。**未知章节原样保留**(不丢用户文字,不猜语义)。
3. **wikilink 解析** → relationships:
   - 命中前缀 `event-` / `entity-` → 对应表 lookup → `references`。
   - 命中本表 `belief-* / case-* / mistake-*` 或目录+文件名 → 按 `(type, slug)` 关联。
   - 其余按**实体 name/alias 解析**(复用 iter3 entityextract 的 resolve 逻辑,用户可能写 `[[银行]]` 而非 `[[entity-xxx]]`)。
   - 全部未命中 → 记 task_runs meta `unresolved_links`(**不假造关系**)。
   - front matter `related_beliefs` / `updates_belief` → 按 §2.2 建 belief↔belief / mistake→belief 关系。
4. **落库**:按 `(type, slug)` upsert;`content_hash` 相同 → 跳过(零 churn);变化 → 更新 content/status/detail/links + `updated_at`。
5. **删除同步**:clone 中已不存在、PG 中还在的 note → `status=archived`(软删保历史,不硬删)。
6. **task_runs**:`{notes_scanned, harvested, changed, archived, skipped_untyped, unresolved_links, first_seen, git_short}`。

**同步机制(D25)**:lab 端 weekly 段先 `git -C /home/rguo/piks/vault-personal pull --ff-only`,再跑 harvest。凭据复用现有 `PIKS_VAULT_GIT_TOKEN`(同一账号私有仓,credential helper 同款)。harvest 本身只读本地 clone。

### 3.2 `internal/personal` — 解析 + store(复用 internal/extract 与 store 模式)

- `ParseNote(path, content) (Note, error)`:front matter(YAML)+ 章节切分 + wikilink 提取,**纯函数可单测**。
- store:`UpsertPersonalNote / GetPersonalNote / ListPersonalNotes(type/status/since filter) / ArchivePersonalNote` + relationships 复用。

### 3.3 `cmd/weekly-report` — 周报聚合页(02-Market/weekly/)

**输入**:`-date`(默认今天,北京时区)→ 该 ISO 周;读 market_snapshots(本周交易日)、events(本周发布)、personal_notes(`first_seen_at`/`updated_at` ∈ 本周)。
**输出**:`02-Market/weekly/YYYY-WW.md` → git commit(复用 publisher 的 md5 幂等,重跑零提交)。

模板:

```markdown
---
id: weekly-2026-W35
type: weekly-review
week: 2026-W35
emotion: Neutral            # 本周日均情绪
pipeline: weekly-report@<git-short>
---

# 周报 2026-W35

## 本周行情概览  指数周涨跌 / 情绪分布 / 涨停·跌停统计(market_snapshots 聚合)
## 本周重要事件  top events wikilink(events)
## 本周你的沉淀  新增/更新 Belief·Case·Mistake 列表 → [[<slug>|标题]] 链回 09-Personal 原文
## Belief 状态一览  当前全部 active beliefs + status(提醒复盘/更新)
## AI 综述        (D26 若采纳)高智档一段,严格只总结上方已有数据;失败 → _综述生成失败(暂缺)_
```

**分域**:周报页进 **Generated 仓库**(服务器生成);「本周你的沉淀」只**引用**个人笔记(`[[slug|标题]]`,Obsidian 按文件名跨仓库解析),不改写 09-Personal。

### 3.4 管线接入(lab `pipeline.sh` 增 weekly 段)

- 日管线**不变**(harvest 不塞进每日;周报只在周五收盘后)。
- weekly 段(周五且过 16:10 且无 `weekly-$(date +%G-W%V).done` stamp):
  1. `git -C vault-personal pull --ff-only`(个人仓同步)
  2. `run harvest`
  3. `run weekly-report`
  4. 打 weekly stamp
- 失败不阻断日管线,下周/下 tick 重试(幂等)。

## 4. AI 成本策略

- **harvest = 零 AI**(纯规则)。个人笔记是**用户事实**,AI 转写有失真风险;模板驱动(front matter + 约定章节)保证结构化。
- **weekly-report = 规则聚合(必做,零 AI)+ 高智档综述(可选,D26)**:综述每周一次、数百 token,符合 iter0 D2「周报/深度复盘 → 高智档,全系统唯一值得花大钱处」。提示词护栏:只允许总结上方已列出的数据,禁止新增事实/推断,限长度。
- 复用 `AIDailyTokenBudget` 护栏;task_runs 记 token。

## 5. 验收标准(smoke test)

| # | 标准 | 判定 |
|---|---|---|
| §5.1 | 迁移 0005:`personal_notes` 建表 + 索引 | information_schema + pg_indexes |
| §5.2 | harvest 解析:构造 belief/case/mistake 三类 sample md(front matter + 章节 + wikilink)→ 落库 + 关系正确 | 单测 + 落库抽查 |
| §5.3 | 幂等:同文件重跑 → content_hash 跳过,updated_at 不变,关系不重复 | 两次运行对比 |
| §5.4 | 删除同步:删文件重跑 → `status=archived` | 运行对比 |
| §5.5 | 诚实:无 type 文件跳过记 meta;未解析 wikilink 记 unresolved_links 不造假;AI 综述失败 → 空态占位 | task_runs meta 抽查 |
| §5.6 | 周报生成:02-Market/weekly/YYYY-WW.md 五节齐全、wikilink 指向真实对象;重跑零提交 | 运行 2 次对比 |
| §5.7 | 回归:迭代 0~3 全链不受影响 | 全命令 smoke |
| §5.8 | 分域:harvest 只读不写 09-Personal;Generated 仓库不含个人内容 | git 状态对比 |

## 6. 定稿拍板点(已确认,2026-08-27)

- **D24 ✅ 个人层存储形态 = 单表 `personal_notes`**(type 判别 + detail JSONB),权威源=Obsidian,PG=单向投影。依据:沿用 entities 单表先例,harvest/store/渲染一套代码。
- **D25 ✅ 收割同步 = lab 新增 PIKS-Personal 私有仓 clone**(`/home/rguo/piks/vault-personal`,compose 挂载 `:ro`),weekly 段 `git pull --ff-only` 后 harvest 读 clone;凭据复用现有 vault token(同账号私有仓)。
- **D26 ✅ 周报 = 规则聚合(必做,零 AI)+ 高智档综述一段**(符合 iter0 D2「周报走高智档」,每周一次成本可忽略;护栏:只总结已有数据、禁新增事实/推断、限长度,失败 → 空态占位)。
- **D27 ✅ 周报目录 = `02-Market/weekly/YYYY-WW.md`**(复盘报告与每日复盘同区,front matter `type: weekly-review` 区分)。
- **D28 ✅ G3/G4/G5(公告/财务/宏观)继续延后**,不进本迭代(独立数据源接入,与个人学习闭环无耦合,一次不做完)。
- **D29 ✅ Belief 状态只镜像 front matter `status`,不自动迁移**(状态迁移是用户认知决策,harvest 不推断)。

## 7. 交付物清单(定稿后按此执行)

| 交付物 | 说明 |
|---|---|
| `0005_personal.sql` | personal_notes 表 + 索引 |
| `internal/personal/` + `cmd/harvest` | 收割解析(front matter + 章节 + wikilink)+ 落库 + 关系 + 软删 |
| `cmd/weekly-report` + 周报模板 | 02-Market/weekly/ 聚合页(规则 + 可选综述) |
| store: personal_notes 查询 | ListPersonalNotes / Upsert / Archive 等 + relationships 复用 |
| 09-Personal 模板文件 | Belief/Cases/Mistakes 模板 + 教程更新(§4.2 目录补全) |
| lab 运维:clone PIKS-Personal + compose 挂载 + pipeline.sh weekly 段 + `.env` 增 `PIKS_PERSONAL_VAULT_PATH` | dev 侧脚本同步(setup/deploy 扩展) |
| 归档 `docs/phase2/stages/iter4.md` + 进度总表更新 + design/README.md 登记 | |
