# research 并入 PIKS：单仓多运行时 + Go 去 vendor

> 状态：**✅ 已定稿冻结（2026-09-12）**。D-2 = **方案 B（同仓双镜像）**；**research 支持独立迭代**（独立性契约见 §4.10，这是本设计的硬约束）。范围：把 `../investment-research`（个股研究智能体，Python 6562 行 / 5 个依赖）并入 PIKS 仓库，成为 PIKS 的「个股深研」能力；同时移除 Go `vendor/`（8.2MB / 280 文件）改为模块代理构建。
> 契约依据：`internal/ai/ai.go`（Provider 接口）、`internal/ai/openai_compat.go`（Chat/StructuredOutput 现状）、`internal/model/model.go`（Event/Entity/Evidence/Trade 真实 DTO）、`internal/web/server.go`（`/api/v1` 路由与 CORS）、`migrations/0011_position_reviews.sql`（迁移与缓存表约定）、`cmd/entity-build/main.go:93`（实体股票代码落位）、investment-research `cli.py` / `report/synthesis.py` / `analysis/number_lint.py` / `quality_gate.py` / `storage/`。

---

## 1. 背景与现状

### 1.1 两个项目的互补关系

| | PIKS | investment-research |
|---|---|---|
| 定位 | 情报与行动中枢：快讯→事件→实体→情绪→真实交易 | 单股深研快照：代码→财务/龙虎榜/量价→评级 |
| 数据 | 东财快讯、涨停池、市场情绪、真实持仓 | akshare 财务、龙虎榜逐日席位、公告 |
| 哲学 | Fact ≠ Inference ≠ Belief；程序算数字，LLM 只写定性 | Fact / Inference / Opinion 分离；quality gate 数字机检 |
| 存储 | PostgreSQL（唯一 Source of Truth） | SQLite（单文件，`storage/runs.sqlite`） |
| 界面 | React SPA + `/api/v1` | 无（CLI + Pi Agent extension） |

**两边哲学同源，覆盖面错开**：PIKS 的 `/api/v1` 没有任何财务/资金面接口，`/chat` 与持仓诊断只能引用事件与实体；research 的报告没有 Web 入口、没有归档生态、Evidence 存在内存与 SQLite 里"自嗨"。

### 1.2 为什么可以并、且应当并

读代码后的关键事实（决定了并入的可行性）：

1. **research 没有自己的 LLM runtime**。`cli.py` 的 `synthesize` 从 **文件或 stdin** 读取 LLM 输出（`--synthesis-file`），校验槽位 → 渲染 → Number Lint → 归档。**它的接口就是为"外部 LLM"设计的**——PIKS 自己就有 LLM（`app_config` 分层），可以直接填进这个槽位。
2. **体量可控**：Python 59 文件 / 6562 行，其中 providers 938 行、analysis 1818 行；**依赖仅 5 个且全是数据/计算类**（akshare / pandas / numpy / pydantic / dateutil），无 web 框架、无 DB 驱动、无 ORM。
3. **akshare 调用面只有 9 个点**（`stock_financial_analysis_indicator` / `stock_individual_info_em` / `stock_individual_notice_report` / `stock_lhb_detail_daily_sina` / `stock_lhb_stock_detail_em` / `stock_news_em` / `stock_zh_a_hist_tx` / `stock_zh_a_spot_em` / `sw_index_third_cons`），可替换面小、风险可界定。
4. **PIKS 已是多语言镜像**：`Dockerfile` 现有 golang + node 两个构建阶段，加 Python 是第三个阶段，不是拓扑变更。
5. **SQLite 层该删**：`storage/` 327 行只被 `cli.py`(3 处) 与 `workflow/engine.py`(2 处) 引用。并入后归档交给 PG，SQLite 层整体删除——这同时消灭"两份存储双写"的隐患。

### 1.3 vendor 现状

| 项 | 值 |
|---|---|
| 体积 | 8.2 MB |
| 文件数 | 280（全部已入 git） |
| 直接依赖 | **仅 1 个**：`github.com/jackc/pgx/v5` |
| 间接依赖 | 5 个（pgpassfile / pgservicefile / puddle / x/sync / x/text） |
| 构建标志 | `Dockerfile:13` `go build -mod=vendor` |
| 代理 | 环境已配 `GOPROXY=https://goproxy.cn,direct` |

即：**为 1 个直接依赖背了 280 个文件。** 该移除。

---

## 2. 目标与非目标

### 目标

- **G1** 单一仓库维护：research 代码移入 PIKS，`../investment-research` 转为只读归档。
- **G2** 深研能力闭环：实体页/持仓行一键深研 → 报告落 PG → 前端呈现 → AI 诊断与 chat 可引用。
- **G3** 复用 PIKS 既有基建：LLM 配置（`app_config`）、预算护栏、`task_runs` 记账、`/api/v1`、设计令牌。
- **G4** 删除 research 的 SQLite 存储层，PG 为唯一归档。
- **G5** Go 构建去 vendor，改为 go.sum 校验 + 模块代理。
- **G6**（用户明确要求，2026-09-12）**research 支持独立迭代**：改动 `research/` 只需构建/重建 `piks-research` 镜像，**不触碰 Go 与前端**；独立性由 §4.10 的契约保证，且该契约在独立测试中校验（§5.6）。

### 非目标

