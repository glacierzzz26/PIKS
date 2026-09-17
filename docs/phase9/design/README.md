# phase9 定稿设计索引

> 阶段：研报体裁与专业版面（呈现层）。已定稿设计在此登记，此后按它执行，改动需走变更。

## P9 研报体裁与版面 ✅ 已定稿（2026-09-16）

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [report-layout.md](./report-layout.md) | ✅ **已定稿** | 2026-09-16 | 用户诉求：研报与个股分析**在页面上拆出来**、公司/行业研报要**专业**、排版格式专业化。把「研报」从「个股深研 run 的仪表盘」中拆出为**独立体裁**（`/reports`，单栏正文 + 左侧目录 + 三域标记），三类研报（行业 #12 / 公司 #11 / 宏观 #13）共用一套版面，差异由 `subject_type` + 章节清单驱动。**零 schema**（章节清单走 `metrics.meta.section_manifest`）。硬前置 = 主体轴泛化 #10；本次落点 = #12 行业研报。 |
| [company-analysis-split.md](./company-analysis-split.md) | 🟡 **待定稿门** | — | issue **#11**：按**功能**把「公司研报」与「个股分析」拆开（用户 2026-09-17「考虑功能上的拆分」）。公司研报=季频文档体裁（`/reports`），个股分析=日频速览（`/research`）。调研发现量价是**承重墙**（risk/scorecard 依赖）→ 决策「改风险引擎」让基本面规则脱离量价独立运行；另发现机检 Evidence 缺口（无 bars 即空，同 P9-3 坑）。**⚠️ 硬阻塞**：dev 缺 P7（`prebuy`/`patterns`），见其 §6.1。 |

## 实施阶段（P9-1 → P9-3）

| 阶段 | 内容 | 依赖 | 交付物 |
|---|---|---|---|
| **P9-1** 主体轴泛化 | Go：`SubjectTypeOf` 判别 + `ValidStockCode`→主体感知 + `ToFullCode` 行业码不加前缀；Python：`resolve_symbol` SI 分支。**前端零改动** | — | #10 |
| **P9-2** 研报版面 | research：摘要前置（`markdown.py`）+ `section_manifest`（`json_report.py`）+ 估值口径硬约束（`synthesis.py`）；Go DTO 加 `subject_type`/`display_name`；TS 类型；`/reports` 路由 + `.report-*` 语义类 + 目录/三域标记；导航 11→12 | P9-1 | 本版面 |
| **P9-3** 行业数据管线 | Python：申万行业指数 provider（层级自适应）+ `analysis/industry.py`（分位/回撤/横截面）+ `profiles/industry.yaml` + 行业 markdown/json 节 | P9-1 | #12 |

> P9-2 与 P9-3 **可并行**（接口 = `section_manifest` + `subject_type`）；P9-1 是两者的硬前置。

## 待办

- [x] **定稿门**：设计经用户过目 → 定稿（2026-09-16）
- [x] **P9-1** 主体轴泛化（#10）—— PR #15 已合并
- [x] **P9-3** 行业数据管线（#12）—— PR #15 已合并
- [ ] **P9-2** 研报版面（`/reports` + 三域标记 + 章节清单 TOC）—— 已实现，待人工审核合并
- [x] 导航：研究组加「研报 `/reports`」，`/research` 改标签「个股分析」（11 → 12 项）
- [ ] 部署 lab

## 前序阶段

- **P6**（`docs/phase6/design/ux-ia.md`）：前端决绝重构，已上生产（镜像 `9454524`）。本阶段在其设计系统与导航骨架上，把研报拆成独立体裁。
- **P4**（`docs/phase4/design/research-merge.md`）：research 深研并入。本阶段改的是它产出的**呈现**，不动其确定性计算。
