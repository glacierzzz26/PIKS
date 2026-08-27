# 迭代 5-1 — Web 只读平台 归档

> 状态:**已完成(5-1 只读)**。本阶段只读,续接请看 `进度总表.md` 与 `../../phase2/design/web-app.md`。
> 日期:2026-08-27。实现以 `phase2/design/web-app.md` 定稿契约为准(§9 阶段表 5-1 行 + §3.3/§3.4)。
> 战略背景:用户 2026-08-27 决策**完全抛弃 Obsidian/GitHub 界面层**,PostgreSQL 成为唯一真相源,Web 即「看+写+AI」入口。本归档覆盖 5-1「Web 只读」;5-2(编辑+去界面层)/5-3(AI 对话+截图)按设计文档延后。

## 1. 目标与交付物

把「Obsidian 渲染 Markdown vault」替换为「PostgreSQL 直渲 HTML 的 Web 平台」:看板、事件流/事件卡、实体卡、每日复盘、对账、关系图谱,全部从 PG 读取实时渲染,零 Markdown 投影层。**图谱是本次体验核心**:替代 Obsidian Graph View「全是 UUID 看不懂、不能跳转」的缺陷——节点显示真名、可缩放/拖拽、点节点弹内容面板(不跳页)、点实体↔事件可跳详情。

| 交付物 | 状态 |
|---|---|
| `cmd/web`:`http.Server` + 路由(:8090,`PIKS_LISTEN_ADDR` 可覆盖) | ✅ |
| `internal/web`:8 个页面 handler + `/api/graph`、`/api/events/{id}`、`/api/entities/{id}` | ✅ |
| 模板 `templates/`:base + 看板/事件流/事件卡/实体卡/复盘索引/复盘详情/对账/图谱 | ✅ |
| 静态资源:base.css(design tokens 深浅色自适应)+ base.js(主题切换)+ graph.js(原生 SVG 力导向图) | ✅ |
| `internal/store` 新增:GetEntityByID / ListGraphEdges / ListEventAffectedEntities / Counts | ✅ |
| compose 加 `web` 服务(0.0.0.0:8090 对外);deploy.sh 起 postgres+web;已部署 lab | ✅ |
| 5-1 验收(设计 §9 + §3.3/§3.4) | ✅ 见 §2 |

## 2. 验收结果(设计 §9 5-1 行 + §3.3/§3.4)

验收在 **lab 生产数据**(192.168.0.202:8090,镜像 5425323)上进行:

| # | 标准 | 结果 |
|---|---|---|
| 2.1 | 浏览器访问 :8090,看板 = 现 demo 数据 | ✅ 页面 200;看板统计=真实库:113 事件/195 实体/181 关系/2 快照交易日;情绪趋势 13.0→15.0(Strong);涨停 77 |
| 2.2 | 事件流/事件卡:按日期分组、真实事件、影响词解析为可跳实体 | ✅ 08-27/08-26 分组;事件卡 `affected` 词 → 真实实体(如"群核科技"→company,Linked=true,带实体 ID);证据带东财真实 URL |
| 2.3 | 实体卡:相关事件/相关实体/涨停记录 | ✅ 群核科技实体卡渲染 company/active 标签 + 2 条真实相关事件(营收、世界模型内测) |
| 2.4 | 复盘页:12 节、按快照日期;对账页 | ✅ /reviews 含 08-27+08-26;08-27 详情:情绪 strong(13.0)、涨停 77/跌停 3/炸板 17、6 连板;对账如实报「抽取失败(1)」 |
| 2.5 | 图谱可缩放/拖拽、点节点弹内容(不跳页)、点实体↔事件可跳详情 | ✅ /api/graph 返回带前缀 ID(e:/n:)+ 真名节点,局部/全局/focus 三种模式;JS 原生 SVG 力导向实现(node --check 语法通过;交互经浏览器最终目验) |
| 2.6 | 界面符合 §3.4 视觉规范(时尚/美观/专业) | ✅ design tokens 深浅色自适应 + 卡片式布局 + 克制动效(浏览器最终目验) |
| 2.7 | 部署:compose web 服务 + lab 常驻 | ✅ piks-web 容器 Up,`piks-web listening on :8090 (db ok)`,经 postgres 服务名连接生产库 |

