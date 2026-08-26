# 生产化 P3 — 部署与运行 设计文档

> 状态:**已定稿冻结(2026-08-27,D-P1~D-P12 用户确认,其中 D-P10=方案 B)**。此后按它执行,改动需走变更。
> 日期:2026-08-27。契约依据:`PIKS架构设计文档.md` §22/§33;`docs/项目详解.md`(技术选型:单机 Docker Compose、任务用 cron + pg 任务表);`docs/进度总表.md` 生产化 P3(原「预留」,本次启动);迭代 0 设计 §10(部署资源评估);2026-08-27 生产机勘察(见 §2)。
> 前置:迭代 0~3 已闭环。目标机:**192.168.0.202(主机名 lab)**。

## 1. 目标与范围

把日频管线从「开发机 WSL2 手动/半自动跑」迁移到**始终在线的生产机 lab**,达成:

1. **定时全自动**:交易日收盘后自动跑完整数据链(采集→抽取→聚类→实体→行情→复盘→发布→对账),无需人工。
2. **数据沉淀在 lab**:PostgreSQL 运行于 lab,跨 dev 机器停机。
3. **Obsidian 侧零代码改动**:vault 仍是同一套双链结构,内容由 lab 生成后推送 GitHub,工作机照旧拉取(方案 B)。
4. **可回滚、可对账、密钥不进 git**:备份 + reconcile + 安全红线保持。

### 范围(本设计覆盖)

lab 上整套运行环境的搭建、调度、备份、更新、同步路径;以及为支撑它新增的仓库文件(Dockerfile / 生产 compose 模板 / 运维脚本)。**不改数据模型、不改业务逻辑**,纯部署与运行层。

### 边界(明确不做)

- ❌ 不引 K8s / 编排平台 / 监控全家桶 —— 单机 Docker + 用户 crontab,维持「任务用 cron」冻结决策。
- ❌ 不做多机 / 高可用 / 负载均衡 —— 个人系统,单机够(迭代 0 §10:1~2 vCPU 即可)。
- ❌ 不在本设计内做前端 / 认证 / 实盘 —— 那是后续阶段。
- ❌ 不假设 lab 有免密 sudo —— **一切以用户态可用为前提**;需要系统级的事(如 NTP)如实标注为风险/待办,不硬做。

## 2. 现状与约束(2026-08-27 勘察事实,只读)

### 2.1 生产机 lab(192.168.0.202)

| 项 | 事实 | 对设计的影响 |
|---|---|---|
| 登录 | rguo 免密 SSH(仅 key,密码方式不通) | 所有运维走 ssh |
| sudo | **无免密 sudo**(`sudo -n` 需交互密码) | ❌ 不能 systemd timer、不能 apt 装包、不能 ufw、不能改系统时区 |
| docker | v29.1.3,`docker ps/run` 可用(rguo 在 docker 组);**无 `docker compose` 插件** | ✅ 跑容器免 sudo;compose 需用户级插件(§6)或裸 docker run(备选) |
| git | 2.53.0 | 代码/vault 操作可用 |
| 网络出站 | github.com 200、opencode.ai 200、push2ex/finance.eastmoney 可达 | 采集/AI/代码拉取全通 |
| 时区/时间 | **Etc/UTC;timesyncd inactive(无 NTP)** | 进程级 `TZ=Asia/Shanghai` 解决;系统 NTP 列为待办(需 sudo) |
| 监听端口 | 仅 22 + localhost DNS | 绿地,无冲突 |
| 磁盘 | 48G,已用 13%(余 ~40G) | 满足(DB 数年 <10G + 镜像 + 备份) |

### 2.2 本地 dev(WSL2 本机)