- 不做代码翻译（不把 Python 改写成 Go）：akshare 是 Python 生态，改写等于丢掉它唯一的数据源价值。
- 不引入第二个 LLM 服务或 agent runtime（复用 PIKS 的 `app_config` 分层）。
- 不把 `/chat` 改成 tool-loop（保持现有检索→引用纪律不动）。
- 不做全市场批量深研（仅手动一键 + 后续定时对持仓/自选）。
- 不改 research 的 provider 业务逻辑（数据源扩展按它自己的 `docs/plan-data-source-integration.md` 走）。

---

## 3. 决策登记（需定稿门确认）

| 编号 | 决策 | 建议 | 理由 |
|---|---|---|---|
| **D-1** | 并入形态：同仓多运行时，不翻译为 Go | ✅ | 6562 行 / 5 依赖 / LLM 可外接，翻译是纯亏损 |
| **D-2** | Python 部署形态：**同仓双镜像**（`piks-tools` + `piks-research`）vs 单镜像加 Python | ✅ **方案 B（同仓双镜像）** | 单镜像：web 容器（nginx）白背 ~400MB；双镜像：仓库/compose 仍是一套，"不用维护两个项目"的诉求已满足，akshare 接口变动可单独重建镜像、且**是 D-11 独立迭代的构建层前提** |
| **D-3** | LLM 合成归属：由 PIKS 的 `ai.Provider` 生成三段定性 | ✅ | 一套配置、一处成本、天然复用预算护栏与 `task_runs` |
| **D-4** | SQLite 存储层：整体删除，`synthesize`/`gate` 改为文件产物 + 归档交给 Go 写 PG | ✅ | 消灭双写；`gate` 现在依赖 `ResearchRunRepository.get_by_run_id`，必须改造 |
| **D-5** | 产物契约：沿用 research 现有文件名（`{symbol}_metrics.json` / `_skeleton.md` / `_synthesis_prompt.txt` / `run_meta.json`），新增 `_final.md` / `_lint.json` / `_gate.json` | ✅ | 不自造格式；metrics.json 是**唯一数字源** |
| **D-6** | 编排方式：Go `os/exec` 调 Python CLI（方案 A），不加 FastAPI | ✅ | 零胶水；CLI 接口原样就是所需契约；与现有 `docker compose run --rm tools ./bin/<cmd>` 模式一致 |
| **D-7** | vendor 移除：从 git 删除 + 改 Dockerfile 为 `go mod download` | ✅ | 1 直接依赖 / 6 总依赖，`goproxy.cn` 可达，go.sum 保证完整性 |
| **D-8** | 本次部署范围：**只做 dev 验证，不部署 lab** | ✅ | 与 trades / trade-loop / weekly-ai-summary 一致；先跑稳再谈生产 |
| **D-9** | Evidence 归并：迭代 1 先整体存 JSONB，不并入 `evidences` 表 | ✅ | 降低首迭代风险；表级归并留迭代 3 |
| **D-10** | 触发范围：手动一键（本轮）；定时批量留迭代 2 | ✅ | 深研单次 10~60s，不宜同步阻塞；范围需可控 |
| **D-11** | **research 独立迭代**：Python 侧改动独立构建、独立测试、独立发版，不牵动 Go/前端 | ✅ **（用户要求，2026-09-12）** | 同仓双镜像（D-2）已提供构建隔离；再加 §4.10 的四重契约保证运行时与产物稳定 |

---

## 4. 方案

### 4.1 目录结构与代码合并

```
PIKS/
├── cmd/
│   ├── research-run/           # 新增:深研编排(采集→合成→机检→归档)
│   └── ...                     # 现有 12 个命令不动
├── internal/
│   ├── research/               # 新增:编排 + 适配层 + 状态机
│   │   ├── runner.go           #   exec python + 产物解析
│   │   ├── artifacts.go        #   产物文件契约(读/校验)
│   │   ├── synth.go            #   ai.Provider 三段定性合成
│   │   └── state.go            #   状态机 pending→gathering→synthesizing→verifying→done/failed
│   └── store/
│       └── research_runs.go    # 新增:PG 读写
├── research/                   # 移入:investment-research/python/src
│   ├── src/
│   │   ├── providers/          #   原样保留(938 行)
│   │   ├── analysis/           #   原样保留(1818 行)
│   │   ├── report/             #   原样保留(683 行)+ synthesize 改造
│   │   ├── models/             #   原样保留(333 行)
│   │   ├── workflow/           #   原样保留(710 行,去掉 storage 调用)
│   │   ├── cli.py              #   改造:去 storage
│   │   ├── quality_gate.py     #   改造:入参改为读产物目录
│   │   └── storage/            #   ❌ 删除(327 行)
│   ├── tests/
│   ├── requirements.txt
│   └── profiles/               #   complete-stock.yaml / short-term.yaml
├── migrations/
│   └── 0012_research_runs.sql  # 新增
└── docs/phase4/                # 本阶段文档
```

**搬迁动作**：`investment-research/python/src/` → `PIKS/research/src/`，`profiles/` 与 `tests/` 一并移入。搬迁用 `git mv` 保留历史（跨仓库时用 `git subtree` 或直接复制 + 首次提交注明来源）。

**保留**：`../investment-research` 仓库**不删**，转为只读归档；等 `research-run` 在 lab 跑稳（建议 ≥2 周）后再正式退役。

### 4.2 Go 构建改造：移除 vendor（D-7）

**删除**
```bash
git rm -r --cached vendor        # 从版本控制移除
rm -rf vendor                    # 本地删除
```
`.gitignore` 增加：
```
# Go 依赖不再入库(改由 go.sum 校验 + 模块代理)
vendor/
```

**Dockerfile 改造**（`build` 阶段）：
```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} GOFLAGS=-mod=mod
# 依赖层单独缓存:go.mod/go.sum 不变则复用,不因业务代码改动重新下载
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/bin/ ./cmd/...
```

