# PIKS Frontend

PIKS 前端（Next.js 重构版）：只读 PostgreSQL 投影数据的专业化 Web 界面。

## 技术栈

- Next.js 14（App Router）+ React 18 + TypeScript
- Tailwind CSS（设计令牌见 `tailwind.config.ts`，与《PIKS 前端重构规范》一致）
- ECharts（连板梯队 / 行业分布）
- React Flow（@xyflow/react，实体关系图谱）
- TanStack Table（事件表 / 涨停池表）
- Framer Motion（抽屉与过场动画）
- lucide-react（线性图标）

## 运行

```bash
npm install
npm run dev    # http://localhost:3000

# 生产
npm run build && npm start
```

## 数据源

`NEXT_PUBLIC_API_BASE_URL`（默认 `http://localhost:8090/api/v1`）指向 Go 后端 `cmd/web`。
后端未连接时自动降级为内置演示数据，界面会显示「演示数据」徽章 —— 数据诚实，宁缺毋假。

### REST 端点约定（只读投影，供后端对齐实现）

列表端点统一支持分页：`page`（1 起）、`size`（默认 20，可选 50/100）。

| 端点 | 说明 | 查询参数 |
|---|---|---|
| `GET /events` | 结构化事件流 | `type` `status` `q` `from` `to` `page` `size` |
| `GET /entities` | 统一实体 | `type` `q` `page` `size` |
| `GET /relationships` | 实体关系 | — |
| `GET /market/snapshot` | 市场状态快照（含涨停池） | `date` |
| `GET /flashes` | 快讯流（raw_documents 投影） | `q` `source` `page` `size` |
| `GET /notes` `GET /notes/:id` | 文档（复盘/笔记/周报 Markdown） | `type` `page` `size` |

## 页面模块

- `/` 事件流（核心）：市场速览 + 类型/状态筛选（URL query 可分享）+ 表格 + 详情抽屉
- `/entities` 实体库：列表 + 详情 + React Flow 关系图谱（聚焦邻域）
- `/ladder` 涨停梯队：情绪头图 + 连板阶梯图 + 行业分布 + 涨停池表
- `/flashes` 快讯流：按日分组时间线，重要快讯高亮
- `/docs`、`/docs/[id]` Markdown 文档阅读

## 已实现的规范约束

- 涨红跌绿（A 股习惯）；数字等宽 + tabular-nums + 右对齐（`Num` 组件）
- 全部筛选状态写入 URL query；全局 ⌘K 命令面板（页面 / 实体 / 命令）
- 异步操作 loading / error / empty 三态；核心数字区不使用 skeleton loader
- 组件单文件 ≤150 行；逻辑 >50 行抽 hook（`useUrlState` / `useData`）
- 非对称栅格（8/4、5/7、7/5），无 3 栏等宽