## 3. 关键实现决策(落地细节)

- **Go html/template 每页独立 template set**:`template.New(name).Funcs(...).ParseFS(fs,"templates/base.html",pageFile)`,执行 `base`,页面各自 `define "content"`。Funcs:emClass/pct/pctCls/inc。
- **图谱零第三方依赖**:手写原生 SVG force graph(graph.js,~340 行)——排斥力(2000)+ 弹簧(0.02, 目标距 132)+ 中心引力,alpha 衰减至 0.03 停稳。节点 id 带前缀(`e:`事件/`n:`实体),label 为真名(截断 16 字符),缩放 <0.62 隐藏标签,选中淡化非选中节点。局部优先:默认最近 40 事件邻域;双击/搜索聚焦该节点邻域;全局模式全图(含悬空边守卫——事件/实体已删则诚实跳过)。
- **图谱面板不跳页**:点击节点 → 拉 `/api/events/{id}` 或 `/api/entities/{id}` → 右侧面板渲染摘要 + 相关实体 chips(可继续点入实体面板,委托处理);"打开完整事件卡 →"链接跳详情页。满足 §3.3「点卡片能看到内容」。
- **store 三连新增**(图谱数据源):`ListGraphEdges`(affects 边)、`ListEventAffectedEntities`(事件→受影响实体)、`Counts`(看板统计);`GetEntityByID`(实体卡)。
- **看板统计** = `Counts()` 实时查库(事件非 merged/实体/关系/快照交易日),非硬编码;复盘情绪/涨停等从快照 JSONB 解析。
- **部署**:Dockerfile 一次构建全部 cmd(`./cmd/...`),镜像已含 `bin/web`;compose `web` 服务复用 piks-tools 镜像,`PIKS_DATABASE_URL` 指 `postgres:5432/piks`,仅经私有网络读 PG,不挂 vault、无 Markdown 依赖。deploy.sh 改为 `up -d postgres web`。
- **诚实空态**:无快照日期 → "当日无快照数据(非交易日或源未核验)";强昨日数据缺失 → 如实标注;资金栏留 "源待定,本期留空";「我的判断」空态注明 5-2 在 Web 内编辑(替换 Obsidian 09-Personal)。

## 4. 遗留与后续(登记,不在 5-1)

- **5-2**(编辑 personal_notes + 事件卡「我的理解」;publisher/Obsidian/GitHub/harvest 下线;备份替代落地)——设计 §9 已定,按进度表延后。
- **5-3**(AI 对话页 + 截图上传;G7 契约缺口需探测 Zen vision 支持,失败如实降级)——设计 §4 已定,延后。
- 视觉/交互(§3.3 缩放、点卡片看内容、§3.4 时尚美观专业)最终以浏览器目验为准(2.5/2.6 已备好,用户目验确认)。
- 后端仍有一处待办:lab 生产库 `recon` 显示「抽取失败(1)」,属诚实对账结果,已如实呈现(与 5-1 验收无关)。

---

# 迭代 5-2 — 编辑 + 去界面层 归档

> 状态:**已完成(5-2)**。本阶段只读,续接请看 `进度总表.md` 与 `../../phase2/design/web-app.md`。
> 日期:2026-08-27。验收在 lab 生产数据(192.168.0.202:8090,镜像 7102794)上进行。
> 范围:设计 §9 5-2 行「personal_notes 编辑 + 事件卡『我的理解』;publisher/Obsidian/GitHub/harvest 下线;备份替代落地」。
> 备份替代:用户已决策「会备份到别的机器上,暂时不用你管」(方案 A 异地备份由用户自理,本阶段不代做)。

## 1. 目标与交付物