要点：
- **保留 `go mod verify`**：go.sum 逐包校验哈希，完整性不弱于 vendor。
- **依赖层缓存**：`go.mod`/`go.sum` 先拷、单独 `go mod download`，业务代码改动不触发重新下载。
- **移除 `-mod=vendor`**，Zig 无需变更。
- 构建期需要代理可达（**这是移除 vendor 的代价**，明确登记）：dev 机已配 `goproxy.cn`；如需离线构建，可用 `docker build --build-arg GOPROXY=off` 配合预热的 module cache（`--mount=type=cache,target=/go/pkg/mod`），列为可选增强。

**同步更新引用**：
- `README.md` 仓库布局段「`vendor/` 自包含构建依赖(镜像构建免网络)」→ 改为「依赖走 go.mod/go.sum + 模块代理，不入库」
- `Dockerfile` 头部注释「自包含构建(go mod vendor,免模块下载…)」→ 同步改写
- 检查 `.dockerignore` 的 `vendor-verify/` 条目（若已无用途则清理）

### 4.10 research 独立迭代契约（D-11，硬约束）

> 用户要求（2026-09-12）：**并入后 research 仍可单独迭代**——改 Python 不需动 Go/前端，不需重建主镜像。这不是"尽力而为"，是设计约束：下列 G1~G4 是可验收的硬指标，破一条即设计违约。

**G1｜零共享状态**
- `research/` 可独立 `git clone` 该子目录 + 独立测试运行，不依赖 `internal/` 的任何代码。
- 反向亦然：**`internal/` 与 `frontend/` 不得 import / 引用 `research/` 的源文件**，只经运行时产物与 HTTP/文件契约交互（§4.4）。
- 校验：`grep -rn "research/" internal/ frontend/src/` 只应命中注释与文档字符串；CI 加此 lint。

**G2｜产物契约版本化（断代保护）**
- `run_meta.json` 增加 `"contract": 1`（整数契约版本）。
- Go 解析时校验版本：
  - `> 当前支持` → `status=failed`，`error="research 产物契约 vN 高于本端支持的 v1，请升级 PIKS"`，**不猜测不硬解**；
  - `< 当前支持` → 按旧版本兼容解析（契约只做加法：新增字段不破坏旧字段语义）。
- **契约变更规则**：新增字段/新 section = 不需升版本（Go 侧忽略未知键）；**改名/删除/改语义 = 必须升 `contract`**。
- 校验：契约测试（§5.6）构造 `contract: 2` 的产物，断言 Go 侧如实失败而非崩溃。

**G3｜依赖独立、构建隔离**
- `research/requirements.txt` 独立（不并入任何 Go/frontend 依赖链）。
- 独立镜像 target：`docker build --target research -t piks-research:<ver>` 只跑 Python 阶段，**不触发 golang/node 阶段**。
- 独立升级路径：akshare 接口变动 → 只重建 `piks-research`；Python 侧包升级同理。**主镜像 `piks-tools` 完全不动**。

**G4｜接口冻结 + 最小版本测试**
- §4.4 的 CLI 三命令（`research` / `synthesize` / `gate`）与产物文件名**冻结**；新增能力可加新命令，不改既有参数语义。
- **独立测试**：`research/tests/` 可不依赖 PIKS 运行（`pytest` 纯 Python，含 provider mock fixture）——这是 research 能自迭代的根基。
- **最小版本测试**（§5.6）：Go 侧用一份**构造好的固定产物 fixture** 跑完整编排，不依赖 Python 运行时。CI 即可验证 Go 侧不因 Python 迭代而回归。

**边界（诚实说明）**
- 若 research 需要**新增数据维度**（如它的 P1-2 披露日历），只要落在既有 section 内 + 产物契约加法（G2），**Go/前端零改动**，前端可先不展示新字段。
- 若需要**新增 section**（前端要展示的新区块），那必然要动前端——这是产品增量，不是独立性破坏；此时 Go 侧只需映射新键，契约仍无需升版本。

### 4.3 Python 并入与部署形态（D-2 = 方案 B：同仓双镜像）

**开发态（dev）**：宿主机 Python venv，直接跑
```bash
cd research && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
./bin/research-run 000560 --profile short-term
```
`cmd/research-run` 通过 `PIKS_PYTHON_BIN`（默认 `research/.venv/bin/python3`）解析解释器。

**部署态（生产 lab）—— 两个候选**：

| | 方案 A：单镜像加 Python | **方案 B：同仓双镜像 ✅ 定稿** |
|---|---|---|
| 镜像 | `piks-tools` 运行时加 `python3` + venv | 新增 `piks-research`；`piks-tools` 保持纯 Go |
| web 容器 | 白背 ~400MB（akshare+pandas+numpy） | 无额外负担 |
| 发版 | 一次 save/load | 两次 save/load（或 compose build），**各自独立** |
| 故障隔离 | akshare 坏了牵连 Go 镜像 | akshare 变动只需重建 `piks-research` |
| 复杂度 | 低 | 中（多一个 Dockerfile target + compose service） |
| **research 独立迭代** | ❌ 做不到（改 Python 必重建主镜像） | ✅ **改 `research/` 只重建本镜像，Go/前端零接触** |

**定稿：方案 B**（用户 2026-09-12 要求"以后支持 research 单独迭代"）。它真正解决了「两种失败面耦合」的问题，且**是 D-11 独立迭代的构建层前提**；「不用维护两个项目」的诉求（单仓库、单 compose、一次改代码）在 B 下同样满足。

