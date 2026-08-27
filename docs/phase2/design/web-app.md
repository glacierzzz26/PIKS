# 迭代 5 — Web 平台(替换 Obsidian 界面层)设计文档

> 状态:**草案(待定稿门确认,2026-08-27)**。用户决策:完全抛弃 Obsidian 与 GitHub,**界面层换成 Web 应用直连 PostgreSQL**,编辑(个人笔记)也搬进 Web,**Web 内嵌大模型对话页(可上传截图提问)**。定稿后进入阶段化实施。
> 日期:2026-08-27。契约依据:本系统 PostgreSQL=真相 架构(`项目详解.md`)、迭代 4 设计 `iter4-learning-loop.md`(personal_notes,本文改造其权威源方向)、生产化 `docs/phase3/stages/prod.md`(lab 部署模型)、`internal/publish`(渲染数据组装,Web 复用)。
> 前置:迭代 0~4 + 生产化 P3 已闭环。本次是**界面层重构 + 个人层改造**,管线(采集→抽取→聚类→行情→复盘)零改动。

---

## 1. 背景与决策(用户 2026-08-27 定)

现状问题:界面 = Markdown 投影 + Obsidian 查看 + GitHub 分发。Obsidian 体验不足(图谱裸 ID、跳转断、需本地拉取),GitHub 是外部依赖(网络不稳/认证)。用户决策:

1. **完全抛弃 Obsidian**——不再用 Markdown 做界面。
2. **编辑也搬进 Web**——个人笔记(Belief/Case/Mistake)在 Web 编辑,直接写库。
3. **不再需要 GitHub**——弃用 vault 的 Git 分发/备份通道。
4. **Web 内嵌大模型对话页**——可上传截图、向知识库提问,答案带引用可跳转。

> ⚠️ 备份替代(需确认):GitHub 曾是唯一**异地**备份。弃用后需替代方案,见 §8.4。

---

## 2. 目标

- 一个 Web 应用(局域网浏览器访问)替代 Obsidian 的全部「读」+「写」。
- 图谱可读、可跳转、可追溯相互影响;看板/复盘/事件流/实体卡一页直达;**支持缩放、点选即看内容**。
- 界面**时尚/美观/专业**(§3.4):卡片式布局、深浅色自适应、克制动效,是用户验收的一部分,不只是功能堆砌。
- AI 对话页:基于知识库问答(带引用)+ 截图上传(识别/问答)。
- 管线不动、数据模型增量(不重构 PG 存量)。

**架构转变**:

```
Before:  PostgreSQL ──投影──> Markdown(vault) ──Git push──> GitHub ──拉取──> Obsidian 界面
                            ▲                                      └─> 手写个人笔记 ──harvest──> PG
After:   PostgreSQL <═════════ Web 应用(lab :8090,读+写+AI)
                            └─ 直接读真相源,零投影层
```

---

## 3. 组件

### 3.1 Web 服务(`cmd/web`,Go)

- `net/http` + `html/template` 服务端渲染;一小撮原生 JS(图谱/ECharts 本地 vendored,不依赖 CDN)。
- compose 加 `web` 服务:同 postgres 网络,`0.0.0.0:8090` 暴露局域网。
- **大模型配置权威源 = 数据库 `app_config` 表**(2026-08-27 修订,替代 env 注入):`config.ApplyAppConfig` 从表合并,worker/cluster/entity-build/web 开库后套用;页面 `/settings` 可编辑(密钥掩码、留空=保持)。代码不再读 `PIKS_AI_*` 环境变量。
- 复用 `internal/publish` 的数据组装(`BuildEntityCardData`、快照聚合、`ListEventsByDate` 等),把 **Markdown 渲染换成 HTML 模板**——渲染逻辑不重写,只换格式层。
- 单用户 LAN 信任模型(与现状一致),不做登录;如需再议。

### 3.2 页面清单

