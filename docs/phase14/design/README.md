# phase14 定稿设计索引

> 阶段：**采集收口 —— 全部数据交给 datahub**（把 PIKS 自建的采集链路全部迁入独立取数服务 datahub，PIKS 退化为**纯消费方**）。
> 已定稿设计在此登记，此后按它执行，改动需走变更。

## P14-0 采集收口方案 📝 草案 v0.1，待定稿（2026-10-13）

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [datahub-collection.md](./datahub-collection.md) | 📝 **草案 v0.1，未定稿** | 2026-10-13 | 承接用户诉求「**数据采集全部交给 datahub**，需要获取数据就调 datahub 的 HTTP 或 MCP 完成；**datahub 负责维护所有数据**」。PIKS 现有 4 个 Go 采集入口（快讯 6 源 / 公告 / 热榜 / 涨停·指数）+ 研报 akshare 按需取数 **全部迁入 datahub**；自选（凭据类）**留 PIKS**；事件原文 **datahub 完全权威**。 |

### 本轮拍板（决策记录，见正文 §0）

- 🔴 **D1 接管范围 = 全交**：快讯 6 源 / 公告 / 热榜 / 涨停·指数 / 研报取数全部交 datahub；**datahub 负责维护所有数据**。
- 🔴 **D2 事件原文 = datahub 完全权威**：PIKS 不再作为原文的独立真源（正文 §4.3 给出「工作镜像 vs 零副本」子决策）。
- 🔴 **D3 自选留 PIKS**：`cmd/watch-sync` 与 `ths_*` 凭据**不迁**（凭据类源不进 datahub）。
- **D4 出口**：HTTP 为真实契约、MCP 为薄门面（沿用 datahub 现状）。
- **D5 迁移**：拒绝 big-bang，**逐数据集闸门灰度 + 对账 + 可单步回滚**。
- **D6**：定稿前不动业务代码（承接 phase13 纪律）。

### 与既有设计的关系

- **datahub 侧上位设计**（steady 仓）：`steady/docs/phase2/design/数据接入层-datahub.md`（总设计）、`…-phase2.md`（采集接管）、`…-phase3.md`（读切换）。本方案把「采集唯一出口 = datahub」的**边界从 steady 扩到整个生态（+PIKS）**。
- **phase13（研究平台）**：phase13 定的「STEADY 经 MCP 接入（唯一）」其载体正是 datahub 的 MCP 出口——本方案**直接交付 phase13 的数据底座**（正文 §2.3、§3.4）。
- **phase11（数据源分层）/ epic #83（事件管线）**：采集迁走，**派生链（worker/cluster/entity/market-state）不动**（正文 §4）。

## 阶段序列（P14）

| 分期 | 内容 | 状态 | 交付物 / 验收 |
|---|---|---|---|
| **P14-0** | 方案定稿（本文档） | 📝 草案 v0.1 | 过定稿门 |
| **P14-1** | 研报数值取数切 datahub（替 akshare） | ⬜ 未开始 | 研报取数与旧链路逐位一致 |
| **P14-2** | 涨停池 / 指数观测切 datahub | ⬜ 未开始 | `observations` 12 指标一致 |
| **P14-3** | 事件原文（快讯 6 源 + 公告）迁入 datahub | ⬜ 未开始 | 逐源对账零偏差；反封禁不变量不丢 |
| **P14-4** | 热榜 2 源迁入 datahub | ⬜ 未开始 | 两源逐条一致、`rank` 不重排 |
| **P14-5** | 保留期清理迁 datahub + 删 PIKS 采集代码（收口） | ⬜ 未开始 | 采集唯一出口 = datahub |

## 前序阶段

- `docs/phase11/design/`：数据源分层（A/B/C/D）+ 事件管线 epic #83（P-1~P-5）。
- `docs/phase12/design/access-control.md`：鉴权（#78）。
- `docs/phase13/design/research-platform.md`：研究平台重构（本方案交付其数据底座）。