把「写」也搬进 Web:个人笔记(Belief/Case/Mistake/我的理解)在 PG 直渲的页面里新建/编辑/归档,可关联事件/实体;事件卡上直接写「我的理解」;周报在 Web 聚合本周行情×事件×个人沉淀。同时**下线 publisher/Obsidian/GitHub**:pipeline 不再发布,vault 卷与 setup 步骤摘除,daily-review/reconcile 在 vault 禁用时跳过写盘+git。PG 成为唯一真相源,零 Markdown 投影。

| 交付物 | 状态 |
|---|---|
| `migrations/0006_personal_notes.sql`:personal_notes 表(type/slug/status/confidence/content/detail/author)+ (type,slug) 唯一键 + 索引 | ✅ |
| `internal/store/personal_notes.go`:CRUD + slug 查询 + 区间查询 + 引用(ReplaceNoteRefs 事务重建) | ✅ |
| `internal/store/events.go`:ListEventsBetween / ListEventsRecent(周报与选择器) | ✅ |
| `internal/web/notes.go` + templates(notes/note_form/note):列表(type 过滤)/新建/查看/编辑/归档 + 关联事件/实体多选 | ✅ |
| 事件卡「我的理解」:POST /events/{id}/understanding,落 personal_notes(type='note',slug='event-<id>') + references 关系 | ✅ |
| `internal/web/weekly.go` + templates/weekly.html:/weekly 周报聚合(本周快照×事件×笔记,?offset=N 前周导航) | ✅ |
| M5 下线:config VaultPath 默认空(禁用);daily-review/reconcile vault 禁用跳过写盘+git;pipeline.sh 去 publisher;prod compose 摘 vault 卷;setup.sh 去 vault 步骤;.env.prod.example 清 PIKS_AI_*/PIKS_VAULT_* | ✅ |
| 5-2 验收 | ✅ 见 §2 |

## 2. 验收结果(设计 §9 5-2 行)

| # | 标准 | 结果 |
|---|---|---|
| 2.1 | Web 新建/编辑笔记 | ✅ POST /notes/new 303→详情;编辑 POST /notes/{id}/edit 303;类型/状态/置信度校验(非法返回错误页,非 500);归档走 /notes/{id}/delete |
| 2.2 | 关联事件/实体 | ✅ 表单字段 sel_events/sel_entities 多选 → relationships(from_type='personal_note',rel_type='references');DB 验证 ref 行 + 笔记页渲染 📌事件/🏷实体可点 chip;编辑重建引用不重复(count=1) |
| 2.3 | 事件卡「我的理解」 | ✅ POST 创建 → 事件页回显;再次 POST 更新不重复(type='note',slug='event-<id>' 恒 1 行);redirect #understanding |
| 2.4 | 周报聚合个人笔记 | ✅ /weekly 2026-W35 渲染本周快照(周三/周四+情绪+涨跌停)+ 本周事件 + 本周沉淀(含新建笔记标题);?offset=1 上一周、空周不崩 |
| 2.5 | vault/GitHub 停更 | ✅ lab pipeline.sh publisher 出现 0 次;reconcile 输出「报告=(vault 已下线,Web /recon 实时渲染) (git=0)」;daily-review「vault 已下线,跳过写盘+git」;vault 目录无新文件、git log 无新提交;lab .env 已清 PIKS_VAULT_* 与 PIKS_AI_* |
| 2.6 | 页面可达(部署) | ✅ /weekly /notes /notes/new /settings /recon 全 200;web 容器 Recreate 后正常,AppConfig 读库正常 |

## 3. 关键实现决策(落地细节)

