# PIKS — Personal Investment Knowledge System

个人 A 股投资知识系统:把公开市场信息(新闻快讯、行情涨停池)自动加工成**结构化事件与实体**,沉淀为个人知识库。**PostgreSQL 是唯一数据源,React SPA 是界面**(迭代 5 建 Web 平台取代 Obsidian/GitHub;2026-08-29 前端去 Next.js 改 **Vite SPA + nginx 网关**,Go 只提供 JSON API)。

> **架构速查**:完整现状架构见 [`docs/架构总览.md`](docs/架构总览.md)(以代码为准)。

## 核心理念

- **Fact ≠ Inference ≠ Belief**:事实层(AI 抽取的事件/实体,含置信度)与你的推断/信念严格分域;卡片"事实"给机器产出,"我的理解"留给你写判断。
- **AI → 结构化输出 → Schema 校验 → 业务校验**:LLM 只产出结构化 JSON,入库前层层校验,不直接写库。
- **数据诚实**:缺失如实标空态(`pending`/`_暂无_`),宁缺毋假;`reconcile` 每日对账,异常不掩盖。

## 一日管线(数据流)

```
migrate → collector -driver all(6 快讯源) → collector -driver cninfo-announce(巨潮公告)
→ hot-topic(热榜两源;常驻进程之外的收盘态兜底)
→ worker -limit 800(AI 抽取 events) → cluster(语义去重 + 重审视 Pass)
→ quote-collector(涨停池,仅交易日) → entity-build(实体) → market-state(市场情绪)
→ daily-review(每日复盘) → reconcile(对账)
```