- `configs/docker-compose.yml`:postgres:16-alpine,容器 `piks-postgres`,宿主端口 **5433**(当前绑 0.0.0.0),named volume `piks_pgdata`。
- 配置走 `.env.local`(gitignored):`PIKS_DATABASE_URL` / `PIKS_AI_BASE_URL`(opencode.ai/zen/go/v1)/ `PIKS_AI_API_KEY` / 模型 / 预算 / `PIKS_VAULT_PATH` / `PIKS_VAULT_REMOTE` 等。`internal/config.Load()` 全环境变量驱动。
- vault:根目录 PIKS-Vault = **Generated 独立仓库**(origin `github.com/glacierzzz26/PIKS-Vault.git`,private,master);`09-Personal` = 嵌套独立仓库(PIKS-Personal.git,private)。两仓互相隔离,同一 Obsidian vault。工作机 Obsidian 从此仓库拉取(方案 B 下保持不变)。
- 代码仓 `glacierzzz26/PIKS`:**PUBLIC**,分支 `dev` → lab 可免密钥 clone/pull。
- Go 1.26.6,单 module,唯一依赖 pgx v5(纯 Go,可静态编译)。
- 10 个可执行命令:migrate / collector / worker / cluster / quote-collector / entity-build / market-state / daily-review / publisher / reconcile(probe 仅 dev 用)。

### 2.3 管线依赖关系(决定调度顺序)

```
migrate(部署时)
collector(新闻→raw) → worker(AI 抽取→events) → cluster(语义去重)
quote-collector(涨停池→observations,仅交易日)
  → entity-build(实体,依赖 zt_pool + events.affected)
  → market-state(→market_snapshots) → daily-review(→02-Market/{date}.md)
publisher(事件卡+实体卡+提交推送;需在 entity-build 之后)
reconcile(对账,放末尾)
```

全部命令**幂等**(content_hash 去重 / upsert / md5 跳写 / git 零提交),所以调度重跑安全。

## 3. 生产拓扑

```
                    ┌──────────── 生产机 lab 192.168.0.202 ────────────┐
工作机 WSL2         │  /home/rguo/piks/                                     │
  Obsidian          │   ├─ docker-compose.yml(prod,静态,非代码仓内)     │
  (Generated)       │   ├─ .env(0600,真实密钥,永不进 git)               │
     ▲   git pull   │   ├─ piks/             ←代码仓 clone(dev,PUBLIC) │
     │   GitHub     │   ├─ vault/            Generated 工作仓(容器内)   │
     └──────────────│   ├─ backups/  logs/  scripts/                   │
         ▲          │   │                                              │
         │ push     │   │  docker:                                      │
      GitHub 私有仓  │   │   piks-postgres(16-alpine)⇄ piks-tools(全部cmd)│
      PIKS-Vault    │   │   网络 piks(内部);5433 只绑 127.0.0.1         │
                    │   │                                               │
                    │   出站:eastmoney / opencode.ai / github.com       │
                    └──────────────────────────────────────────────────┘
```

- **Postgres = 唯一可信源**(不变);vault 是投影,可整体再生成。
- 工具镜像内运行所有命令(静态二进制 + git + ca-cert + tzdata),一次构建。
- **vault 推送走 GitHub 私有仓(方案 B,已定)**:lab 容器内生成 → push 到 PIKS-Vault → 工作机照旧 pull。零工作机改动。

## 4. 决策清单(D-P1~D-P12)—— 已定稿

