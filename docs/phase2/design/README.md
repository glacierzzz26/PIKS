# phase2 定稿设计索引

> 阶段:增值(迭代 2 + 迭代 3 + 迭代 4 + 迭代 5 Web 平台)。已定稿设计在此登记,此后按它执行,改动需走变更。

## 交易功能 — 截图识别录入 + 知识库带引用解读 ✅ 已定稿冻结(2026-08-28)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [trades.md](./trades.md) | ✅ 已定稿冻结 | 2026-08-28 | 用户新需求:每日自交易入知识库并由库解读、库缺则补数据;录入=同花顺今日交易/持仓截图 → vision 抽取 → 预览确认入库(migration 0010 trades+positions)+ 手动兜底;`StructuredOutput` 加 Image(探针验证);实体自动补全(已有复用);解读带引用 + 防未来函数;mistake 用户确认存笔记;**只做 dev 验证不部署 lab** |
| [../stages/trades.md](../stages/trades.md) | ✅ 已归档 | 2026-08-28 | 验收 design §4 十三项全过(含降级/记账/防未来函数实测);修 2 bug(positions 反序列化丢 cost/mv、mistake 表单嵌套 range 模板崩溃) |

## 周报 AI 综述 — Web 适配 iter4 D26 ✅ 已定稿冻结(2026-08-28)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [weekly-ai-summary.md](./weekly-ai-summary.md) | ✅ 已定稿冻结 | 2026-08-28 | 兑现 iter4 D26 可选「高智档综述」到 Web `/weekly`:新表 `weekly_summaries` 按 ISO 周缓存 + 手动按钮触发生成(GET 永不调 LLM);模型 reasoning 回退 extract + 预算护栏 + task_runs 记账;未配置/预算/无数据/失败如实降级;**只做 dev 验证不部署 lab** |

## 聚类质量 — 重审视 Pass ✅ 已定稿冻结(2026-08-28)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [cluster-quality.md](./cluster-quality.md) | ✅ 已定稿冻结 | 2026-08-28 | 修复跨簇重复盲区(降准两簇实测);重审视 Pass 并入 `cmd/cluster`;复用迭代 1 阈值不降门槛;零迁移;**只做 dev 验证不部署 lab** |

## G8 — /chat 语义检索 ✅ 已定稿冻结(2026-08-28,方案 B)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [g8-semantic-retrieval.md](./g8-semantic-retrieval.md) | ✅ 已定稿冻结 | 2026-08-28 | 探针:Zen 无 embeddings 端点(404);方案 A(Ollama)评估后否决,**选方案 B LLM 同义扩展**;零迁移零新服务;**只做 dev 不升级生产** |

## 迭代 5 — Web 平台(替换 Obsidian/GitHub 界面层)🔵 5-1 完成

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [web-app.md](./web-app.md) | ✅ 已定稿冻结 | 2026-08-27 | PG 直渲 Web:看板/事件/实体/复盘/对账/图谱(缩放·点选看内容·局部优先);§3.4 时尚美观专业视觉规范;5-1 只读 + 5-2 编辑去界面层 + 5-3 AI 对话截图 |
| [../stages/web-5.md](../stages/web-5.md) | ✅ 5-1 归档 | 2026-08-27 | 验收 §2 七项全过(真实生产数据);cmd/web + 8 页 + 原生 SVG 图谱;部署 lab:8090;5-2/5-3 延后 |

## 迭代 4 — 个人学习闭环 ✅ 已定稿冻结(实现并入迭代 5)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [iter4-learning-loop.md](./iter4-learning-loop.md) | ✅ 已定稿冻结 | 2026-08-27 | Belief/Case/Mistake 单表 personal_notes + 单向 harvest 收割器(G6)+ 周报(规则聚合 + 高智档综述);lab clone PIKS-Personal 同步(权威源翻转为 PG,5-2 Web 内编辑) |
| [../stages/iter4.md](../stages/iter4.md) | ⬜ 待实现归档(并入迭代 5) | — | — |

## 迭代 2 — 市场情报 ✅ 已完成并冻结

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [iter2-market-intel.md](./iter2-market-intel.md) | ✅ 已定稿冻结 | 2026-08-26 | 每日复盘 12 项、observations 填充、情绪规则模型、02-Market 聚合页;G2 东财行情 |
| [../stages/iter2.md](../stages/iter2.md) | ✅ 归档 | 2026-08-26 | 验收 §5 八项全过;AI 调用=0;push2 源待核验 |

## 迭代 3 — 实体补全 ✅ 已完成并冻结

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [iter3-entities.md](./iter3-entities.md) | ✅ 已定稿冻结 | 2026-08-26 | entities 单表 + 实体构建(种子源零 AI + affected 便宜档分类)+ 实体卡 + hot_topics 补链 |
| [../stages/iter3.md](../stages/iter3.md) | ✅ 归档 | 2026-08-27 | 验收 §5 七项全过;90 实体卡 + affected wikilink;重跑零提交 |

## 待办

- [x] 迭代 2 定稿门确认(D12~D17,用户确认:东财源 + 只做市场情报)
- [x] 迭代 2 实现与验收(§5 八项)
- [x] 归档 `../stages/iter2.md`
- [x] 迭代 3 定稿门确认(D18~D21,用户确认:单表 entities / 便宜档分类 / 仅叶子行业 / G3 延后)
- [x] 迭代 3 实现与验收(§5 七项)
- [x] 归档 `../stages/iter3.md`
- [x] G8 定稿门确认(方案 B:LLM 同义扩展,2026-08-28;探针 Zen 无 embeddings 端点)
- [x] G8 实现与验收 + 归档 `../stages/g8.md`(dev-only 不升级生产)
- [x] 聚类质量定稿门确认(重审视 Pass,2026-08-28;复用迭代 1 阈值不降门槛)
- [x] 聚类质量实现与验收 + 归档 `../stages/cluster-quality.md`(dev-only 验证不部署 lab)
- [x] 周报 AI 综述定稿门确认(Web 适配 D26,2026-08-28;缓存表 + 手动触发 + 预算护栏)
- [x] 周报 AI 综述实现与验收 + 归档 `../stages/weekly-ai-summary.md`(dev-only 验证不部署 lab)