**Dockerfile 结构（multi-target，单文件）**
```dockerfile
# ---- research:Python 深研运行时(独立 target,不触发 golang/node 阶段)----
FROM python:3.12-slim AS research
WORKDIR /app
COPY research/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY research/ ./research/
COPY bin/research-run /app/bin/research-run          # Go 侧产物(cmd/research-run)
ENTRYPOINT ["/app/bin/research-run"]                 # 容器即命令:docker run piks-research -- 000560

# ---- build:Go 编译(去 vendor)----
FROM golang:1.26-alpine AS build
# ... 见 §4.2 ...

# ---- frontend:Vite 构建 ----
FROM node:20-alpine AS frontend
# ... 现状不变 ...

# ---- tools:默认最终阶段(现状不变,纯 Go)----
FROM nginx:alpine
# ...
```

要点：
- `piks-research` **只跑 Python 阶段**（`docker build --target research`），golang/node 阶段完全不触发。
- `research-run` 由 Python 镜像里的 `ENTRYPOINT` 直接承载：`docker run --rm piks-research 000560 --profile short-term`，**无需 compose 额外服务**；需要调 LLM 时由它回连 PIKS web 的 `/api/v1` 或直连 PG（编排逻辑在 Go 二进制内，见 §4.4）。
- 生产 `deploy.sh` 变为**双 save/load**（`piks-tools` + `piks-research`），两者可分开升级。

**不推荐**把 research 做成常驻 HTTP 服务（FastAPI）：多一个进程与端口要管，收益只是省一次进程启动。

### 4.3.1 独立迭代工作流（D-11 落地）

```
只改 research(如接 P1-2 披露日历):
  1. vim research/src/providers/calendar/...
  2. pytest research/tests                                    # 独立测试
  3. docker build --target research -t piks-research:latest . # 只跑 Python 阶段
  4. docker compose run --rm research 000560                  # 冒烟
  5. 提交: "feat(research): 披露日历 provider"
  → Go/前端零改动,主镜像 piks-tools 不重建
```

Go 侧的兼容性由 **§5.6 最小版本测试**（固定产物 fixture）保证：即使 Python 侧迭代，CI 也能独立验证 Go 编排不回归。

### 4.4 编排契约：`cmd/research-run`（D-6）

```
bin/research-run <code> [--profile complete-stock|short-term] [--days N] [--force]
```

**状态机**
```
pending → gathering → synthesizing → verifying → done
                 ↘          ↘            ↘
                   failed（任一步；error 如实落库，可 --force 重跑）
```

**执行流程**
```
1. 归一代码 + 解析 profile/as_of → INSERT research_runs (status=pending)
2. status=gathering
   exec: python -m src.cli research <code> --profile <p> [--days N] --out-dir <dir>
   产物: {code}_skeleton.md / {code}_metrics.json / {code}_synthesis_prompt.txt / run_meta.json
   → 落 metrics, as_of = metrics.meta.as_of
3. status=synthesizing
   读 {code}_synthesis_prompt.txt → System
   ai.Provider.StructuredOutput(schema = {summary,trend,conclusion: string})  [extract 档,回退 reasoning]
   → 写 {code}_synthesis_llm.json；落 model/tokens
4. exec: python -m src.cli synthesize <dir> <code> --synthesis-file <file>
   产物: {code}_final.md / {code}_lint.json      ← Number Lint 机检
5. status=verifying
   exec: python -m src.cli gate <dir> <code> --profile <p>
   产物: {code}_gate.json                        ← 六项机检
6. status=done；落 markdown/lint/gate/synthesis
```

**约束**
- **契约校验（§4.10 G2）**：第 2 步后读 `run_meta.json`，校验 `contract` 版本；高于本端支持即 `failed`，不进后续步骤。
- **超时**：每步 `context.WithTimeout`（采集 180s / 合成 120s / 机检 30s），超时即 `failed`。
- **预算护栏**：合成前查 `ai_daily_token_budget`（现键名 `ai_daily_token_budget`，见 `migrations/0005_app_config.sql`），耗尽 → `failed` 并如实标注，**不降级不编造**。
- **记账**：`task_runs` 记 `research-run:gather` / `research-run:synth` / `research-run:verify`，含 `ai_tokens`（`internal/model/model.go` TaskRun 的 `Meta` JSONB）。
- **幂等**：同一 `run_id` 重复导入走 `ON CONFLICT (run_id) DO UPDATE`；`--force` 生成新 run 而非覆盖旧档（保留历史版本，支撑后续 delta 复研）。
- **断点重试**：产物目录已存在则跳过对应步骤（采集结果可复用），仅重跑失败步。

**机检失败处置（关键）**：`lint.passed == false` 或 `gate.passed == false` 时**不静默**：
- 保留 `markdown` 为**确定性骨架报告**（`_skeleton.md`），`synthesis` 落库但前端标注「AI 研判未通过数字机检」；
- status 仍为 `done`（产物可用），但 `gate`/`lint` JSONB 原样呈现问题清单；
- 若 `synthesize` 直接拒收（`validate_synthesis` 槽位校验失败），重试合成一次，仍失败则 `failed`。

### 4.5 LLM 合成：复用 PIKS ai.Provider（D-3）

```go
// internal/research/synth.go（示意，非最终实现）
req := ai.StructuredRequest{
    System: prompt,              // {code}_synthesis_prompt.txt 原文(含硬性约束:只能引用指标卡数字)
    User:   "请按提示要求输出三段定性分析。",
    Schema: json.RawMessage(`{
      "type":"object",
      "properties":{
        "summary":{"type":"string"},"trend":{"type":"string"},"conclusion":{"type":"string"}
      },
      "required":["summary","trend","conclusion"]
    }`),
}
resp, err := provider.StructuredOutput(ctx, req)
```