| # | 决策点 | 结论 | 备选 | 依据 |
|---|---|---|---|---|
| D-P1 | 部署形态 | 单机 Docker + **用户 crontab** | — | 无免密 sudo → 系统 timer 不可行;docker 组 + 用户 crontab 全免 sudo |
| D-P2 | 编排方式 | **compose**:用户级装插件 `~/.docker/cli-plugins/docker-compose`(免 sudo,一次性) | 裸 `docker run`(零安装,脚本内写死网络/卷) | dev 已用 compose;同一份配置单一来源 |
| D-P3 | 时区 | 容器级 `TZ=Asia/Shanghai`(所有进程);lab 系统时区不改;NTP 系统级待办 | — | 非交易日判定/快照 trade_date/复盘页日期全是「本地日」,UTC 会错 8h;无 sudo 改不了系统 |
| D-P4 | 代码来源 | lab `git clone` 公开仓 `origin/dev`,免密钥 | rsync 从 dev | PIKS 仓 PUBLIC;跟踪 dev 与发布线一致 |
| D-P5 | 构建 | **dev 本地编译** → `docker save\|ssh docker load` 传输 lab(**用户 2026-08-27 指示变更**);lab 不保留代码仓库 | lab 本地 build(已弃) | 实测 lab 拉 GitHub git 协议不稳(HTTP2 framing / 超时);dev 编译最稳,镜像唯一交付物 |
| D-P6 | 配置/密钥 | 提交 `configs/.env.prod.example`(无真实值);lab `/home/rguo/piks/.env`(0600);**密钥永不进 git、不打印** | — | 安全红线(不变) |
| D-P7 | 数据库 | compose 内 postgres 容器 + named volume;migrate 容器内跑;初始数据 **pg_dump dev→lab 一次性同步** | 空库重采 | 迁移现有真实数据(事件/实体/快照/来源) |
| D-P8 | 调度 | 用户 crontab 每 15min 触发 `pipeline.sh`;**脚本内用北京时间判定**「交易日 + 已过 16:10 + 今日未跑」;stamp + 幂等防重跑 | crontab 写死 `CRON_TZ` 与具体时刻 | cron 用宿主时区(UTC),写死时刻会错 8h;脚本判定免疫 |
| D-P9 | 备份 | 每晚 `pg_dump -Fc` → `/home/rguo/piks/backups/`,保留 14 天;可选 rsync 副本到工作机 | — | DB 是唯一需落盘备份的资产;vault 在 git(GitHub)天然备份 |
| D-P10 | vault 同步 | **方案 B(已确认):lab 直接推 GitHub** 私有 PIKS-Vault;工作机零改动 | 方案 A LAN 裸仓(已弃) | 用户确认 B;工作机 Obsidian 流程不变;PAT 只存 lab `.env` |
| D-P11 | 健康/对账 | 管线末尾 `reconcile`(已有)+ 日志落 `/home/rguo/piks/logs/`;**stamp 缺失/对账异常 → 当日 report 标注**,不掩盖 | 邮件/推送(后续) | 数据诚实原则;个人系统先可观察 |
| D-P12 | 更新/回滚 | **dev 侧 `deploy.sh`**:build → 镜像传 lab → 起 postgres → migrate → 触发管线(§11);回滚 = **备份恢复 DB + dev checkout 旧 commit 重传镜像**(迁移只前向) | — | 迁移无 down;DB 回滚靠备份;镜像随 commit 可重建 |

> 定稿门已过(2026-08-27):D-P1~D-P12 用户确认,D-P10 选定**方案 B**。此后按本设计执行。

## 5. 目录与文件布局

### 5.1 仓库新增(全部进代码仓,dev 分支)

```
Dockerfile                    # 多阶段构建全部 cmd(§6)
configs/docker-compose.prod.yml  # 生产 compose 模板(§8;lab 实例化为 /home/rguo/piks/docker-compose.yml)
configs/.env.prod.example     # 生产配置模板,仅键名与占位,无真实值(§7)
scripts/pipeline.sh           # 日管线(§9,lab 侧)
scripts/backup.sh             # 每晚备份(§8.4,lab 侧)
scripts/deploy.sh             # 更新部署(§11,dev 侧:build→传镜像→migrate)
scripts/setup.sh              # lab 一次性初始化(§14,dev 侧)
scripts/health.sh             # 可选:完整性自检(§12,lab 侧)
```

### 5.2 lab 运行布局(非代码仓)

```
/home/rguo/piks/
├── docker-compose.yml   # 由模板实例化(静态,含绝对路径)
├── .env                 # 0600,生产真实配置(含 PIKS_VAULT_GIT_TOKEN)
├── vault/               # Generated 工作仓(容器内 /srv/vault,origin=GitHub 私有仓)
├── backups/             # piks-YYYY-MM-DD.dump
├── logs/                # pipeline-YYYY-MM-DD.log + stamp
└── scripts/             # 从 dev 同步的 3 个 lab 侧脚本(pipeline/backup/health)
```

