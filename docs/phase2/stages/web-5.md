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
