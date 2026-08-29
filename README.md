# PIKS — Personal Investment Knowledge System

个人 A 股投资知识系统:把公开市场信息(新闻快讯、行情涨停池)自动加工成**结构化事件与实体**,沉淀为个人知识库。**PostgreSQL 是唯一数据源,Web 是界面**(迭代 5 起 PG 直渲 Web,取代 Obsidian/GitHub 界面层)。

## 核心理念

- **Fact ≠ Inference ≠ Belief**:事实层(AI 抽取的事件/实体,含置信度)与你的推断/信念严格分域;卡片"事实"给机器产出,"我的理解"留给你写判断。
- **AI → 结构化输出 → Schema 校验 → 业务校验**:LLM 只产出结构化 JSON,入库前层层校验,不直接写库。
- **数据诚实**:缺失如实标空态(`pending`/`_暂无_`),宁缺毋假;`reconcile` 每日对账,异常不掩盖。

## 一日管线(数据流)

```
migrate → collector(东财 7x24 快讯) → worker(AI 抽取 events) → cluster(语义去重 + 重审视 Pass)
→ quote-collector(涨停池,仅交易日) → entity-build(实体) → market-state(市场情绪)
→ daily-review(每日复盘) → reconcile(对账)
```

9 个管线命令(见 `cmd/`),各自幂等、可单独重跑;失败步骤记录不阻断(下次重试)。
> 迭代 5-2 起:`publisher` / vault / GitHub 下线(Web 直读 PG,管线已无发布步骤)。

## 功能模块

- **每日管线**:新闻→事件抽取→语义去重聚类(含重审视 Pass 修跨簇重复)→涨停池→实体构建→市场情绪→每日复盘→对账,全自动幂等。
- **Web 平台**(`cmd/web`,PG 直渲,lab :8090):看板 / 事件 / 实体 / 图谱(原生 SVG 缩放·拖拽·点选看内容) / 复盘 / 对账 / 笔记(personal_notes 编辑,含事件卡「我的理解」) / 周报(规则聚合 + AI 综述手动触发) / 交易(截图识别录入 + AI 带引用解读 + 持仓 AI 诊断) / AI 对话(问答带引用 + 截图 vision 识别) / 设置(大模型配置)。
- **交易闭环**(2026-08-28,dev):每日自交易截图 → 视觉抽取 → 确认入库;AI 解读带知识库引用、防未来函数;持仓 AI 诊断;本周交易/持仓进周报。**dev-only,未部署 lab**。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.26(静态编译,`go mod vendor` 自包含构建) |
| 数据源 | PostgreSQL 16(唯一 Source of Truth;11 个前向迁移) |
| 界面 | Web(PG 直渲 HTML + 原生 SVG 图谱;Obsidian/GitHub 已下线,`PIKS-Vault/` 仅存档) |
| AI | OpenCode Zen,OpenAI 兼容;**base URL 必须带 `/go` 路由**(`https://opencode.ai/zen/go/v1`);配置存 `app_config` 表(/settings 可编辑),模型分层 extract/reasoning/vision |
| 部署 | Docker Compose(dev 单机 + 生产 lab) |

## 仓库布局

```
cmd/           12 个可执行命令(9 个管线:migrate/collector/worker/cluster/quote-collector/
              entity-build/market-state/daily-review/reconcile + web 常驻服务 + probe 探针 + publisher 遗留)
internal/      业务包(collector/web/store/config/model/...)
migrations/    SQL 迁移(前向,无 down;0001~0011)
prompts/       AI 抽取提示词(extract.md)
configs/       docker-compose(dev/prod)+ .env 模板
scripts/       dev 侧 setup.sh/deploy.sh;lab 侧 pipeline.sh/backup.sh/health.sh
vendor/        自包含构建依赖(镜像构建免网络)
docs/          项目详解、进度总表、各阶段设计定稿 + 实现归档
PIKS-Vault/    Obsidian vault 存档(界面层已下线,不再更新)
```