- **模型选择**：`ai_model_extract`（便宜档）优先，未配置回退 `ai_model_reasoning`——与 `weekly-ai-summary` 同模式。
- **天然的第二道防幻觉闸门**：PIKS 的 LLM 若编造数字，第 4 步 `synthesize` 的 Number Lint（容差 6%，`analysis/number_lint.py`）会拒收或标记。**这比让 chat 裸写结论安全得多**，也是选择"PIKS 提供 LLM"而非"research 自跑 LLM"的核心理由。
- **Fact/Opinion 分域**：`metrics`（确定性计算）→ **Fact**；`synthesis`（LLM 定性）→ **Opinion**；`evidence`（research 的 Evidence 链）→ 支撑 Fact 的溯源。前端分区呈现（见 4.8）。

### 4.6 数据模型：`migrations/0012_research_runs.sql`

```sql
-- 个股深研报告归档(research 并入,design research-merge.md D-5/D-9)。
-- 一个 run = 一次深研快照;同 code 多 run = 时间序列(支撑后续 delta 复研)。
-- metrics 是数字唯一源(Fact);synthesis 是 LLM 定性(Opinion);两者严格分域。
CREATE TABLE IF NOT EXISTS research_runs (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id     TEXT NOT NULL UNIQUE,          -- python 侧 run_id(幂等键)
  code       TEXT NOT NULL,                 -- 归一 6 位(join entities.detail->>'code')
  symbol     TEXT NOT NULL,                 -- research full_code(如 sh600519)
  profile    TEXT NOT NULL,                 -- complete-stock / short-term
  as_of      DATE NOT NULL,                 -- 数据截止交易日(防未来函数基准)
  status     TEXT NOT NULL DEFAULT 'pending', -- pending/gathering/synthesizing/verifying/done/failed
  metrics    JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {code}_metrics.json(数字唯一源 = Fact)
  synthesis  JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {summary,trend,conclusion}(LLM 定性 = Opinion)
  markdown   TEXT,                                -- 最终报告(机检通过)或骨架报告(未通过)
  lint       JSONB NOT NULL DEFAULT '{}'::jsonb,  -- {scanned,matched,ignored,passed,issues[]}
  gate       JSONB NOT NULL DEFAULT '{}'::jsonb,  -- 六项机检结果
  evidence   JSONB NOT NULL DEFAULT '[]'::jsonb,  -- research Evidence 链(迭代 3 再并入 evidences 表)
  error      TEXT,                                -- 失败原因(如实,不掩盖)
  model      TEXT NOT NULL DEFAULT '',            -- 合成所用模型(如实标注)
  tokens     BIGINT NOT NULL DEFAULT 0,           -- 本次合成 token(task_runs 同记,双份留痕)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_research_runs_code ON research_runs(code, as_of DESC);
CREATE INDEX IF NOT EXISTS idx_research_runs_status ON research_runs(status);
```

**代码归一规则**（关键 join 点）：
| 来源 | 形态 | 例 |
|---|---|---|
| research `symbol_metrics.json` → `meta.symbol` | 带市场前缀 full_code | `sh600519` |
| PIKS `entities.detail->>'code'`（`cmd/entity-build/main.go:93`） | 6 位 | `600519` |

归一函数：`sh600519 → 600519`（去前缀）；反向由前缀推 `market`。**join 查询**：
```sql
SELECT r.* FROM research_runs r
JOIN entities e ON e.type='company' AND e.detail->>'code' = r.code
WHERE e.id = $1 ORDER BY r.as_of DESC;
```

### 4.7 Python 侧改造清单

| 文件 | 改动 | 说明 |
|---|---|---|
| `src/storage/`（整个目录） | ❌ 删除 | 327 行；`db.py` / `research_run.py` / `__init__.py` |
| `src/cli.py` | 去 storage（3 处 import） | `run_synthesize` 的 `ResearchRunRepository.archive_synthesis` 删除，改为：渲染 final.md + 写 lint.json，**归档交给 Go**；`run_gate` 改从产物目录读，不再 `get_by_run_id` |
| `src/workflow/engine.py` | 去 storage（2 处调用） | 删 `create` / `finalize`；run_id 生成保留（写进 `run_meta.json`） |
| `src/report/synthesis.py` | 无改动 | `build_synthesis_prompt` / `parse_synthesis` / `validate_synthesis` 原样 |
| `src/analysis/number_lint.py` | 无改动 | Number Lint 原样（PIKS 侧消费其 JSON 输出） |
| `src/quality_gate.py` | 入参改造 | `run_quality_gate` 已经是纯函数（profile_sections + json_report + evidence + lint + fingerprint），只需让 `gate` CLI 从目录读这些入参 |
| `requirements.txt` | 加版本锁 | 建议 `pip freeze` 锁版本，减少 akshare 变动面 |
| `run_meta.json` | 加 `run_id` + **`contract: 1`** 字段 | Go 侧幂等键 + 契约版本（§4.10 G2） |
| `tests/` | 保持独立可跑 | `pytest` 纯 Python + provider mock fixture，不依赖 PIKS（§4.10 G4） |

### 4.8 前端呈现

**新增页面**：`frontend/src/pages/research.tsx`（路由 `/research/:runId`）——只读报告页。

