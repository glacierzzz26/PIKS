# PIKS 前端重构规范

## 项目背景
PIKS 是 A 股投资知识系统：快讯/涨停池 → 结构化事件与实体 → PostgreSQL → Markdown/Obsidian。
前端全量 React：只读页消费 PostgreSQL 投影数据；交互页（笔记/周报/交易/AI 对话/设置）经 `/api/v1` JSON 写接口操作业务。

## 技术栈（如已确定）
- 构建：Vite 5 + React 18 + TypeScript（纯客户端 SPA，静态产物 `frontend/dist/`）
- 路由：React Router v6（客户端路由；筛选状态写 URL query）
- 服务：nginx（生产网关，单入口 :8090）—— 服务 SPA 静态文件 + 反代 `/api/*`
- 后端：Go web 只监听 `127.0.0.1:8090`（与 nginx 同容器），不直接暴露局域网
- 样式：Tailwind CSS
- 图表：ECharts 6
- 关系图谱：React Flow
- 表格：TanStack Table
- 动画：Framer Motion
- 数据获取：REST API，base URL 默认相对 `/api/v1`（生产同源走 nginx、开发经 vite proxy），可用 `VITE_API_BASE_URL` 覆盖

## 设计令牌（Design Tokens）
### 配色
- 背景：#F7F8FA
- 卡片：#FFFFFF
- 边框：#E5E7EB
- 主色：#003366
- 涨红（A 股习惯）：#E24B4A
- 跌绿（A 股习惯）：#1D9E75
- 文字主色：#1A1A1A（禁止纯黑 #000000）

### 字体
- 界面：Inter / PingFang SC
- 数字：等宽字体 + tabular-nums + 右对齐

### 尺寸
- 圆角：6px
- 阴影：0 2px 8px rgba(0,0,0,0.04)，禁止重阴影
- 表格行高：44-52px
- 间距：8px 网格基准
- 触摸目标最小：44x44px

## 强制规则
1. 涨跌色必须是 A 股习惯（涨红跌绿），禁止欧美配色
2. 数字必须等宽、右对齐、tabular-nums
3. 图标统一用 Lucide / Heroicons 线性图标，禁止 emoji
4. 布局用非对称栅格，禁止 3 栏等宽
5. 组件单文件不超过 150 行，JSX 超 80 行必须拆子组件
6. 函数逻辑超 50 行必须抽 custom hook
7. 所有筛选状态必须反映到 URL query（可分享）
8. 全局 ⌘K 命令面板：跳转实体 / 切换页面 / 执行 cmd
9. 所有异步操作必须处理 loading / error / empty 三态
10. 禁止紫粉渐变、禁止 playful 字体、禁止 AI 套话文案

## 页面模块（2026-08-30 起：全部页面由 React SPA 提供，无 Go HTML）
- **全部页面（`frontend/src/pages/`，React Router 注册）**：看板 / 事件流（含 `/events/:id` 详情抽屉兜底）/ 实体库（含 `/entities/:id` → `?id=` 重定向）/ 图谱 / 涨停梯队 / 快讯流 / 笔记（列表 + 新建 `/notes/new` + 阅读 `/notes/:id` + 编辑 `/notes/:id/edit`）/ 周报（周导航 + AI 综述生成）/ 对账 / 交易（手动录入 + 截图导入 + AI 解读 + 组合诊断）/ 复盘 / AI 对话 / 设置
- 写操作经 `/api/v1` JSON 写接口（`internal/web/api_write.go`）；nginx 仅反代 `/api/*` + 服务 SPA 静态文件，无交互页反代
- 分流规则见 `configs/nginx.conf`（生产）与 `frontend/vite.config.ts`（dev proxy 复刻）

## 禁用清单
- 禁止直接改 PostgreSQL schema（前端重构不涉及）
- 禁止引入与现有栈冲突的 UI 库
- 禁止破坏性 API 变更
- 禁止在核心数字区使用 skeleton loader