## 快速开始(dev,本机)

```bash
# 1. 起 postgres(宿主端口 5433,与外部隔离)
docker compose -f configs/docker-compose.yml up -d

# 2. 准备数据库连接(仅此环境变量必需;AI 配置走 app_config 表,见第 5 步)
set -a; source .env.local; set +a   # 键:PIKS_DATABASE_URL / PIKS_LISTEN_ADDR / PIKS_UPLOAD_DIR

# 3. 构建并跑迁移(migrate 会种子 app_config 默认值)
go build -mod=vendor -o bin/ ./cmd/...
./bin/migrate

# 4. 手动跑一次全链(或等生产 crontab 自动;命令均幂等)
./bin/collector -driver dongcai
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

> 生产环境用 `dongcai` 驱动(东方财富 7x24 快讯);`file` 驱动仅迭代 0 保底。
> 大模型配置不再读 `PIKS_AI_*` 环境变量(2026-08-27 起改存 `app_config` 表)。

## 生产部署(lab)

- **模型**:dev 本地编译镜像 → `docker save | ssh lab docker load` 传输;lab 不保留代码仓库,镜像 = 唯一交付物。编排全在 dev 侧。
- **服务**:`postgres`(常驻)+ `web`(常驻,:8090 对 lab 局域网暴露)+ `tools`(profile=run,跑管线命令)。
- **文档**:设计 `docs/phase3/design/prod-deploy.md`(D-P1~P12);实现与验收 `docs/phase3/stages/prod.md`。
- **运维速查**:
  - 更新:`./scripts/deploy.sh`(dev 侧 build → 传镜像 → migrate)
  - 日管线:crontab 每 15min 自判(北京时间非交易日/已过 16:10/今日未跑),stamp 防重跑
  - 备份:每晚 `pg_dump` → `/home/rguo/piks/backups/`,14 天留存
  - 日志:`ssh lab 'tail -50 /home/rguo/piks/logs/pipeline-$(date +%F).log'`

## 文档索引

| 文档 | 内容 |
|---|---|
| `docs/项目详解.md` | 全局架构、数据流、技术栈、边界、执行决策(权威决策登记处) |
| `docs/进度总表.md` | 里程碑跟踪(各阶段状态 + 子阶段明细 + 契约缺口 + 已知遗留) |
| `docs/phase1/` | 迭代 0 地基 + 迭代 1 可靠性(冻结) |
| `docs/phase2/` | 迭代 2~5(增值 + Web 平台)设计定稿 + 实现归档(冻结);G8/聚类质量/周报综述/交易/交易闭环 |
| `docs/phase3/` | 生产化(设计定稿 + 实现验收归档) |
| `PIKS架构设计文档.md` | v1.0 权威架构蓝图(元信息含现状偏差注记) |

## 安全红线

- **API key / GitHub token 永不进 git**。`configs/.env.prod.example` 只放键名与 `CHANGE_ME` 占位;`.env*` 在 `.gitignore`。
- 生产真实密钥只存 lab `/home/rguo/piks/.env`(0600)与 `app_config` 表(页面显示掩码);GitHub token 经 credential helper 读环境变量,不落 `.git/config`。
- 聊天/日志不打印密钥值。

## 当前状态

- ✅ 迭代 0~3:最小闭环 → 可靠性 → 市场情报 → 实体补全(真实数据全链运行)
- ✅ 迭代 4~5:个人学习闭环 + Web 平台(PG 直渲:看/写/理解/周报/AI 对话/截图;vault/GitHub 停更)
- ✅ 生产化 P3:lab(192.168.0.202)部署落地,验收全过;Web :8090 常驻
- ✅ dev 验收全过、**未部署 lab**(随未来镜像重建生效):G8 /chat 语义检索、聚类质量重审视 Pass、周报 AI 综述、交易功能、交易闭环
