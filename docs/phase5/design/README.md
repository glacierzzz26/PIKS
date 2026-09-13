# phase5 定稿设计索引

> 阶段:前端信息架构重组(个股轴心的个人投研平台)。已定稿设计在此登记,此后按它执行,改动需走变更。

## 前端 IA 重组 — 个股轴心 + 自选股截图同步 ✅ 已实现(dev)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [frontend-ia.md](./frontend-ia.md) | ✅ **已实现(dev,Phase 1–4)** | 2026-09-13 | 把 Web IA 从「按后端产物类型平铺」重组为**以个股为轴心**:首页=自选列表(`/`),`/stock/:code` 个股中心一站看全,**市场数据迁 `/market`**;**引入自选股**(复用 `entities.status` 三态,零迁移)并以**同花顺自选截图镜像同步**(复用交易截图两段式,`kind='watchlist'`);导航移除对账页侧栏位(降至设置子入口)。已修 `UpsertEntity` 空 Status 地雷(§2.6.1)。实现情况与偏差见文档 §7。**只做 dev 验证不部署 lab**。 |

## 待办

- [x] Phase 1 个股中心(聚合端点 `/api/v1/stock/:code` + `/stock/:code` 页 + 导流)
- [x] Phase 2 自选(`UpsertEntity` 地雷修复 + `/` 自选首页 + 市场概览条;硬验收:跑 entity-build 后 watch 仍在 ✓)
- [x] Phase 3 自选截图镜像同步(ImportFlow 拆分 + watchlist 分支)
- [x] Phase 4 收口(全面导流 + CLAUDE.md/进度总表更新)
- [ ] (后续)部署 lab(镜像重建生效)
- [ ] (候选)实体库加「已移出自选」灰标;多图自选截图支持
