# PIKS — Personal Investment Knowledge System

个人 A 股投资知识系统:把公开市场信息(新闻快讯、行情涨停池)自动加工成**结构化事件与实体**,沉淀为个人知识库。**PostgreSQL 是唯一数据源,Markdown 是投影,Obsidian 是界面。**

## 核心理念

- **Fact ≠ Inference ≠ Belief**:事实层(AI 抽取的事件/实体,含置信度)与你的推断/信念严格分域;卡片"事实"给机器产出,"我的理解"留给你写判断。
- **AI → 结构化输出 → Schema 校验 → 业务校验**:LLM 只产出结构化 JSON,入库前层层校验,不直接写库。
- **数据诚实**:缺失如实标空态(`pending`/`_暂无_`),宁缺毋假;`reconcile` 每日对账,异常不掩盖。

## 一日管线(数据流)

```
migrate → collector(东财 7x24 快讯) → worker(AI 抽取 events) → cluster(语义去重)
→ quote-collector(涨停池,仅交易日) → entity-build(实体) → market-state(市场情绪)
→ daily-review(每日复盘) → publisher(卡片 → Obsidian vault → GitHub 私有仓) → reconcile(对账)
```

10 个可执行命令(`cmd/`),各自幂等、可单独重跑;失败步骤记录不阻断(下次重试)。

## 技术栈

| 层 | 选型 |
|---|---|
| 语言 | Go 1.26(静态编译,`go mod vendor` 自包含构建) |
| 数据源 | PostgreSQL 16(唯一 Source of Truth) |
| 界面 | Obsidian vault(Markdown + wikilink 双链;Generated/Personal 双仓库) |
| AI | OpenCode Zen,OpenAI 兼容;**base URL 必须带 `/go` 路由**(`https://opencode.ai/zen/go/v1`) |
| 部署 | Docker Compose(dev 单机 + 生产 lab) |

## 仓库布局

```
cmd/           10 个可执行命令(migrate/collector/worker/cluster/quote-collector/
              entity-build/market-state/daily-review/publisher/reconcile)
internal/      业务包(collector/publish/store/config/model/...)
migrations/    SQL 迁移(前向,无 down)
prompts/       AI 抽取提示词(extract.md)
configs/       docker-compose(dev/prod)+ .env 模板
scripts/       dev 侧 setup.sh/deploy.sh;lab 侧 pipeline.sh/backup.sh/health.sh
vendor/        自包含构建依赖(镜像构建免网络)
docs/          项目详解、进度总表、各阶段设计定稿 + 实现归档
PIKS-Vault/    Obsidian vault(Generated 独立仓库,含内嵌 09-Personal 个人仓库)
```

## 快速开始(dev,本机)

```bash
# 1. 起 postgres(宿主端口 5433,与外部隔离)
docker compose -f configs/docker-compose.yml up -d

# 2. 准备环境变量:本机维护 `.env.local`(不进 git)。键名:
#    PIKS_DATABASE_URL / PIKS_AI_BASE_URL / PIKS_AI_API_KEY /
#    PIKS_AI_MODEL_EXTRACT / PIKS_AI_MODEL_REASONING
set -a; source .env.local; set +a

# 3. 构建并跑迁移
go build -mod=vendor -o bin/ ./cmd/...
./bin/migrate

# 4. 手动跑一次全链(或等生产 crontab 自动)
./bin/collector -driver dongcai
./bin/worker
./bin/cluster
./bin/quote-collector
./bin/entity-build
./bin/market-state
./bin/daily-review
./bin/publisher
./bin/reconcile
```

> 生产环境用 `dongcai` 驱动(东方财富 7x24 快讯);`file` 驱动仅迭代 0 保底。

## 生产部署(lab)

- **模型**:dev 本地编译镜像 → `docker save | ssh lab docker load` 传输;lab 不保留代码仓库,镜像 = 唯一交付物。编排全在 dev 侧。
- **文档**:设计 `docs/phase3/design/prod-deploy.md`(D-P1~P12);实现与验收 `docs/phase3/stages/prod.md`。
- **运维速查**:
  - 更新:`./scripts/deploy.sh`(dev 侧 build → 传镜像 → migrate)
  - 日管线:crontab 每 15min 自判(北京时间非交易日/已过 16:10/今日未跑),stamp 防重跑
  - 备份:每晚 `pg_dump` → `/home/rguo/piks/backups/`,14 天留存
  - 日志:`ssh lab 'tail -50 /home/rguo/piks/logs/pipeline-$(date +%F).log'`

## 文档索引

| 文档 | 内容 |
|---|---|
| `docs/项目详解.md` | 全局架构、数据流、技术栈、边界、执行决策 |
| `docs/进度总表.md` | 里程碑跟踪(各阶段状态 + 子阶段明细 + 契约缺口) |
| `docs/phase1/` | 迭代 0 地基 + 迭代 1 可靠性(冻结) |
| `docs/phase2/` | 迭代 2 市场情报 + 迭代 3 实体补全(冻结);迭代 4 待设计 |
| `docs/phase3/` | 生产化(设计定稿 + 实现验收归档) |
| `PIKS-Vault/09-Personal/PIKS-Obsidian-使用教程.md` | Obsidian 端使用教程(个人仓库内) |

## 安全红线

- **API key / GitHub token 永不进 git**。`configs/.env.prod.example` 只放键名与 `CHANGE_ME` 占位;`.env*` 在 `.gitignore`。
- 生产真实密钥只存 lab `/home/rguo/piks/.env`(0600);GitHub token 经 credential helper 读环境变量,不落 `.git/config`。
- 聊天/日志不打印密钥值。

## 当前状态

- ✅ 迭代 0~3:最小闭环 → 可靠性 → 市场情报 → 实体补全(真实数据全链运行)
- ✅ 生产化 P3:lab(192.168.0.202)部署落地,验收 §15 全过
- ⬜ 迭代 4:个人学习闭环(Belief/Case/Mistake、harvest 回写、周报)
