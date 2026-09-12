# phase4 定稿设计索引

> 阶段:能力并入(research 深研并入 PIKS + 构建优化)。已定稿设计在此登记,此后按它执行,改动需走变更。

## research 并入 — 单仓多运行时 + Go 去 vendor ✅ 已定稿冻结(2026-09-12)

| 文档 | 状态 | 定稿日期 | 备注 |
|---|---|---|---|
| [research-merge.md](./research-merge.md) | ✅ **已定稿冻结** | 2026-09-12 | 把 `../investment-research`(Python 6562 行/5 纯数据依赖/无自有 LLM runtime)并入 PIKS 成为个股深研能力:同仓多运行时(`research/` 目录 + `cmd/research-run` 编排)、**删除其 SQLite 存储层**(PG 为唯一归档)、LLM 合成复用 PIKS `ai.Provider`(extract 档)、`synthesize` 的 Number Lint 作第二道防幻觉闸门;新增 `research_runs` 表(0012)+ 三个 `/api/v1` 端点 + 深研报告页(Fact/Opinion 分区);**同时移除 Go vendor**(8.2MB/280 文件,1 直接依赖 → go.sum + 模块代理)。**D-2 = 方案 B(同仓双镜像)**;**D-11 = research 支持独立迭代**(硬约束:§4.10 零共享状态 / 产物契约版本化 / 依赖与构建隔离 / 接口冻结 + 最小版本 fixture 测试,§5.6 独立验收)。**只做 dev 验证不部署 lab** |