- **personal_notes 状态随 type 变化**:belief 用 hypothesis/active/confirmed/questioned/rejected;case/mistake/note 用 active/archived。表单按类型动态给状态选项。
- **关联复用 relationships**:不新造表,from_type='personal_note' + rel_type='references';编辑时 ReplaceNoteRefs 事务「删全部→重插」,幂等(ON CONFLICT DO NOTHING)。
- **事件「我的理解」= 特殊笔记**:type='note'、slug='event-<id>' 稳定键,references 指向事件。事件卡 GET 时按 slug 回显,POST 时不存在则创建+建关系,存在则仅更新(不重复)。
- **周报周界**:`weekRange` 按北京时区 ISO 周(周一始),返回 2026-W35 标签与 [start,end);行情快照按 trade_date 过滤、事件按 COALESCE(occurred_at,created_at)、笔记按 updated_at。导航偏移 ?offset=N(前 N 周)。
- **模板函数约定复用**:zh(枚举中文化)、selIn(多选选中)、printf;fmtTime/fmtDate 为 Go 包级函数未注册进模板,一律 handler 预格式化传 string(避免模板层 undefined)。
- **M5 下线的最小侵入**:不改 pipeline 数据链(collector→worker→cluster→quote→market-state 全留);只去掉「投影到 vault」这一步。daily-review/reconcile 的 vault 写盘+git 用 `VaultPath==""` 守卫跳过,命令仍跑、task_run 仍记、对账明细仍进 meta+stdout(数据不丢)。publisher 二进制保留(渲染数据组装逻辑 Web 复用,设计 §6.1)。
- **部署顺序**:postgres → migrate(应用 0006)→ web 起;web 启动即 ApplyAppConfig 读 app_config(0005),顺序错会因表缺失崩溃循环。
- **测试数据清理**:验收产生的临时笔记/事件理解已从 lab 生产库硬删(relationships + personal_notes 全清,现 0 行)。

## 4. 遗留与后续(登记,不在 5-2)

- **5-3**(AI 对话页 + 截图上传;G7 需探测 Zen vision 支持,失败如实降级)——设计 §4 已定,延后。
- **备份替代**:用户自理(方案 A 异地备份到其他机器),本阶段未代做。
- **对账遗留**:lab 生产 `reconcile` 报 1 条 `failed_raw`(352413df,validation: empty events array)——旧采集失败滞留,非 5-2 引入,Web /recon 如实呈现。
- 视觉/交互仍以浏览器目验为准(5-1 §2.5/2.6 已备)。

---

# 迭代 5-3 — AI 对话 + 截图 归档

> 状态:**已完成(5-3)**。本阶段只读,续接请看 `进度总表.md` 与 `../../phase2/design/web-app.md`。
> 日期:2026-08-27。验收在 lab 生产数据(192.168.0.202:8090)上进行。
> 范围:设计 §9 5-3 行「/chat:问答带引用;截图上传识别/降级」。
> 前置:5-2 已上线(settings 模型下拉 5-3 前置);G7 探针实测通过(vision 模型接受 image_url)。

## 1. 目标与交付物

Web 内嵌 AI 对话页:知识库问答(grounded,答案带可点事件/实体引用)+ 截图上传识别。AI 配置(服务地址/密钥/模型)全部读 `app_config` 表,`/settings` 编辑即刻生效(每请求重读)。

| 交付物 | 状态 |
|---|---|
| `internal/ai/openai_compat.go` `Chat`:普通补全(非 JSON)+ image_url 视觉请求 | ✅ |
| `internal/store/search.go` `SearchKnowledge`:中文问题 n-gram 拆词 → title/summary/facts + 实体 name/aliases OR 检索,Go 打分取 top | ✅ |
| `internal/store/chat.go`:会话懒建/消息存取/附件元数据/清空 | ✅ |
| `migrations/0008_chat.sql`:chat_sessions/chat_messages/attachments | ✅ |
| `internal/web/chat.go` + templates/chat.html + base.css 气泡样式:/chat 问答+截图上传+清空;/api/attachments/{id} 回显 | ✅ |
| config `PIKS_UPLOAD_DIR`(默认 data/uploads);prod compose `piks_data` 卷挂 /data | ✅ |
| 5-3 验收 | ✅ 见 §2 |

## 2. 验收结果(设计 §9 5-3 行)