**入口**：
1. 实体库股票实体卡（`frontend/src/pages/entities.tsx`）→ 「深研」按钮
2. 持仓行（`frontend/src/pages/trades.tsx`）→ 「深研」按钮
3. 已有报告 → 「查看报告」（跳 `/research/:runId`）

**页面结构（Fact / Opinion 分区）**
```
┌─ 头部：股票名 + 代码 + as_of + profile + 机检徽标 ──┐
│   ✅ 数字已机检通过   /   ⚠️ AI 研判未通过数字机检   │
├─ 一、Fact 区（确定性计算，可下钻 Evidence）─────────┤
│   股价表现 / 成交量价 / 基本面 / 事件 / 龙虎榜      │
│   每个数字：tabular-nums + 右对齐；涨红跌绿          │
│   每节标「来源：确定性计算 + Evidence N 条」          │
├─ 二、Opinion 区（AI 综合研判）───────────────────┤
│   执行摘要 / 趋势解读 / 综合结论                     │
│   显著标注「AI 定性研判，非事实」+ 模型名 + tokens    │
├─ 三、机检详情（折叠）───────────────────────────┤
│   Number Lint 扫描/匹配/忽略/问题清单               │
│   六项 Quality Gate 逐项通过/失败 + 依据             │
└────────────────────────────────────────────────┘
```

**组件拆分**（遵循 CLAUDE.md 强制规则 5/6/9）：
| 文件 | 职责 | 预计行数 |
|---|---|---|
| `pages/research.tsx` | 路由 + 三态（loading/error/empty）+ 布局 | <100 |
| `components/research/ReportHeader.tsx` | 徽标 + 元信息 | <60 |
| `components/research/FactSection.tsx` | Fact 区渲染 | <80 |
| `components/research/OpinionSection.tsx` | Opinion 区 + 免责标注 | <60 |
| `components/research/GatePanel.tsx` | lint/gate 折叠详情 | <80 |
| `components/research/DeepResearchButton.tsx` | 按钮 + 状态徽标轮询 | <70 |
| `hooks/useResearchRun.ts` | 触发 + 轮询（>50 行逻辑抽 hook） | — |

**新增 `/api/v1` 接口**（`internal/web/api_research.go`，复用 `research_runs` store）
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/v1/research-runs` | 触发深研（body: `{code, profile?, days?}`）→ 返回 `run_id` + `status` |
| GET | `/api/v1/research-runs?code=` | 某股票报告列表（as_of DESC） |
| GET | `/api/v1/research-runs/:runId` | 单份报告全量（metrics/synthesis/markdown/lint/gate/evidence/error） |

**接口契约（响应形状，逐字对齐 DTO）**
```jsonc
// GET /api/v1/research-runs/:runId
{
  "run_id": "…", "code": "000560", "symbol": "sz000560",
  "profile": "short-term", "as_of": "2026-09-11",
  "status": "done",                       // pending|gathering|synthesizing|verifying|done|failed
  "metrics": { "meta": {...}, "price": {...}, "volume": {...},
               "financial": {...}, "events": {...}, "risk": {...},
               "capital": {...}, "scorecard": {...} },   // Fact
  "synthesis": { "summary": "…", "trend": "…", "conclusion": "…" },  // Opinion
  "markdown": "# 个股研究报告…",
  "lint":  { "scanned": 42, "matched": 41, "ignored": 30, "passed": true, "issues": [] },
  "gate":  { "passed": true, "checks": [{"name":"data_completeness","passed":true,"detail":"…"}] },
  "evidence": [ { "id": "…", "type": "fact", "tier": "structured", "statement": "…", "section": "price" } ],
  "error": null, "model": "deepseek-chat", "tokens": 563,
  "created_at": "…", "updated_at": "…"
}
```

**状态轮询（三态）**：`pending/gathering/synthesizing/verifying` → 状态徽标（"采集中/合成中/机检中"），**用文字徽标不用骨架屏**（CLAUDE.md：核心数字区禁 skeleton loader）；`failed` → 错误态 + `error` 原文 + 重试按钮；`done` → 正常渲染。

### 4.9 与现有功能的接入（迭代 2 预留，本轮只留接口）

- **持仓诊断**（`internal/web/trades.go` 的 `tradeReview` 管线）：注入该股最新 `research_runs` 的 `risk` + `scorecard` 摘要，引用体系加 `[R:run_id]`。
- **`/chat`**：报告按 section 切块进 `SearchKnowledgeExpanded`，引用类型加 `R`。
- **周报 AI 综述**：本周深研进 `generateWeeklySummary` 上下文。
- **research 反向消费 PIKS**：改用 `/api/v1/market/snapshot`（情绪分+涨停池，替代其 P2-2 自建）与 `/api/v1/events`（结构化事件，替代 akshare 原始新闻降噪）。`/api/v1` 已带 `Access-Control-Allow-Origin: *` 且只读（`internal/web/server.go:60`）。

---

## 5. 验收清单（dev-only）

### 5.1 vendor 移除（可独立先行）
- [ ] `git rm -r --cached vendor` + `.gitignore` 加 `vendor/`；仓库体积下降
- [ ] `go build ./...` / `go vet ./...` / `go test ./...` 全过（`go.sum` 校验生效）
- [ ] `docker build` 成功；确认依赖层缓存生效（改业务代码不重新下载）
- [ ] `README.md` / `Dockerfile` 注释中 vendor 表述已同步
- [ ] 生产镜像重建后 `docker compose run --rm tools ./bin/migrate` 冒烟通过

### 5.2 Python 并入 + 存储层归并
- [ ] `research/` 目录就位，`storage/` 已删；`grep -r "storage" research/src` 无残留引用
- [ ] `python3 -m src.cli research 000560 --out-dir /tmp/r` 产出四文件（skeleton/metrics/prompt/run_meta）
- [ ] `python3 -m src.cli synthesize …` 产出 `_final.md` + `_lint.json`（不再碰 SQLite）
- [ ] `python3 -m src.cli gate …` 产出 `_gate.json`（从目录读入参）
- [ ] `000560` + `600519` 两票冒烟全过（与并入前输出一致，diff 无业务差异）

### 5.3 编排 + 落库
- [ ] `bin/research-run 000560 --profile short-term` 端到端 `done`
- [ ] `research_runs` 有行：status=done、metrics/synthesis/markdown/lint/gate 齐全
- [ ] `task_runs` 三条记录（gather/synth/verify），`ai_tokens` 已记
- [ ] 幂等：同 run_id 重跑不增行；`--force` 生成新行保留旧档
- [ ] 失败注入（断网/超时/预算耗尽）→ status=failed + error 如实，无编造数据
- [ ] 机检故意失败样本 → markdown 回落骨架报告，lint/gate 问题清单原样落库

### 5.4 前端
- [ ] 实体卡「深研」按钮 → 状态徽标轮询 → 报告页
- [ ] 持仓行「深研」按钮同上
- [ ] Fact 区数字：等宽 + tabular-nums + 右对齐；涨红跌绿（`#E24B4A`/`#1D9E75`）
- [ ] Opinion 区显著标注「AI 定性研判，非事实」+ 模型名 + tokens
- [ ] 机检详情可折叠，问题清单可见
- [ ] 三态（loading/error/empty）齐全；核心数字区**无 skeleton**
- [ ] `npm run build` / `tsc --noEmit` 全过；组件文件 ≤150 行

