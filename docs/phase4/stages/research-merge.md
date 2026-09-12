# 能力并入 P4:research 个股深研并入 PIKS(实现与验收归档)

> 子阶段:research 并入实现与验收(2026-09-12)
> 前置:`docs/phase4/design/research-merge.md` 已定稿冻结(D-1~D-11)
> 状态:✅ 已落地(dev-only;未部署 lab —— D-8)

## 0. 落地范围

把独立的 `investment-research`(Python 个股深研 agent)并入 PIKS 同仓,一期实现 T1~T7:

| 任务 | 交付物 | 状态 |
|---|---|---|
| T1 | Go 去 vendor(gosu 模块代理)+ Dockerfile 依赖层 | ✅ |
| T2 | `research/` 迁移 + 删 SQLite storage + CLI 三命令改造 | ✅ |
| T3 | `migrations/0012_research_runs.sql` + `internal/store/research_runs.go` | ✅ |
| T4 | `internal/research` 编排 + `cmd/research-run` | ✅ |
| T4.5 | Dockerfile `research` 独立 target + compose 入口 | ✅ |
| T5 | `internal/web/api_research.go` 三端点 | ✅ |
| T6 | 深研报告页 + 实体/持仓入口 | ✅ |
| T7 | 独立迭代验收(fixture 测试 + 演练)+ 归档 | ✅ |

`investment-research` 保留为**只读归档**(不删,历史可回溯)。

## 1. 架构落地(D-2 同仓双镜像 / D-6 os/exec 编排)

```
┌─ piks-tools(主镜像)──────────────┐   ┌─ piks-research(深研镜像)─────┐
│ nginx 网关 + Go bin + React dist │   │ python:3.12-slim + research/ │
│ 路由 /api/v1 只读 + 写接口        │   │ + bin/research-run(ENTRYPOINT)│
└──────────────────────────────────┘   └──────────────────────────────┘
        ▲                                        ▲
        │ 读 research_runs(PostgreSQL)           │ exec `python -m src.cli <cmd>`
        │                                        │ 读/写产物文件(冻结契约)
        └──────────── Go 编排(internal/research)─┘
```

- **数字唯一源 = PostgreSQL**:research 不再持有 SQLite(storage 层已删),产物经 Go 落 `research_runs`。
- **编排不引入 FastAPI**(D-6):Go 用 `os/exec` 调 Python CLI 三命令,零胶水进程。
- **Fact/Opinion 分域**:`metrics`(确定性计算)= Fact;`synthesis`(LLM 三段)= Opinion;`evidence` = Fact 溯源链。

## 2. 数据面

### 2.1 迁移 `0012_research_runs.sql`

`research_runs` 一行 = 一次深研快照;`run_id` UNIQUE(幂等键);索引 `(code, as_of DESC)` + `status`。

### 2.2 存取层 `internal/store/research_runs.go`

- `NormalizeCode`:research full_code(`sh600519`)→ PIKS 6 位(`600519`),与 `entities.detail->>'code'` 对齐(§4.6 join 的根)。
- `CreateResearchRun` 幂等(`ON CONFLICT DO NOTHING RETURNING`,返回 `created bool`)。
- `SaveResearchArtifacts` 全 JSONB 用 `COALESCE`,nil 字段不覆盖已落值(分阶段写入的关键)。
- `FinishResearchRun` 一次性收口 status + error + gate。

### 2.3 编排状态机(§4.4)

```
pending → gathering → synthesizing → verifying → done
                                                     ↘ failed(任一步失败,error 原文如实落库)
```

断点重跑:产物目录绑定 run_id,已产出的步骤跳过(`artifacts.exists`);`--force` 才清空。

## 3. 接口面

`GET /api/v1/research-runs?code=|entity=|limit=` · `POST /api/v1/research-runs` · `GET /api/v1/research-runs/:runId`。
POST 同步建 pending 行并立即返回 `{run_id,status}`(202),编排在后台 goroutine(总超时 `TimeoutTotal=6m`);前端轮询 GET 取状态。

`apiEntity` 增 `code` 字段(取 `entities.detail.code`,`NormalizeCode` 归一),前端据此决定是否给「深研」入口 —— 行业/概念实体无个股深研。

## 4. 前端(§4.8)

- `pages/research.tsx`:`/research/:runId` 只读报告页,Fact / Opinion / 机检 三分区。
- `components/research/`:ReportHeader(机检徽标)/ FactSection / OpinionSection / GatePanel(折叠)/ DeepResearchButton;子组件 MetricGroup·EventGroup·Scorecard·metricRows 拆分,单文件均 ≤150 行。
- `hooks/useResearchRun.ts`:触发 + 轮询(>50 行逻辑抽 hook);换股票/卸载即 abort。
- 入口:实体卡(仅带 `code` 的公司实体)+ 持仓行(新增「深研」列)。

## 5. 验收结果

### 5.1 vendor 移除 ✅
- `go build/vet/test ./...` 全过(`go.sum` 校验生效);`vendor/` 已出仓。
- Dockerfile 依赖层独立(`go mod download && go mod verify`),改业务代码不重下依赖。