| 页 | 内容 | 数据源 |
|---|---|---|
| 看板 `/` | 情绪趋势、热点板块、行业分布、Top 事件(即 demo 看板,升服务端渲染) | market_snapshots / events / industry_dist |
| 关系图谱 `/graph` | 事件↔实体 force layout;**局部图谱**(选中实体→其相关事件/相关实体),可点跳转 | events / entities / relationships |
| 事件流 `/events` | 按日分页,全文+影响+证据 | events / evidence / relationships |
| 事件卡 `/events/{id}` | 发生了什么/事实/影响(可点)/证据/**我的理解**(可编辑) | events + personal_notes |
| 实体卡 `/entities/{id}` | 基本信息/相关事件(可点)/相关实体(可点)/涨停 | entities / relationships |
| 复盘 `/reviews/{date}` | 每日复盘 12 项(继承 daily-review) | market_snapshots / observations |
| 对账 `/recon` | reconcile 异常清单可视化 | task_runs / 检查项 |
| 系统配置 `/settings` | 大模型配置编辑(密钥掩码、留空=保持) | app_config |
| 个人笔记 `/notes` | belief/case/mistake 列表/新建/编辑;关联事件/实体 | personal_notes / relationships |
| **AI 对话** `/chat` | 问知识库 + 上传截图(§4.3) | LLM + 检索 |

### 3.3 图谱交互(替代 Obsidian Graph View 的体验缺陷)

- 节点显示**实体真名 / 事件标题**(不再裸 ID)——修复已定位的悬空链接同源问题,这次在 web 直接渲染。
- **局部优先**:默认只画「选中事件↔其 affected 实体↔实体所属行业」邻域,全局图谱按需(性能 §7)。
- 点击事件→其影响实体→实体相关事件→……双向追溯;**affects 边带箭头与语义**(事件→实体=影响)。
- **缩放与拖拽**:滚轮/触控板缩放、拖拽平移、一键复位;局部↔全局无缝切换,节点过多时按阈值自动聚合。
- **点节点/卡片即看内容(不跳页)**:选中即右侧详情面板(或浮层卡片)即时呈现——事件显示标题/摘要/影响列表,实体显示基本信息/相关事件;看得清内容,再决定是否点进详情页。
- 颜色:事件=一类、实体按 type(company/industry/concept/topic)分区。

### 3.4 视觉与交互规范(时尚 · 美观 · 专业)

用户对界面的明确要求:**时尚、美观、专业**。落实为设计约束:

- **信息架构**:卡片式布局、清晰视觉层级(标题→摘要→细节),一屏一焦点,不堆砌不散乱。
- **视觉设计**:统一设计 token(配色/间距/圆角/字体),主题色克制——数据本身是主角,红涨绿跌等语义色不滥用;**深浅色主题自适应**(沿用看板 demo 的 `:root` + `prefers-color-scheme` token 方案)。
- **动效克制**:悬停高亮、展开/淡入过渡等微动效提升质感,不喧宾夺主(图谱 force layout 本身即天然动效)。
- **交互直觉**:图谱可缩放、可拖拽、点选看内容(§3.3);列表可筛选分页;页面导航可回退。
- **响应式**:浏览器自适应(桌面优先,移动端可用),不单独做 App。
- **验收归口**:§3.3 与本节共同作为 5-1 交互与视觉验收标准(见 §9 阶段表)。

---

## 4. AI 对话页(核心新增)

### 4.1 能力

1. **知识库问答(grounded)**:提问 → 检索 PG → 组装上下文 → LLM 回答 + **引用卡片链接**(可点跳转)。检索初版用**关键词**(title/summary/facts 全文 + 实体 name/alias),零成本、够用;语义/向量检索列为后续(§9 G8)。
2. **截图上传**:保存到 `data/uploads/` → 识别/问答。

### 4.2 模型档位(遵循 iter0 D2 分层)

- 问答用 **extract 档**(deepseek-v4-flash),便宜;截图深度分析/周报综述可选 **reasoning 档**(按需)。
- provider = 现有 Zen `/zen/go` 路由,配置读 `app_config` 表(`ai_service_base_url` / `ai_model_extract` / `ai_model_reasoning`),`/settings` 页编辑。

### 4.3 截图流程(契约缺口,§9 G7)

```
上传 → 校验类型/大小 → 存 data/uploads/{date}/ → 调 LLM(带图,若支持)
  ├─ 支持视觉: 直接问"图里讲了什么/与知识库何关" → 回答 + 引用
  └─ 不支持视觉: 降级——提示改用文字描述,或先 OCR(可选装 tesseract)→ 文本再走问答
```

**G7 探针**:接入前必须验证 Zen `deepseek-v4-flash` 是否接受 `image_url` 输入(OpenAI 兼容格式)。不支持则该特性降级(见上),如实标注,不造假。

### 4.4 对话历史(可选)

`chat_sessions` / `chat_messages`(type=user/assistant,含引用 ids、附件 id),保留用户提问轨迹供复盘。

---

## 5. 数据模型变更(迁移)

### 5.1 `personal_notes`(改造 iter4 设计:权威源翻转)

iter4 原设计:权威源=Obsidian,PG=harvest 投影(有 `source_path`/`content_hash`/`harvested_at` 收割语义)。Web 化后 **PG 即权威**,改造:

```sql
-- 新增列(相对 iter4 草案):
--   author TEXT NOT NULL DEFAULT 'me'   -- 单人系统,预留
--   updated_by TEXT                     -- 手动编辑记录
-- 语义调整:
--   source_path   → 去掉(无 Obsidian 文件)
--   content_hash  → 去掉(无文件快照)
--   harvested_at  → 改 created_at/updated_at 承担
--   slug          → 保留为稳定键(用户可给笔记起短名,如 "低价股并不代表便宜")
```

`type ∈ {belief, case, mistake}`;`status` 枚举沿用(belief:hypothesis/active/confirmed/questioned/rejected;case/mistake:active/archived);`confidence`(belief 自评);`detail` JSONB 存分节。**关联复用 `relationships`**:`references`(↔event/entity)、`supports`/`contradicts`(↔case/other belief)、`updates`(mistake→belief)。

### 5.2 新增表(聊天/附件,轻量)

```sql
CREATE TABLE chat_sessions ( id UUID PK DEFAULT gen_random_uuid(), title TEXT, created_at, updated_at );
CREATE TABLE chat_messages (
  id UUID PK DEFAULT gen_random_uuid(),
  session_id UUID REFERENCES chat_sessions, role TEXT,        -- user/assistant
  content TEXT,
  refs JSONB DEFAULT '{}'::jsonb,                             -- {"events":[...],"entities":[...]}
  attachments JSONB DEFAULT '[]'::jsonb,                      -- 关联截图
  created_at
);
-- 附件元数据(文件本体存 data/uploads/,不进库)
CREATE TABLE attachments (
  id UUID PK, filename TEXT, mime TEXT, size INT,
  path TEXT, created_at
);
```

### 5.3 事件卡「我的理解」归属

原为 Markdown 占位。Web 化后:事件卡上直接内嵌「我的理解」编辑框 → 落 `personal_notes(type='note' 或 belief, 关联该 event)`。设计取**归并入 personal_notes 并关联 event**(统一沉淀层,周报可聚合),不新增 `event_notes` 列。

---

## 6. 删除与保留

### 6.1 删除

| 项 | 处置 |
|---|---|
| Obsidian / vault 查看 | 整体下线 |
| GitHub 推送(publisher 的 commit/push) | 停用;vault 目录、setup.sh 的 clone/scp、deploy 中 vault 相关一并摘除 |
| publisher 命令 | 从 pipeline.sh 移除;渲染数据组装(§3.1)由 Web 复用,不删逻辑 |
| harvest 回写(迭代 4 原方案) | 取消——Web 直接写 personal_notes,不再需要 Obsidian→PG 单向收割 |

### 6.2 保留

- 管线(collector→worker→cluster→quote→market-state→daily-review→reconcile)**全部不动**。
- DB 备份 `backup.sh`(本地)。
- 复盘/事件/实体数据本就在 PG,Web 直接读,无数据迁移。

### 6.3 ⚠️ 备份替代(需用户确认)

弃 GitHub 后失去**异地**备份。建议二选一:
- A. 保留本地 DB 备份(现状)+ 定期 `pg_dump` 拷贝到 dev 机(异地一份,`rsync`/`scp`)。
- B. 接受单站点风险(个人系统,可接受则选)。
> 数据诚实:不推荐裸单机,至少 A。

---

## 7. 性能与规模

- 直接查 PG(索引齐全),渲染在 Go 侧,万级事件/千级实体无压力。
- 图谱**局部渲染**(§3.3),避免整图 force layout;整图按需 + 阈值聚合。
- 看板聚合查询走现有 market_snapshots,数据涨了加索引即可,无架构性问题。

---

## 8. 契约缺口

| # | 缺口 | 归属 | 处置 |
|---|---|---|---|
| G7 | Zen 视觉/图像输入支持(截图) | 迭代 5 | ⬜ **实施前探针验证**;不支持则截图降级(§4.3) |
| G8 | 语义检索(embedding) | 迭代 5+ | ⬜ 初版关键词检索够用;语义检索延后,不阻塞 |

---

## 9. 阶段化实施(每阶段独立验收)

| 阶段 | 内容 | 验收 |
|---|---|---|
| **5-1 Web 只读**(替代「看」) | web 服务 + 看板 + 事件流/事件卡 + 实体卡 + 复盘页 + 局部图谱 | 浏览器访问 :8090;图谱可缩放/拖拽、点节点弹内容(不跳页)、点实体↔事件可跳详情;看板=现 demo 数据;界面符合 §3.4 视觉规范 |
| **5-2 编辑 + 去界面层**(替代「写」) | personal_notes 编辑(Belief/Case/Mistake)+ 事件卡「我的理解」;publisher/Obsidian/GitHub/harvest 下线;备份替代落地 | Web 新建/编辑笔记并关联事件/实体;周报聚合个人笔记;vault/GitHub 停更;备份生效 |
| **5-3 AI 对话 + 截图** | `/chat`:问答带引用;截图上传识别/降级 | 问"降准影响哪些板块"→ 答案带事件卡链接可点;截图流程走通(G7 通过或如实降级) |

每阶段:设计 §该阶段细化 → 定稿 → 实现 → 归档 `stages/web-5.md` → 更新进度总表。

---

## 10. 边界(明确不做)

- ❌ 不做多用户/权限(单人 LAN 系统)。
- ❌ 不做移动端 App(浏览器自适应即可)。
- ❌ 不做语义向量检索于初版(G8 延后)。
- ❌ 不重构 PG 存量 schema(只增量加表/列)。
- ❌ 不保留 Markdown 生成(export 若需要,迭代 5+ 另议,不阻塞)。

---

## 涉及文件

- 新增:`cmd/web/`(HTTP 服务 + 模板)、`internal/web/`(页面 handler)、`migrations/0006_*.sql`(personal_notes 改造 + chat/attachments)、`web/static/`(JS/CSS,ECharts 本地 vendored)。
- 复用:`internal/publish` 数据组装、`internal/store`、`internal/config`(LLM env)。
- 修改:`configs/docker-compose.prod.yml`(+web 服务)、`scripts/pipeline.sh`(去 publisher)、`scripts/setup.sh`/`scripts/deploy.sh`(去 vault/GitHub 相关)。
- 文档:本设计定稿后,`进度总表.md` 登记迭代 5;旧 iter4 设计中「Obsidian 权威源/harvest」章节标注被本文替代。