### 5.5 契约
- [ ] 三个 `/api/v1/research-runs*` 端点 curl 全 200，形状对齐前端 `types.ts`
- [ ] `code` join：实体 → 报告列表能正确命中（`detail->>'code'` 归一验证）
- [ ] CORS 头正常（复用现有 `cors` 中间件）
- [ ] prod 未动：无 lab 部署、prod DB 无新迁移

### 5.6 research 独立迭代（D-11，硬验收）
- [ ] **零共享状态**：`grep -rn "research/" internal/ frontend/src/` 只命中注释/文档字符串（CI lint）
- [ ] **构建隔离**：`docker build --target research -t piks-research:test .` 成功，且**日志中无 golang/node 阶段**；反向 `--target tools` 不跑 Python 阶段
- [ ] **契约升版保护**：构造 `run_meta.json` 含 `contract: 2` 的 fixture → Go 编排 `status=failed` + `error` 明示版本不支持，**不崩溃、不硬解**
- [ ] **契约降版兼容**：`contract: 1` 产物正常 `done`
- [ ] **最小版本测试**：`go test ./internal/research/...` 用固定产物 fixture 跑完整编排，**不依赖 Python 运行时**（CI 可跑）
- [ ] **Python 独立测试**：`pytest research/tests` 在该子目录内可独立通过（不依赖 PIKS）
- [ ] **端到端独立迭代演练**：改一处 research 代码（如加日志）→ 只 `docker build --target research` + 冒烟，**Go 与前端未重建**，报告页仍正常
- [ ] **产物契约冻结**：CLI 三命令的参数语义与五类产物文件名与设计一致

### 5.5 契约
- [ ] 三个 `/api/v1/research-runs*` 端点 curl 全 200，形状对齐前端 `types.ts`
- [ ] `code` join：实体 → 报告列表能正确命中（`detail->>'code'` 归一验证）
- [ ] CORS 头正常（复用现有 `cors` 中间件）
- [ ] prod 未动：无 lab 部署、prod DB 无新迁移

---

## 6. 涉及文件

**新增**
```
cmd/research-run/main.go
internal/research/{runner,artifacts,synth,state}.go
internal/store/research_runs.go
internal/web/api_research.go
migrations/0012_research_runs.sql
research/…                         (自 investment-research 移入)
frontend/src/pages/research.tsx
frontend/src/components/research/{ReportHeader,FactSection,OpinionSection,GatePanel,DeepResearchButton}.tsx
frontend/src/hooks/useResearchRun.ts
docs/phase4/design/research-merge.md  (本文档)
docs/phase4/stages/research-merge.md  (落地后归档)
internal/research/artifacts_test.go   (最小版本测试:固定产物 fixture,不依赖 Python)
```

**修改**
```
Dockerfile                         (去 vendor + `research` 独立 target;见 §4.3)
.gitignore                         (+vendor/)
README.md                          (仓库布局 + 技术栈表述)
internal/web/server.go             (注册 3 个路由)
internal/store/migrate.go          (如迁移列表需登记)
frontend/src/types.ts              (+ResearchRun 类型)
frontend/src/api.ts                (+3 端点)
frontend/src/router.tsx            (+/research/:runId)
configs/docker-compose*.yml        (+research 服务/运行入口)
docs/进度总表.md                    (阶段总览 + 子阶段明细 + 目录映射 → phase4)
```

**删除**
```
vendor/                            (280 文件 / 8.2MB)
research/src/storage/              (327 行)
```

---

## 7. 边界（明确不做）