### 5.2 Python 并入 + 存储层归并 ✅
- `research/` 就位,`storage/` 已删;CLI 三命令(`research`/`synthesize`/`gate`)不再碰 SQLite。
- 修两处并入暴露的缺陷:① `requirements.txt` 漏 `PyYAML`(新 venv 必崩);② `report/markdown.py` 漏 `Any` 导入(Python 3.12 即时求值注解 → `NameError`,3.14 惰性求值掩盖)。

### 5.3 编排 + 落库 ✅
- `bin/research-run 000560 --profile short-term` 端到端 `done`;`research_runs` 行 status=done + metrics/synthesis/markdown/lint/gate 齐全;`task_runs` 三条(gather/synth/verify)带 `ai_tokens`。
- 幂等:同 run_id 重跑不增行;`--force` 新档保留旧档。
- 失败注入(429/采集失败)→ status=failed + error 原文,无编造数据(实测 429 如实落库)。
- 机检故意失败(mock 注入编造数字 `987.65`)→ Number Lint 命中「未在指标卡中找到出处」,markdown **回落确定性骨架报告**,问题清单原样落 lint JSONB。

### 5.4 前端 ✅
- 报告页四区全渲染、机检详情可展开;实体卡与持仓行按钮就位;点「深研」→ 内联「采集中」徽标(文字,非骨架屏)→ 自动跳报告页。
- 验证方式:Vite dev + Playwright(1400px)渲染,console 无报错。
- `tsc --noEmit` + `npm run build` 通过;新增组件文件均 ≤150 行。

### 5.5 契约 ✅
- 三端点 curl 全 200,形状对齐前端 `types.ts`(17 字段逐一对齐)。
- `code` join:实体 → 报告列表经 `entity` 参数命中(`detail->>'code'` 归一验证)。
- CORS 头正常(复用现有中间件);**prod 未动**:无 lab 部署、prod DB 无新迁移(D-8)。

### 5.6 research 独立迭代(D-11 硬验收)✅
- **零共享状态**:`scripts/check-research-isolation.sh` 入选 CI —— Go/前端只经「CLI 参数 + 产物契约」交互;禁止 `go:embed research`、读 `.py` 源、前端引用 research 源码。
- **构建隔离**:`docker build --target research` 只跑 Python 阶段,日志无 golang/node;反向 `--target tools` 的 Python 阶段因本机无 buildx 无法跳过(已如实标注,产物隔离不受影响 —— 两镜像内容实测互不含对方运行时)。
- **契约升版保护**:`contract: 2` fixture → Go 明确报错「research 产物契约 v2 高于本端支持的 v1,请升级 PIKS」,不崩溃不硬解;`contract: 1` 与缺字段均正常 `done`。
- **最小版本测试**:`internal/research/artifacts_test.go` 用固定产物 fixture,`go test ./internal/research/...` **不 exec Python**(CI 可跑);完整状态机测试(注入 fakeCLI + mock provider)在 DB 集成开关下跑通 `pending→done`。
- **Python 独立测试**:`pytest research/tests` 71 项在 `research/` 子目录内独立通过(不依赖 PIKS)。
- **端到端独立迭代演练**:见 §6。
- **产物契约冻结**:五类产物文件名与 `research/README.md` 契约表一一对应(测试断言 + CI 校验)。

## 6. 独立迭代演练(D-11 核心验证)

目标:证明「改 Python 只需重建 `piks-research`,Go 与前端零接触」。

1. 改动 `research/` 内一处 Python 代码;
2. `docker build --target research -t piks-research:test .` —— 构建日志中**无 golang/node 阶段**;
3. 冒烟:该镜像内 `research-run` 跑通;
4. Go 二进制与前端 dist **未重建**,报告页仍正常。

结论:独立迭代成立。唯一耦合面是**冻结的产物契约 + CLI 参数**(§4.10 G4),契约加法变更即向前兼容(前端可先不展示新字段)。

## 7. 已知边界与后续

- **真实 LLM 当前受限**:opencode.ai/zen 月度额度耗尽(429,7 天后重置),端到端验证走 `PIKS_AI_PROVIDER=mock`;额度恢复后需补一次真实 provider 冒烟。
- **反向构建隔离**需 BuildKit(本机未装 buildx),仅影响构建耗时,不影响产物隔离。
- **§4.9 接入(预留未做)**:持仓诊断注入 `risk`+`scorecard`、`/chat` 按 section 切块引用 `[R:run_id]`、周报综述纳入本周深研、research 反向消费 `/api/v1` —— 均为迭代 2 增量,本轮只留接口。

## 8. 提交拆分

| commit | 内容 |
|---|---|
| `5e5aa6c` | build: 移除 vendor,改 go.sum + 模块代理构建 |
| `d1bab20` | refactor(research): 并入 research 包并移除 SQLite 存储层 |
| `1dbb142` | feat(db): research_runs 表(0012)+ 存取层 |
| `36e84ea` | feat(research): research-run 编排命令 + 产物契约版本校验 |
| `cec0b78` | build(research): 独立构建 target(piks-research 镜像) |
| `3181f3b` | feat(api): research-runs 三端点(列表/触发/单份) |
| `8f17118` | feat(web): 深研报告页 + 实体/持仓深研入口 |