## 6. 镜像与构建(D-P5)

`Dockerfile`(仓库根,**dev 本地构建**):

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor          # 自包含构建,免模块下载(实测 proxy 可达性不稳)
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -o /out/bin/ ./cmd/...

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git tzdata
ARG GIT_SHORT=unknown
ENV PIKS_GIT_SHORT=${GIT_SHORT}   # 容器内无 .git,血缘字段取烘焙值
WORKDIR /app
COPY --from=build /src/migrations /app/migrations
COPY --from=build /src/prompts /app/prompts
COPY --from=build /out/bin/ /app/bin/   # 与 dev bin/ 布局一致,脚本统一 ./bin/<cmd>
ENTRYPOINT []
```

- 一次构建 10 个命令(`migrate collector worker cluster quote-collector entity-build market-state daily-review publisher reconcile`)。
- 静态链接(无 CGO)→ 直接跑 alpine,运行镜像 ~50MB。
- **构建在 dev 执行**(`scripts/deploy.sh`),`docker save | ssh lab docker load` 传输;`PIKS_GIT_SHORT` 随构建烘焙血缘。
- 运行镜像内**不含源码/密钥**;密钥只经 `/home/rguo/piks/.env` 注入。
- 依赖 `go mod vendor`(仓库内 `vendor/`),避免镜像构建时访问 proxy.golang.org。

## 7. 配置与密钥(D-P6)

### 7.1 `.env.prod.example`(提交,无真实值)

```
# 生产配置模板:lab 实例化为 /home/rguo/piks/.env(0600)。真实值手工填,永不进 git。
# 用法:set -a; source /home/rguo/piks/.env; set +a(脚本内)
PIKS_DB_PASSWORD=CHANGE_ME            # postgres 容器口令
# 容器内通过服务名访问(postgres 在 piks 网络内)
PIKS_DATABASE_URL=postgres://piks:CHANGE_ME@postgres:5432/piks?sslmode=disable
PIKS_AI_BASE_URL=https://opencode.ai/zen/go/v1
PIKS_AI_API_KEY=CHANGE_ME
PIKS_AI_MODEL_EXTRACT=deepseek-v4-flash
PIKS_AI_MODEL_REASONING=deepseek-v4-flash
PIKS_AI_DAILY_TOKEN_BUDGET=0
PIKS_VAULT_PATH=/srv/vault             # 容器内挂载点
# 方案 B:推 GitHub 私有仓(origin=该 URL);token 只存本 .env,由 git credential helper 注入
PIKS_VAULT_REMOTE=https://github.com/glacierzzz26/PIKS-Vault.git
PIKS_VAULT_GIT_TOKEN=CHANGE_ME
PIKS_VAULT_GIT_NAME=glacierzzz26
PIKS_VAULT_GIT_EMAIL=glacierzzz26@gmail.com
```

### 7.2 密钥纪律(红线,不变)

- 真实 `.env` 只存在于 lab `/home/rguo/piks/.env`,权限 **0600**。
- 代码仓、vault 仓、镜像、聊天:一律不出现真实密钥值。
- **GitHub token(PIKS_VAULT_GIT_TOKEN)只进 `/home/rguo/piks/.env`**,不进 vault 的 `.git/config`(用 credential helper 在 push 时读环境变量,见 §10)。
- dev 的 `.env.local` 与 lab 的 `/home/rguo/piks/.env` 各自独立,互不复制;AI key 两侧一致与否由用户决定(生产用同一 provider)。

## 8. 数据库(D-P7)

### 8.1 生产 compose(模板 `configs/docker-compose.prod.yml`)

```yaml
services:
  postgres:
    image: postgres:16-alpine
    container_name: piks-postgres
    environment:
      POSTGRES_USER: piks
      POSTGRES_PASSWORD: ${PIKS_DB_PASSWORD:-piks_dev_password}
      POSTGRES_DB: piks
    ports: ["127.0.0.1:5433:5432"]      # 只绑回环,不对外
    volumes: [piks_pgdata:/var/lib/postgresql/data]
    healthcheck: { test: ["CMD-SHELL","pg_isready -U piks -d piks"], interval: 5s, timeout: 3s, retries: 10 }
    restart: unless-stopped
    networks: [piks]
  tools:
    image: piks-tools:latest
    profiles: ["run"]                    # 不随 up 常驻,只 compose run --rm 用
    env_file: [".env"]
    environment: [TZ=Asia/Shanghai]
    volumes:
      - /home/rguo/piks/vault:/srv/vault
    networks: [piks]
    command: ["sh","-c","true"]