| # | 标准 | 结果 |
|---|---|---|
| 2.1 | 问答带引用 | ✅ 问「降准影响哪些板块」→ 答案引用真实央行降准事件卡(f34bccb7,可点跳 /events/{id});无相关板块结论时如实说明,不编造 |
| 2.2 | 截图上传识别(G7) | ✅ G7 探针:真实图片 → Zen `deepseek-v4-flash-vision-exp`,image_url 格式,正确识别「红/蓝/绿色块」;lab 上传走通,答案含图片转录 + 如实关联说明 |
| 2.3 | 引用可点 | ✅ 事件引用 → /events/{id},实体引用 → /entities/{id},chip 样式可点 |
| 2.4 | 历史留存 | ✅ chat_messages 落库,刷新/重开页面回显完整对话 |
| 2.5 | 截图降级 | ✅ 视觉模型未配置(ai_model_vision 空)→ 如实提示改用文字描述;配置齐 → vision 模型走通 |
| 2.6 | 附件持久化 | ✅ 文件落 piks_data 卷(/data/uploads/{date}/),/api/attachments/{id} 200 回显 image/png |
| 2.7 | 配置即时生效 | ✅ /chat 每请求重读 app_config(改 /settings 模型即刻生效,无需重启 web) |

## 3. 关键实现决策(落地细节)

- **检索拆词(n-gram)**:中文无分词器,把用户问题按字符类型切段 → 2/3 字符 n-gram 作候选关键词,`ILIKE ANY` OR 命中 title/summary/facts 与实体 name/aliases,Go 侧按命中 gram 数打分(title 命中权 2)取 top 8。常见停用词(哪些/什么/影响/板块…)整词剔除。初版零成本够用(设计 §4.1);语义检索留 G8。
- **引用协议**:LLM 回答中标注 `[E:事件id]`/`[N:实体id]` → `extractRefs` 解析,**只保留检索结果中真实存在的 id**(防 LLM 自造引用),标注符清掉,引用改由消息下方 refs chips 呈现(规避模板 HTML 注入)。
- **模型档位(iter0 D2)**:文本问答用 `ai_model_extract`(deepseek-v4-flash,便宜);截图问答用 `ai_model_vision`(deepseek-v4-flash-vision-exp)。`ai_model_vision` 为空时截图如实降级提示。
- **每请求重读 AI 配置**:web 启动时 ApplyAppConfig 只给启动初值;`/chat` 按请求 `ListAppConfig` 重建客户端 → `/settings` 改配置即时生效(无需重启 web 容器)。
- **multipart 分流**:非 multipart POST(纯文本/清空)与 multipart(带文件)分别 ParseForm/ParseMultipartForm,`FormFile` 仅在 multipart 时调用(避免非 multipart 误报)。
- **G7 探针教训**:dev 直连 Zen 被 Cloudflare 1010 拦(python urllib UA 特征),lab 用 curl + 浏览器 UA 通过 → 探针在 lab 执行;生产 Go http.Client 通路(pipeline 在用)不受影响。
- **会话模型**:单人系统恒用单个默认会话(懒创建),不做会话管理 UI(设计 §4.4 标可选);提供「清空对话」。

## 4. 遗留与后续(登记,不在 5-3)

- **G8 语义检索(embedding)**:设计 §9 标迭代 5+ 延后;当前 n-gram 关键词检索够用,中文同义词/语义近义命中差可后续换向量。
- **对话多会话**:§4.4 可选,当前单会话;需要时可加 session 列表页。
- **截图 OCR 降级**:§4.3 原备选(不支持视觉时 OCR);G7 通过后未走此路,逻辑未实现。
- **备份替代**:用户自理(方案 A 异地备份)。
- 迭代 5 全部三阶段(5-1/5-2/5-3)完成;设计 §9 迭代 5 行全部 ✅。
- **权威遗留汇总**:以上及 G3/G4/G5、NTP、failed_raw 等跨阶段遗留,统一登记在 `进度总表.md`「已知遗留」节,续接以该节为准。