1. **不做代码翻译**：Python 保留，不重写为 Go。
2. **不引入第二个 LLM 服务**：复用 `app_config`。
3. **不把 `/chat` 改成 tool-loop**：保持检索→引用纪律；深研作为可引用对象，不作为对话内可调用动作。
4. **不做全市场批量深研**：仅手动触发（迭代 2 加持仓/自选定时）。
5. **不改 research 的 provider 业务逻辑**：数据源扩展按它自己的 P0/P1/P2 计划走（建议 P0 在本轮之后、于 PIKS 仓内执行）。
6. **不做 Evidence 表级归并**：迭代 1 整体存 JSONB。
7. **不做 delta 复研 / 报告 diff**：迭代 3。
8. **不部署 lab**：D-8。
9. **不删 `../investment-research` 仓库**：转只读归档，验稳后再退役。

---

## 8. 执行顺序（任务卡）

> 依赖关系：**T1 独立可先行**（不依赖 Python 并入）；T2 是 T3~T6 的前置。

| 卡 | 任务 | 依赖 | 验收 |
|---|---|---|---|
| **T1** | Go 去 vendor + Dockerfile 依赖层改造 | — | §5.1 全过 |
| **T2** | 代码搬迁 + 删 storage + CLI 三命令改造 + `run_meta.json` 加 `run_id`/`contract` | T1 | §5.2 全过 |
| **T3** | `0012` 迁移 + `internal/store/research_runs.go` | — | 迁移可应用、CRUD 测试过 |
| **T4** | `internal/research/*` 编排 + `cmd/research-run` + 契约版本校验 | T2, T3 | §5.3 全过 |
| **T4.5** | Dockerfile `research` 独立 target + compose 入口 | T2 | §5.6 构建隔离项全过 |
| **T5** | `internal/web/api_research.go` 三端点 | T3 | curl 200 + 形状对齐 |
| **T6** | 前端报告页 + 深研按钮 + 接入实体/持仓 | T5 | §5.4 全过 |
| **T7** | 独立迭代验收（fixture 测试 + 演练）+ 归档 + 进度总表 + 冻结 | T1~T6 | §5.5 + **§5.6 全过** |

**建议提交切分**（每卡至少一次提交，便于回滚）：
1. `build: 移除 vendor,改 go.sum + 模块代理构建`
2. `refactor(research): 并入 research 包并移除 SQLite 存储层`
3. `feat(db): research_runs 表(0012)`
4. `feat(research): research-run 编排命令 + 产物契约版本校验`
5. `build(research): 独立构建 target(piks-research 镜像)`
6. `feat(api): research-runs 三端点`
7. `feat(web): 深研报告页与入口`
8. `test(research): 最小版本 fixture 测试 + docs 归档`

**分支纪律**：从 `dev` 拉 `feat/research-merge`，全卡完成且验收通过后合回 `dev`；`master` 不直接推。

---

## 9. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| **akshare 接口变动 / 网络受限** | 深研失败 | **独立镜像（D-2 B）隔离**：只需重建 `piks-research`，Go/前端零接触；`failed` 状态如实落库；research 自己的 `spike/findings-data-sources.md` 已实测 20+ 接口，**直接复用结论不重测** |
| **移除 vendor 后构建依赖代理** | 离线/代理不可达时构建失败 | dev 机已配 `goproxy.cn`；可选 `--mount=type=cache` 预热 module cache；`go.sum` 保证完整性 |
| **镜像体积** | 若单镜像则 web 容器白背 ~400MB | **D-2 定稿方案 B（双镜像）规避** |
| **research 迭代破坏产物契约** | Go 解析崩溃/错值 | §4.10 G2 契约版本 + 升版显式失败 + §5.6 最小版本 fixture 测试；契约只做加法 |
| **Go 侧意外耦合 research 源文件** | 独立性名存实亡 | §4.10 G1 零共享状态 + CI `grep` lint 硬验收 |
| **research 的 P0 数据源扩展与本轮撞车** | 合并期有未提交改动 | 现状工作树 clean，可先合；**P0 在合并后于 PIKS 仓内做**（`research/` 子目录独立迭代，§4.3.1），顺手对齐 provider 规范 |
| **Number Lint 误报** | 报告被标"未通过机检" | 容差 6% 已有；问题清单原样呈现供人工判断，不自动改数 |
| **`gate` CLI 改造不彻底** | 残留 SQLite 依赖 | §5.2 用 `grep -r storage research/src` 作为硬验收 |
| **代码 join 归一错误** | 深研挂在错实体 | 归一函数单测 + §5.5 显式验收 join |

---

## 10. 决策留痕

| 日期 | 决策 |
|---|---|
| 2026-09-12 | 评估 steady 是否并入 → **不并入**（两个完整 Go+Python 服务栈，含模拟交易系统，合并只增耦合）；数据交集待 SQL 验证后再谈窄点集成 |
| 2026-09-12 | 评估 research 是否并入 → **并入**（6562 行 / 5 纯数据依赖 / LLM 可外接 / SQLite 层可删，翻译为 Go 是纯亏损） |
| 2026-09-12 | research 的 LLM 归属 → 由 PIKS `ai.Provider` 提供（D-3）；`synthesize` 的 Number Lint 作为第二道防幻觉闸门 |
| 2026-09-12 | `/chat` 不改 tool-loop（D-非目标）：保持检索→引用纪律，深研为可引用对象而非可调用动作 |
| 2026-09-12 | 用户提出移除 Go vendor（8.2MB / 280 文件）→ 纳入本轮（D-7） |
| 2026-09-12 | 用户确认 **D-2 = 方案 B（同仓双镜像）**，并明确要求「**以后支持 research 单独迭代**」→ 升级为硬约束 **D-11**（§4.10 四重契约 + §5.6 独立验收），**本设计就此定稿冻结** |
