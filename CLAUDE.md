# PIKS 前端重构规范

## 项目背景
PIKS 是 A 股投资知识系统：快讯/涨停池 → 结构化事件与实体 → PostgreSQL → Markdown/Obsidian。
前端只读 PostgreSQL 投影数据，不直接写业务逻辑。

## 技术栈（如已确定）
- 框架：Next.js（App Router）+ React 18 + TypeScript
- 样式：Tailwind CSS
- 图表：ECharts 6
- 关系图谱：React Flow
- 表格：TanStack Table
- 动画：Framer Motion
- 数据获取：REST API，base URL 通过环境变量配置

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

## 页面模块
重构按以下顺序推进，每完成一个模块提交一次：
1. 全局布局 + 左侧导航 + 顶部筛选栏
2. 事件流页面（核心）
3. 实体库页面（含关系图谱）
4. 涨停梯队页面
5. 快讯流页面
6. Markdown 文档阅读页

## 禁用清单
- 禁止直接改 PostgreSQL schema（前端重构不涉及）
- 禁止引入与现有栈冲突的 UI 库
- 禁止破坏性 API 变更
- 禁止在核心数字区使用 skeleton loader