networks: { piks: {} }
volumes: { piks_pgdata: {} }
```

- `docker compose up -d` 只起 postgres;所有命令 `docker compose run --rm tools ./bin/<cmd>`。
- 模板在代码仓,lab 实例化到 `/home/rguo/piks/docker-compose.yml`(setup 一次,不随每次 deploy 覆盖)。
- vault 工作仓只挂载进容器(容器内 `/srv/vault`,git 操作发生在挂载目录,配置持久)。

### 8.2 迁移

- `docker compose run --rm tools ./bin/migrate`(幂等,schema_migrations 跟踪;`migrations/` 4 个文件已随镜像进容器)。

### 8.3 初始数据同步(一次)

```
# dev(WSL2)侧导出
docker compose exec -T postgres pg_dump -U piks -d piks -Fc > piks-initial.dump
# 传到 lab 并恢复(restore 到新库)
scp piks-initial.dump rguo@192.168.0.202:/home/rguo/piks/
ssh rguo@192.168.0.202 'docker compose -f /home/rguo/piks/docker-compose.yml exec -T postgres pg_restore -U piks -d piks --clean --if-exists < /home/rguo/piks/piks-initial.dump'
```

- 带回 sources / raw_documents / events / evidences / relationships / observations / market_snapshots / entities / task_runs。
- 恢复后 migrate 幂等跑一次收尾;reconcile 校验同步完整(验收 §15)。

### 8.4 备份(D-P9)

`backup.sh`:

```bash
#!/usr/bin/env bash
set -uo pipefail
export TZ=Asia/Shanghai
C=/home/rguo/piks
docker compose -f $C/docker-compose.yml exec -T postgres \
  pg_dump -U piks -d piks -Fc > $C/backups/piks-$(date +%F).dump
