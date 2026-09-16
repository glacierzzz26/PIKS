# phase9 定稿设计索引

> 阶段：研报体裁与专业版面（呈现层）。已定稿设计在此登记，此后按它执行，改动需走变更。

## P9 研报体裁与版面 ✅ 提案（待定稿门，2026-09-16）

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [report-layout.md](./report-layout.md) | 🟡 **提案，待定稿门** | — | 用户诉求：研报与个股分析**在页面上拆出来**、公司/行业研报要**专业**、排版格式专业化。把「研报」从「个股深研 run 的仪表盘」中拆出为**独立体裁**（`/reports`，单栏正文 + 左侧目录 + 三域标记），三类研报（行业 #12 / 公司 #11 / 宏观 #13）共用一套版面，差异由 `subject_type` + 章节清单驱动。**零 schema**（章节清单走 `metrics.meta.section_manifest`）。硬前置 = 主体轴泛化 #10；本次落点 = #12 行业研报。 |

## 待办

- [ ] **定稿门**：设计经用户过目 → 定稿（2026-09-16）
- [ ] 主体轴泛化（#10）：`subject_type` DTO 判别 + `ValidStockCode` 主体感知 + `resolve_symbol` SI 分支
- [ ] 版面实现：`/reports` + `/reports/:runId` + `.report-*` 语义类 + 章节清单驱动 TOC
- [ ] #12 行业研报数据管线（Python：申万指数/估值/成分 + `industry` profile + 估值横截面口径）
- [ ] research 侧：摘要前置（`markdown.py`）+ `section_manifest` 产出（`json_report.py`）+ 估值口径硬约束（`synthesis.py`）
- [ ] 导航：研究组加「研报 `/reports`」，`/research` 改标签「个股分析」（11 → 12 项）
- [ ] 部署 lab

## 前序阶段

- **P6**（`docs/phase6/design/ux-ia.md`）：前端决绝重构，已上生产（镜像 `9454524`）。本阶段在其设计系统与导航骨架上，把研报拆成独立体裁。
- **P4**（`docs/phase4/design/research-merge.md`）：research 深研并入。本阶段改的是它产出的**呈现**，不动其确定性计算。