10 个管线命令(见 `cmd/`),各自幂等、可单独重跑;失败步骤记录不阻断(下次重试)。
> 迭代 5-2 起:vault / GitHub 下线(SPA 直读 PG API,管线已无发布步骤)。
> **盘中增量**(issue #68 C 层):另有常驻 `collector` 服务,交易日 09:15–15:05 **每 3 分钟**采快讯 6 源(与日管线幂等共存;见 `docs/数据源总览.md` §3)。
> **热榜**(issue #68 D 层):常驻 `hot-topic` 服务,交易日**每 30 分钟**采两源,落**独立表** `hot_topic_items`;与事件链路零交集(见 `docs/phase11/design/hot-topic.md`)。

## 功能模块

- **每日管线**:新闻→事件抽取→语义去重聚类(含重审视 Pass 修跨簇重复)→涨停池→实体构建→市场情绪→每日复盘→对账,全自动幂等。
- **Web 平台**(`cmd/web` JSON API + React SPA,lab :8090):今天(自选)/ 市场概况 / 消息(重要 + 快讯 + 公告三 tab)/ 涨停股 / **热榜**(两源分列)/ 研报(独立阅读器)/ 个股分析 / 个股中心 `/stock/:code`(含买入前速评)/ 交易与持仓(截图识别录入 + AI 带引用解读 + 持仓 AI 诊断)/ 持仓诊断 / 周报(规则聚合 + AI 综述手动触发)/ 笔记 / 问 AI(问答带引用 + 截图 vision)/ 设置(大模型配置);实体库·图谱·对账移入设置页「数据与运维」。
- **交易闭环**(2026-08-28):每日自交易截图 → 视觉抽取 → 确认入库;AI 解读带知识库引用、防未来函数;持仓 AI 诊断;本周交易/持仓进周报。后续已上生产(截图识别依赖视觉模型配置;`/settings` 配好后可用)。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.26(静态编译,依赖走 go.mod/go.sum + 模块代理,不入库) |
| 数据源 | PostgreSQL 16(唯一 Source of Truth;**18 个前向迁移**,0001~0018) |
| 界面 | **React SPA**(Vite 5 + React 18 + TS,React Router v6;Tailwind 只做布局,视觉走 `globals.css` 语义类;ECharts 按需 + 自绘 SVG 力导图谱;nginx 单入口 :8090 服务静态 + 反代 `/api/*`)。Obsidian/GitHub 已下线,`PIKS-Vault/` 仅存档 |
| AI | OpenCode Zen,OpenAI 兼容;**base URL 必须带 `/go` 路由**(`https://opencode.ai/zen/go/v1`);配置存 `app_config` 表(/settings 可编辑),模型分层 extract/reasoning/vision |
| 部署 | Docker Compose(dev 单机 + 生产 lab) |

## 仓库布局

```
cmd/           14 个可执行命令(10 个管线:migrate/collector/hot-topic/worker/cluster/
              quote-collector/entity-build/market-state/daily-review/reconcile + web 常驻 API
              + research-run 深研 CLI + research-worker 深研队列 worker + probe 探针(不进任何镜像))
internal/      13 个业务包(store / web / collector / research / ai / cluster / publish
              / announce(公告分级规则)/ entityextract / marketstate / extract / model / config)
frontend/      React SPA(Vite;src 145 文件;28 条路由;产物 dist/)
research/      深研 Python agent(独立运行时见下「深研并入」;不写库、不调 LLM,产物落 PG)
migrations/    SQL 迁移(前向,无 down;0001~0018)
prompts/       AI 抽取提示词(extract.md)
configs/       docker-compose(dev/prod)+ .env 模板 + nginx.conf
scripts/       dev 侧 setup.sh/deploy.sh/check-research-isolation.sh/check-image-topology.sh/check-event-type-parity.sh;lab 侧 pipeline.sh/backup.sh/health.sh(setup.sh 装 crontab)
(依赖不入库:go.sum 校验 + GOPROXY 模块代理,见 Dockerfile)
docs/          架构总览(现状架构,以代码为准)、项目详解、进度总表、各阶段设计定稿 + 实现归档
PIKS-Vault/    Obsidian vault 存档(界面层已下线,不再更新)
```

## 深研并入(research,能力并入 P4)

`research/`(自 `investment-research` 并入)是 Python 个股深研 agent;Go 侧 `internal/research`
用 `os/exec` 调其 CLI 三命令(`research`/`synthesize`/`gate`),读产物落 `research_runs`。
设计与验收见 `docs/phase4/{design,stages}/research-merge.md`。

**独立迭代(代码级)**:Go 与前端**不依赖 research 源码**,只经「CLI 参数 + 产物契约」
(冻结,`research/README.md` 契约表)交互 —— 由 `scripts/check-research-isolation.sh` 校验。

**部署形态(2026-09-20 起,四镜像)**:单 Dockerfile 多 target,拆成 `piks-gateway`(纯 nginx)
/ `piks-web`(纯 Go API)/ `piks-tools`(10 个管线命令)/ `piks-research`(Python 运行时 + 队列
worker)。深研**不再由 web 进程内 `os/exec python3` 触发** —— web 只写一条 `pending` 行并
`NOTIFY`,research 容器的常驻 worker 认领执行(`migrations/0015`、`cmd/research-worker`)。
这样「改前端只重建 gateway、改 Python 只重建 research」,升级半径与实际改动对齐。

```bash
# 独立性 CI 检查(零共享状态 / 产物契约面)
./scripts/check-research-isolation.sh
# 镜像拓扑检查(四 target 内容边界;防 web 混入 nginx/Python)
./scripts/check-image-topology.sh
# 事件类型枚举单一真源检查(防前端再存本地枚举表;issue #61)
./scripts/check-event-type-parity.sh

# 两侧测试(可各自独立跑)
go test ./internal/research/...                                 # fixture 状态机,不依赖 Python
cd research && .venv/bin/python -m pytest tests                 # Python 单测,不依赖 PIKS
#   依赖:requirements.txt(运行)+ requirements-dev.txt(测试;不进运行镜像)

# 生产深研(research 容器内;worker 常驻,CLI 为旁路入口)
docker compose exec research ./bin/research-run 000560 --profile short-term
```

## 快速开始(dev,本机)

```bash
# 1. 起 postgres(宿主端口 5433,与外部隔离)
docker compose -f configs/docker-compose.yml up -d

# 2. 准备数据库连接(仅此环境变量必需;AI 配置走 app_config 表,见第 5 步)
set -a; source .env.local; set +a   # 键:PIKS_DATABASE_URL / PIKS_LISTEN_ADDR / PIKS_UPLOAD_DIR

# 3. 构建并跑迁移(migrate 会种子 app_config 默认值)
go build -o bin/ ./cmd/...
./bin/migrate

# 4. 手动跑一次全链(或等生产 crontab 自动;命令均幂等)
./bin/collector -driver all      # 6 快讯源;盘中增量为 -driver news -interval 3m(常驻)
./bin/worker
./bin/cluster
./bin/quote-collector -date $(date +%F)
./bin/entity-build
./bin/market-state -date $(date +%F)
./bin/daily-review -date $(date +%F)
./bin/reconcile -date $(date +%F)

# 5. 起 Web 平台并配置大模型
./bin/web          # http://localhost:8090
# 浏览器打开 /settings 填 AI 服务地址 / API Key / 模型(extract/reasoning/vision),保存即生效
```

> 生产环境用 6 机构源(`-driver all`,见 `docs/数据源总览.md` §2.1.1);`file` 驱动仅迭代 0 保底。
> 大模型配置不再读 `PIKS_AI_*` 环境变量(2026-08-27 起改存 `app_config` 表)。

## 生产部署(lab)

- **模型**:dev 本地**分镜像**构建 → `docker save | ssh lab docker load` 传输;lab 不保留代码仓库,镜像 = 唯一交付物。编排全在 dev 侧。
- **服务**:`postgres`(常驻)+ `gateway`(常驻,唯一对外 `:8090`)+ `web`(常驻,私网内 `:8090`,不发布宿主端口)+ `research`(常驻,深研队列 worker)+ `collector`(常驻,盘中每 3 分钟采快讯,**复用 tools 镜像**,issue #68 C 层)+ `hot-topic`(常驻,盘中每 30 分钟采热榜,**复用 tools 镜像**,issue #68 D 层)+ `tools`(profile=run,跑管线命令)。
- **文档**:设计 `docs/phase3/design/prod-deploy.md`(D-P1~P12);实现与验收 `docs/phase3/stages/prod.md`;四镜像拆分设计 `docs/phase10/design/container-split.md`;快讯提频/护栏设计 `docs/phase11/design/flash-cadence.md`。
- **公网入口**(2026-09-22,issue #78):**https://piks.5home.online** —— 阿里云宿主边缘 Nginx(443,SNI 分流)经 **frp stcp 隧道**回源 lab 的 `piks-gateway:8090`(全栈仍跑 lab,阿里云只做入站;回源口 `127.0.0.1:17010` 只绑 loopback,公网不可达)。⚠️ **当前无鉴权(裸奔)**,见 `docs/架构总览.md` §9.5 与 issue #78。
- **运维速查**:
  - 更新:`./scripts/deploy.sh`(dev 侧按镜像建/传 → 同步 compose **与 lab 侧 `scripts/`** → migrate → 起 web/research/collector/gateway)
  - 日管线:crontab 每 15min 自判(北京时间非交易日/已过 16:10/今日未跑),stamp 防重跑
  - 盘中采集:`collector` 常驻服务自判(工作日 + 09:15–15:05 时段闸),每 3 分钟一轮;`per-host` 反封禁护栏(令牌桶/空响应哨兵/熔断)
  - 备份:每晚 `pg_dump` → `/home/rguo/piks/backups/`,14 天留存
  - 日志:`ssh lab 'tail -50 /home/rguo/piks/logs/pipeline-$(date +%F).log'`

## 文档索引

| 文档 | 内容 |
|---|---|
| `docs/项目详解.md` | 全局架构、数据流、技术栈、边界、执行决策(权威决策登记处) |
| **`docs/架构总览.md`** | **以代码为准的完整现状架构**(三运行时 / 数据层 / API / 前端 / 部署 / 约束速查 / 文档偏差审计) |
| `docs/进度总表.md` | 里程碑跟踪(各阶段状态 + 子阶段明细 + 契约缺口 + 已知遗留) |
| `docs/phase1/` | 迭代 0 地基 + 迭代 1 可靠性(冻结) |
| `docs/phase2/` | 迭代 2~5(增值 + Web 平台)设计定稿 + 实现归档(冻结);G8/聚类质量/周报综述/交易/交易闭环 |
| `docs/phase3/` | 生产化(设计定稿 + 实现验收归档) |
| `docs/phase4/`~`phase9/` | 能力并入(research)/ 前端 IA / 决绝重构 / 买入前速评 / 手机投递 / 研报体裁 |
| `docs/phase10/` | 容器拆分(单镜像 → 四镜像,issue #47)设计定稿 |
| `docs/phase11/` | 事件类多源交叉验证(epic #43 T2/T3/T4)+ 数据源分层(issue #68:S1 公告分级 / C 层快讯提频)设计;快讯提频 dev-only |
| `PIKS架构设计文档.md` | v1.0 权威架构蓝图(冻结不改正文;顶部含现状偏差注记) |

## 安全红线

- **API key / GitHub token 永不进 git**。`configs/.env.prod.example` 只放键名与 `CHANGE_ME` 占位;`.env*` 在 `.gitignore`。
- 生产真实密钥只存 lab `/home/rguo/piks/.env`(0600)与 `app_config` 表(页面显示掩码);GitHub token 经 credential helper 读环境变量,不落 `.git/config`。
- 聊天/日志不打印密钥值。

## 当前状态

- ✅ 迭代 0~3:最小闭环 → 可靠性 → 市场情报 → 实体补全(真实数据全链运行)
- ✅ 迭代 4~5:个人学习闭环 + Web 平台(vault/GitHub 已停更,`PIKS-Vault/` 存档)
- ✅ 生产化 P3:lab(192.168.0.202)部署落地,验收全过;Web :8090 常驻
- ✅ P4 能力并入(research 深研)/ P5 前端 IA 个股轴心 / P6 决绝重构 / P7 买入前速评 / P8 手机截图投递 / P9 研报体裁与版面——均已上生产
- ✅ P10 容器拆分(单镜像 → 四镜像,issue #47):`piks-gateway`/`web`/`tools`/`research`;深研改 DB 队列 + 常驻 worker
- ✅ 2026-09-17~09-20 修复批次:换手率口径(#4)、资金面龙虎榜(#26)、实体名去空格(#6)、账户资金汇总(#19)、股票代码掩码(#30)、宏观封面数据源(#13)、消息排序(#37)、来源外链(#38)、容器拆分(#47)
- 📌 最新生产栈:`v0.0.0-616f31b`(gateway `c4377ad` / web `bb80388` / tools `602d148` / research `2426e05`,见 lab `stack-manifest.json`;未发版,版本号恒 `v0.0.0`)