find $C/backups -name 'piks-*.dump' -mtime +14 -delete
echo "backup done $(date +%F\ %T)"
```

- 容器内置 pg_dump,无需额外依赖。
- 保留 14 天;vault 在 git(GitHub)已天然备份。
- 可选:每日 rsync 一份到工作机做 offsite(见 §16 待办)。

## 9. 调度(D-P8)

### 9.1 `pipeline.sh`(脚本内判定,免疫宿主时区)

```bash
#!/usr/bin/env bash
set -uo pipefail
export TZ=Asia/Shanghai
C=/home/rguo/piks; LOG=$C/logs
TODAY=$(date +%F); DOW=$(date +%u); HMS=$(date +%H%M)
[ -f $LOG/pipeline-$TODAY.done ] && exit 0    # 今日已跑
[ "$DOW" -ge 6 ] && exit 0                     # 周末
[ "$HMS" -lt 1610 ] && exit 0                  # 未过收盘后(16:10 放行)
cd $C
run(){ echo "== $(date '+%F %T %Z') $*" | tee -a $LOG/pipeline-$TODAY.log; docker compose run --rm tools ./bin/"$@" >> $LOG/pipeline-$TODAY.log 2>&1; }
run migrate
run collector
run worker
run cluster
run quote-collector
run entity-build
run market-state
run daily-review
run publisher
run reconcile
touch $LOG/pipeline-$TODAY.done
```

- 节假日(周一~五休市):quote-collector 已内置「qdate==当日 才视为交易日」→ 自动跳过,不产生脏快照;collector/worker 照跑(新闻照采,无害)。
- 命令全部幂等 → 即使真重跑也零副作用。
- 日志按日落盘;stamp 文件即「今日已跑」凭证。

### 9.2 用户 crontab(免 sudo)

```
# crontab -e(rguo)
HOME=/home/rguo
SHELL=/bin/bash
PATH=/usr/bin:/bin
*/15 * * * * /home/rguo/piks/scripts/pipeline.sh >> /home/rguo/piks/logs/cron.log 2>&1
59 23 * * *   /home/rguo/piks/scripts/backup.sh  >> /home/rguo/piks/logs/cron.log 2>&1
```

- 每 15min 触发,`pipeline.sh` 自判「时间 + 交易日 + 是否已跑」;16:10 后第一次触发即执行全链。
- 备份独立一条 23:59(北京时间由脚本内 TZ 生效)。
- 备选:`CRON_TZ=Asia/Shanghai` + 具体时刻;不推荐(依赖 cron 版本对该变量的支持)。

## 10. Vault 生成与同步(D-P10,方案 B)

### 选定方案:B — lab 直接推 GitHub 私有仓

```
setup 一次性:
  git init /home/rguo/piks/vault
  git -C /home/rguo/piks/vault remote add origin $PIKS_VAULT_REMOTE   # https://github.com/glacierzzz26/PIKS-Vault.git
  git -C /home/rguo/piks/vault config credential.helper \
    '!f(){ echo username=glacierzzz26; echo password=${PIKS_VAULT_GIT_TOKEN}; }; f'   # token 只从环境读,不落盘
  # 首次拉取 GitHub 现有历史(避免首次 push 非快进冲突;Generated 内容可再生,仅需基线)
  git -C /home/rguo/piks/vault pull --rebase --autostash origin master   # 需 token(同上 helper)
容器内 publisher(每次):
  对 /srv/vault 写卡 → git add/commit → PIKS_VAULT_REMOTE 非空 → git push(credential helper 注入 token)
  → GitHub 私有仓更新
工作机(零改动):
  照旧 git pull(origin=GitHub),Obsidian 打开同一目录
```

- token **只在 push/pull 时经 credential helper 从进程环境读取**,永不写入 `.git/config`。
- `CommitVaultWithMsg` 在 remote 非空时 `git push -q`;推送失败仅记录不阻断(已有行为),当日重跑幂等。
- **09-Personal**(PIKS-Personal.git,private)仍只在工作机维护,不迁移;Obsidian 双链不受影响。
- 方案 A(LAN 裸仓)已评估并弃用(需工作机改 remote + 无外部备灾),此处仅存档不执行。

### 备灾说明

- vault 是 Postgres 的投影,数据在 DB(有备份),vault 仓损坏可直接重建;GitHub 只是分发通道。

## 11. 更新与回滚(D-P12)

**deploy 在 dev 侧执行**(`scripts/deploy.sh`,镜像为唯一交付物):

```bash
#!/usr/bin/env bash
# dev 侧
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
LAB="${PIKS_LAB:-rguo@192.168.0.202}"
GS="$(git -C "$REPO" rev-parse --short HEAD)"
docker build --build-arg GIT_SHORT="$GS" -t piks-tools:latest "$REPO"
docker save piks-tools:latest | ssh "$LAB" docker load
ssh "$LAB" 'docker compose -f /home/rguo/piks/docker-compose.yml up -d postgres && \
  docker compose -f /home/rguo/piks/docker-compose.yml run --rm tools ./bin/migrate'
