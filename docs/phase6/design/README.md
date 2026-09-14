# phase6 定稿设计索引

> 阶段:前端决绝重构 + 投研闭环补齐(P6)。已定稿设计在此登记,此后按它执行,改动需走变更。

## P6 前端决绝重构 + 投研闭环补齐 ✅ 已定稿(2026-09-14)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [ux-ia.md](./ux-ia.md) | ✅ **已定稿** | 2026-09-14 | 用户诉求:专业、简约、清爽、**新手友好**(引导式首页 + 白话导航 + 隐藏管线内部)、补齐 epic #1 闭环。**决绝重构**(重建呈现层:设计系统 + IA + 页面,移植数据层与业务逻辑——非字面从零重写);后端允许新端点/新表(P6 用零 schema 方案);先设计定稿再分阶段。五大阶段见 §7。定稿门结论见文档抬头。 |

## 待办

- [x] 定稿门:设计经用户过目 → 定稿(2026-09-14)
- [x] P6-1 地基与诚实(设计系统重建 + 5 处数据诚实/死重修复)★ 最先 —— 见 [../stages/p6-1-foundation.md](../stages/p6-1-foundation.md)
- [x] P6-2 白话导航 + 隐藏管线 + 词汇表 + 文案(含修 e2e 冒烟脚本) —— 见 [../stages/p6-2-plain-nav.md](../stages/p6-2-plain-nav.md)
- [x] P6-3 引导式首页 + 自选富化(后端 `/watchlist` 富化) —— 见 [../stages/p6-3-guided-home.md](../stages/p6-3-guided-home.md)
- [x] P6-4 决策记录闭环(研究→决策→持仓;图谱过滤同 commit) —— 见 [../stages/p6-4-decision-loop.md](../stages/p6-4-decision-loop.md)
- [x] P6-5 复盘→沉淀 + 研究 delta + 收尾 —— 见 [../stages/p6-5-reflection-delta.md](../stages/p6-5-reflection-delta.md)
- [ ] 部署 lab(P6 全部完成后)

## 前序阶段

- **P5**(`docs/phase5/design/frontend-ia.md`):个股轴心 IA,已上生产(镜像 `c81c6d2`,2026-09-13)。P6 在其骨架上决绝重构呈现层,保留其深链与个股中心语义。