```

- 之后 `pipeline.sh` 由 cron 自然触发,或手动跑一次立即生效。
- **回滚**:数据=从昨晚/当日备份 `pg_restore --clean` 恢复;代码=dev `git checkout <旧commit>` + 重跑 deploy 传旧镜像 + migrate 前滚(迁移无 down,旧代码+新表若有兼容问题以备份为准)。
- pipeline 血缘字段 `@<git-short>` 随 dev HEAD 前进 → 每次 deploy 后卡片 lineage 更新(已确认是诚实预期行为)。

## 12. 健康 / 对账 / 日志(D-P11)

- **reconcile 即日检**(已有):异常写入 `00-System/recon-{date}.md` + task_runs meta,结论如实「通过/需关注」。
- **管线完整性**(`health.sh`,可选):
  - 检查最近交易日(北京时间周一~五)的 `pipeline-<date>.done` 是否存在;
  - 检查最新 recon 结论;
  - 结果一行写入 `logs/health.log`,并在 `00-System/` 留痕(可并入 recon 报告,实现时定)。
- 日志:`logs/pipeline-*.log` / `cron.log`,find -mtime +30 清理(backup.sh 顺带或独立)。
- 不做:邮件/推送告警(个人系统,V1 先可观察,后续迭代)。

## 13. 安全面(D-P1/P7)

- postgres 端口只绑 `127.0.0.1:5433`,**不对外**(lab 上由容器网络+回环端口双隔离)。
- 对外仅留 22(SSH);出站按需(eastmoney/opencode/github)。无 sudo 不开 ufw,靠「不暴露端口」实现最小面。
- 密钥只存 `/home/rguo/piks/.env`(0600):AI key + GitHub token;镜像/代码/仓库零密钥;聊天零密钥。
- ssh 仅 key 认证(现状),禁密码登录建议列入待办(需 sudo)。

## 14. 一次性初始化(setup.sh)

**模型(实现期修正):所有编排在 dev 侧执行**;lab 只收镜像/compose/脚本/基线,不保留代码仓库、不 clone GitHub。由 `scripts/setup.sh`(dev 侧,幂等可重跑)执行,共 10 步:

1. **目录 + 传输**:`mkdir -p /home/rguo/piks/{vault,backups,logs,scripts}`;scp `configs/docker-compose.prod.yml` → `docker-compose.yml`,scp lab 侧 3 脚本 `pipeline.sh/backup.sh/health.sh` → `scripts/` 并 `chmod +x`。
2. **.env 校验**:lab `/home/rguo/piks/.env` 已存在且不含 `CHANGE_ME`(部署者先按 `.env.prod.example` 填真实值);`chmod 600`。
3. **compose 插件**:从 dev 拷贝二进制 `/usr/libexec/docker/cli-plugins/docker-compose` → lab `~/.docker/cli-plugins/docker-compose`(免网络下载,版本随 dev 锁定)。
4. **镜像构建 + 传输**:调 `scripts/deploy.sh` —— dev `docker build`(bake GIT_SHORT)→ `docker save | ssh lab docker load` → `docker compose up -d postgres` → `run --rm tools ./bin/migrate`。
5. **postgres 就绪**:`until pg_isready` 轮询确认。
6. **migrate**:`run --rm tools ./bin/migrate`(幂等,重复执行安全)。
7. **vault 工作仓**:若 `vault/.git` 不存在,`rsync -a --delete --exclude 09-Personal --exclude .obsidian` 把 dev `PIKS-Vault/Generated` 基线同步过去(避免 lab 拉 GitHub 私有仓);设 origin = GitHub 私有仓 + credential.helper 读 `PIKS_VAULT_GIT_TOKEN` 环境变量(不落 `.git/config`)。
8. **crontab(幂等)**:pipeline 每 15 分钟 + backup 每日 23:59,追加写 `logs/cron.log`(先去掉旧行再重写,可重跑不重复)。
9. **时间核对**:`run --rm tools date` 应显示 Asia/Shanghai;系统 NTP 状态如实记录(风险项,见 §16 待办)。
10. **初始数据同步(单独执行)**:§8.3 的 pg_dump dev→lab 由部署者显式执行,不在 setup.sh 内。

> 说明:postgres 常驻、tools 按需 `run --rm`;lab 上镜像 = 交付物,更新=dev 重 build→重 load(§11)。

## 15. 验收清单

| # | 标准 | 判定 |
|---|---|---|
| §15.1 | postgres 容器运行,`pg_isready` ok;5433 仅回环监听 | `ss -ltn` 无 0.0.0.0:5433 |
| §15.2 | 初始数据同步:lab 端 events/entities/market_snapshots 计数与 dev 一致;reconcile 异常=0 | 计数 + 对账 |
| §15.3 | 首次全链:collector 拉新闻、worker 抽取、publisher 生成卡片并 push 到 GitHub 私有仓;工作机 `git pull` 无新历史冲突 | vault 日志 + 工作机 pull |
| §15.4 | 幂等:当日重跑全链 → 零新提交、无重复处理(task_runs 不翻倍) | 重跑比对 |
| §15.5 | 非交易日:周六触发 → market-state 跳过、无快照;collector 不报错 | 日志 |
| §15.6 | 备份:`backup.sh` 产 `.dump`;恢复到一个临时空库可读(验证一次) | 恢复演练 |
| §15.7 | 密钥:lab `.env` 0600;代码仓/vault 仓 grep 无真实 key;`.env.prod.example` 无真实值;vault `.git/config` 无 token | grep + ls -l |
| §15.8 | 时区:容器 `date` 显示 Asia/Shanghai;复盘页/快照 trade_date 为北京日期 | date + 抽查 |
| §15.9 | 调度:stamp 缺失时 16:10 后触发全链;已有 stamp 时零动作 | cron 日志 |
| §15.10 | 安全面:无对外 5433;`docker ps` 仅 postgres 常驻 | ss -ltn + docker ps |

## 16. 决策登记与待办

### 决策登记(已定稿)

| 决策点 | 结论 | 确认日期 |
|---|---|---|
| D-P1~D-P12 | 见 §4;D-P10 = 方案 B(lab 直推 GitHub) | 2026-08-27 |

### 实现期修正(2026-08-27)

- **运行根目录 `/srv/piks` → `/home/rguo/piks`**:实测 rguo 无 sudo,`/srv` 为 root 属主不可建目录;迁到用户家目录。其余布局/脚本不变。
- **构建模型改为 dev 本地编译 + 镜像传输**(用户明确「本地编译,把镜像拷贝过去」):lab 不再 clone 代码仓库、不再 lab 侧 `docker build`。交付物 = 镜像(`docker save | ssh docker load`)+ compose + 3 个 lab 侧脚本。`deploy.sh`/`setup.sh` 均为 dev 侧执行;§4 D-P5/D-P12、§6、§11、§14 已同步更新。
- **vault 基线从 dev rsync 而非 lab clone GitHub**:lab 拉 GitHub 私有仓实测 HTTP2 framing error / 超时(即使 `http.version HTTP/1.1` 仍极慢);改由 dev 把 `PIKS-Vault/Generated` 基线(排除 09-Personal/.obsidian)rsync 到 lab,lab 侧只设 origin + env 凭据 helper,首次 push 仍由 publisher 完成(§10 方案 B 不变)。
- **compose 插件从 dev 拷贝**:lab 无 compose 插件,`docker-compose` 二进制由 dev scp 到 `~/.docker/cli-plugins/`,免 GitHub 下载。

### 待办(需 sudo / 后续)

- [ ] 系统 NTP/chrony 同步(时间戳是核心数据;当前 timesyncd inactive,进程级 TZ 只解决显示、不解决漂移)——需用户提供一次 sudo 或 lab 管理员开启。
- [ ] ssh 禁密码登录(可选加固,需 sudo)。
- [ ] 可选:备份 offsite rsync 到工作机。
- [ ] 可选:盘中二次采集(如 09:30 一次新闻链),V1 不做。
- [ ] 可选:GitHub 备灾镜像以外的进一步冗余(DB 备份即够)。
