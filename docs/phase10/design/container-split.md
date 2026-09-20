# 容器按功能拆分:单镜像 → 四镜像

> 阶段 P10。对应 issue **#47**。状态:**已定稿并实施**(2026-09-20),已上生产。
> 关联决策:D-2(2026-09-12 单镜像,本阶段取代)、D-7(DB 隔离)、D-11(research 独立迭代)。

## 1. 问题

`piks-tools:latest` 一个镜像(934MB)同时背四种职责:nginx 网关 + Go web API + 12 个批处理 CLI
+ Python 深研运行时。后果:

- **改前端 → 整镜像重建**,且 `RUN go build ./cmd/...` **从不命中缓存** —— Dockerfile 用
  `COPY . .` 把整个仓库(含 `frontend/`)灌进 Go 阶段,改一行 `frontend/src/main.tsx` 即击穿
  Go 层、13 个二进制全量重编(实测 **27s**)。
- **改 Python 深研 → 整镜像重建 + 重启 web**。
- 每次部署 `docker save | ssh docker load` 传整镜像 ~216MB,无法按改动范围裁剪。

## 2. 目标形态

单 `Dockerfile`、多 `--target`、四镜像(而非四个 Dockerfile —— Go 与前端构建阶段在四镜像间
完全共享,拆成四个文件会复制 4 份 `go mod download` + `go build`,且版本漂移无单一真源):

```
FROM golang:1.26-alpine AS build          # 共享:go build ./cmd/... → /out/bin/
FROM node:20-alpine    AS frontend        # 共享:npm run build → dist

FROM nginx:alpine      AS gateway         # COPY nginx.conf + --from=frontend dist
FROM alpine:3.20       AS web             # COPY --from=build bin/web
FROM alpine:3.20       AS tools           # COPY 9 bins + migrations + prompts
FROM python:3.12-slim  AS research        # pip + research/ + research-run + research-worker
```

| 镜像 | 实测 save 体积 | 升级触发条件 |
|---|---|---|
| `piks-gateway` | 25MB | 前端 / nginx.conf 变 |
| `piks-web` | 8MB | `cmd/web` / `internal` 变 |
| `piks-tools` | 42MB | `cmd` / `internal` / `migrations` / `prompts` 变 |
| `piks-research` | 139MB | `research/` / `internal/research` 变 |

### 2.1 缓存击穿的修法

`build` 阶段只 `COPY go.mod go.sum cmd/ internal/`,**不含 `frontend/`**。于是改前端只重建
`frontend` 阶段与 `gateway` 镜像,Go 层是 cache hit。这是 issue #47 的核心诉求。

> ⚠️ legacy builder(本机 Docker 29,无 buildx)支持 `--target`,但**无 `--mount=type=cache`**:
> Go 阶段自身源码变动时仍会重编(不可避免)。本期只消除「前端改动击穿 Go 层」;builder cache
> 需先装 buildx,**不在本期范围**。**禁写 `# syntax=` 指令**(需 BuildKit,会硬失败)。

> ⚠️ **元数据层必须排在网络层之后**(issue #57,2026-09-20 修):research 阶段的
> `ARG GIT_SHORT`/`ENV PIKS_GIT_SHORT` 曾被排在 `pip install` **之前**,而 `GIT_SHORT`
> 每次提交都变 → 每次部署都击穿 pip 层、重下全部依赖(几十 MB,实测把构建拖到数十分钟)。
> 现移至 `pip install` 之后:只要 `requirements.txt` 不变,pip 层永久命中(实测 `GIT_SHORT`
> 由 `test` 改 `zzz9` 重建,pip 层 `Using cache`、层 ID 不变)。**凡「每次构建都变」的
> ARG/ENV,一律放到最后一个网络操作之后** —— 这是 §2.1「缓存击穿」在**层序**上的同一条原则。

### 2.2 tools 逐条 COPY

`tools` target **逐条** `COPY --from=build /out/bin/<cmd>`,不用
`COPY --from=build /out/bin/ /app/bin/`:后者会把 `web`/`research-run`/`research-worker`/`probe`
也塞进 tools,既白占体积,又让 `run --rm tools ./bin/web` 这类误用成为可能。逐条列 = 内容
显式可审计,拓扑检查可断言。

### 2.3 镜像内烘焙标识

四 target 各 `ENV PIKS_VERSION` + `PIKS_GIT_SHORT` + **`PIKS_IMAGE_ROLE`**
(`gateway`/`web`/`tools`/`research`,供容器自证身份)。

> ⚠️ `ARG GIT_SHORT`/`ARG PIKS_VERSION` 必须声明在**首个 `FROM` 之前**(全局作用域):在阶段内
> 声明的 ARG 只对该阶段可见,后续阶段的 `ARG GIT_SHORT` 取不到它的默认值(会是空串而非
> `unknown`)。此坑在实施时由 `deploy.sh` 的干净树门控间接挡下后修正。

## 3. 深研触发:进程内 goroutine → DB 队列 + 常驻 worker

这是唯一改变**运行时行为**的改动。

**为什么要改**:web 容器拆分后**无 python3**。若仍在 web 进程内 `os/exec python3`,就是
2026-09-12 生产事故的复现(`exec: python3: not found`,UI 深研必 failed —— 该事故记录于
`docs/phase4/stages/research-merge.md`)。故触发必须离开 web 进程。

### 3.1 队列原语(无需新表,仅加两列)

`migrations/0015_research_run_queue.sql`:

```sql
ALTER TABLE research_runs ADD COLUMN IF NOT EXISTS quick BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE research_runs ADD COLUMN IF NOT EXISTS days  INT     NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_research_runs_pending   ON research_runs(created_at) WHERE status='pending';
CREATE INDEX IF NOT EXISTS idx_research_runs_heartbeat ON research_runs(updated_at) WHERE status IN (...);
```

`internal/store/research_runs.go` `ClaimPendingResearchRun`:

```sql
UPDATE research_runs SET status='gathering', error=NULL, updated_at=now()
WHERE run_id = (
  SELECT run_id FROM research_runs WHERE status='pending'
  ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED
) AND status='pending'
RETURNING <researchRunCols>
```

- 单条语句原子领取;`SKIP LOCKED` 天然支持多副本。
- **并发安全**:实测 4 goroutine × 8 行,每行恰好被领一次。

**为什么把 `quick`/`days` 落库**:worker 只拿得到 `research_runs` 这一行。不落库则认领后
无从还原 `RequireSynthesis`(quick 语义)与回溯天数。备选「复用 profile」会污染档案语义,
「写进 status」会污染状态机,均否决。

### 3.2 唤醒:NOTIFY 加速 + 轮询兜底

`internal/store/research_queue.go`:

- `NotifyResearchPending`:`pg_notify('piks_research_pending', '')`(**仅加速**,失败不致命)。
- `ListenResearchNotify`:**专用连接**(`pool.Acquire` —— pgxpool 复用会清掉 LISTEN 状态);
  断线退避重连,且每次成功 LISTEN 后**回调一次**(补上断线期间错过的通知与存量 pending)。

worker 的正确性**不依赖** NOTIFY:轮询(`-poll 5s`)是兜底。实测 `-poll 60s` 下通知同秒唤醒。

### 3.3 孤儿回收(租约)

`ReapStuckResearchRuns(grace)`:把 `updated_at` 超期的进行中 run 置 `failed`。`grace =
TimeoutTotal + 2min = 8min`(**必须 > `TimeoutGather`**,否则误杀正在跑的 run)。诚实登记
中断而非让它永久卡在 `gathering`。

> 旧 `researchRunStale` 启发式已删:web 不再判断谁在跑(它无从知道),收口归 worker 的 reaper。

### 3.4 web 侧改动

`researchRunTrigger` 改为:校验主体 → `FindActiveResearchRun` 防重 → `CreateResearchRun`
(pending)→ `NotifyResearchPending` → 返回 `{run_id,status}`。**响应形状逐字不变**(202/200
分支保留),前端轮询契约不受影响。**删除**:`go func(){o.Run}`、`researchProvider()`、
`context.WithTimeout(TimeoutTotal)`、`researchRunStale`。

### 3.5 cmd/research-worker

`SIGTERM` 只停**认领**(`stopCtx`);在跑的 run 用 `context.Background() + TimeoutTotal`
跑完,不被信号打断。compose 设 `stop_grace_period: 7m`(> `TimeoutTotal` 6min)—— docker
默认 10s 会 SIGKILL 在跑的 run。`concurrency 2` + `poll 5s` + `reap-interval 1m`(均可调)。

## 4. 编排与网关

### 4.1 compose(`configs/docker-compose.prod.yml`,五服务)

| 服务 | 说明 |
|---|---|
| `postgres` | :5433 **loopback** |
| `gateway` | **唯一对外** `0.0.0.0:8090:80`;`depends_on web(service_healthy)` |
| `web` | **不发布宿主端口**;healthcheck `wget /api/v1/dashboard`;`piks_data:/data` |
| `research` | 常驻 worker;`stop_grace_period 7m`;与 web 共享 `piks_data`(子目录不重叠) |
| `tools` | `profile=run`,`user: 1000:1000` |

### 4.2 nginx:resolver + 变量式 proxy_pass(必须,非优化)

```nginx
location /api/ {
    resolver 127.0.0.11 valid=10s ipv6=off;
    set $piks_web "http://web:8090";
    proxy_pass $piks_web$request_uri;   # 变量式必须拼 $request_uri
}
```

nginx 对 `proxy_pass` 里的**字面主机名**只在 worker 启动时解析一次并缓存到进程结束。web 每次
重建都换 IP → 不这样改,`deploy.sh` 每跑一次 `up -d web`,gateway 就会 502 到自己被重启为止。

> 变量式 `proxy_pass` **不会自动带原 URI**,必须显式拼 `$request_uri`,否则 `/api/v1/x` 会请求成 `/`。

**安全性不退化**:web 只是 Docker 私网 `piks` 内可路由(gateway 经 `web:8090` 反代),
**未发布宿主端口**,对外仍只有 gateway 一个入口。

## 5. deploy.sh 重写

1. **每镜像版本 = 自己输入的 hash**,与 HEAD hash 解耦。输入集**必须覆盖该镜像构建真正读的
   每个文件** —— 少算一个就是「改了文件、tag 不动、不重建不传输,生产跑旧像」的**静默错误**,
   比多算严重得多。故:
   - **前端** → 整个 `frontend/` 的 git tree(递归覆盖 `index.html`/`vite.config.ts`/
     `tailwind.config.ts`/`package-lock.json` 等全部构建输入;`node_modules`/`dist` 已 gitignore,
     不在 tree 里)+ `configs/nginx.conf`。
   - **Go** → **`go list -deps` 取该命令的真实依赖闭包**,而非手列目录。手列在「新增一个 import」
     时静默漏掉(实施中实测:`research-worker` 依赖 `internal/model`,手列清单曾漏它;工具链
     不可用时回退整树 hash = 安全方向)。
   - ⚠️ **四处输入集都含 `Dockerfile` 整文件**(issue #55 复盘补上):本节第 1 条(「输入集必须
     覆盖该镜像构建真正读的每个文件」)当初**没被贯彻到 Dockerfile 自己身上** —— Dockerfile 不在
     任何输入集里,于是改了 Dockerfile 也不改 tag → `build_image` 判「已存在、跳过构建」→
     修好的镜像既不重建也不传输,**修复静默失效**。issue #55 修 gateway 冷启动挂死时正是撞上
     这一点:只改 Dockerfile 一行,若不先补输入集,生产仍跑带 bug 的旧像。取整文件 hash
     (改注释也触发重建)是**故意的**:方向安全(宁可多重建,不可漏),Dockerfile 极少改动。
2. 改前端不动 web/tools/research 的 tag → 它们**既不重建也不传输**。
3. `docker build --target <role>`(tag 已存在则跳过)→ `save|load`(仅传 lab 上缺的 tag)。
4. **干净树门控**:镜像从**工作树**构建却以**版本 tag** 命名,工作树脏时 tag 与内容不符。
   默认拒绝;调试用 `PIKS_ALLOW_DIRTY=1`(tag 附 `-dirty`)。
5. **顺序硬约束**:`up postgres` → `run --rm tools ./bin/migrate` → `up web research` →
   `up gateway`。migrate 必须先于 web(`cmd/web/main.go` 启动即读 `app_config`,缺表 fatal
   崩溃循环);gateway 必须最后(否则 nginx 短暂反代半启动的 web)。
6. **stack 清单**(`stack-manifest.json`)落 lab:本次上线的四镜像 tag + commit,是
   「生产在跑什么」的单一答案。

**回滚**:`rollback-pre-split`(四镜像各一份)+ `rollback-pre-split-single`(拆分前旧单镜像,
含 nginx+Go+Python 三件套,已实测可启动并服务 `/api/v1/dashboard`)。

> ⚠️ **回滚快照只打一次、绝不覆盖**:回滚点必须指向「升级之前」的形态,它是一次性的。
> 若无脑 `docker tag latest rollback-pre-X`,第二次部署时 `latest` 已是升级后的新镜像 →
> 回滚点被**静默改写**成新镜像,名字还叫 `rollback-pre-X`(标签与内容不符,真回滚时才发现
> 回滚不了)。实施中确实踩到:`-single` 被第二次部署覆盖成了新 tools 像(不含 nginx)。
> 故脚本改为**存在即跳过**;`-single` 从已知的拆分前 tag(`PIKS_PRE_SPLIT_REF`,默认
> `piks-tools:v0.0.0-b996864`)取,而非 `latest`。

## 6. CI 守卫

- `scripts/check-research-isolation.sh` 新增规则 4:`exec.Command` 只允许出现在白名单
  (`internal/research/runner.go`、`cmd/daily-review/main.go`、`internal/publish/publish.go`)。
  拆分后 web 容器无 python3,**任何新增进程执行点都是回退信号**。
- `scripts/check-image-topology.sh`(新增):四 target 存在、底座正确、只有 research 装 Python、
  tools 逐条 9 命令(拒绝 bulk COPY)、dist 只进 gateway、web 不含 nginx/dist。

## 7. 验收(2026-09-20,lab 生产)

| 项 | 结果 |
|---|---|
| 13 页经 :8090 | 全 **200** |
| `/api/v1/{dashboard,research-runs,watchlist}` | 全 **200** |
| UI 触发深研 → done | `pending`→`gathering` **3s 内**(NOTIFY 生效)→ `done` **33s** |
| worker 日志 | `worker#1 认领 … quick=true` / `完成: gate=true lint=true tokens=2453` / `收口: status=done` |
| 容器边界 | web **无** python3、**无** nginx;gateway **无** python3 |
| 上传 10MB 边界 | 2MB → 到 web(400 应用层);11MB → nginx **413** |
| **resolver 实测** | 重建 web(新容器新 IP)后 **gateway 未重启**,API 仍 200 |
| 深研 CLI 旁路 | `exec research ./bin/research-run --help` 正常 |
| 栈清单 | `v0.0.0-616f31b`(gateway `c4377ad` / web `bb80388` / tools `602d148` / research `2426e05`) |
| 回滚镜像 | `rollback-pre-split`(4 个)+ `rollback-pre-split-single`(旧单镜像 934MB) |

## 8. 已知代价(如实登记)

- **拆分不减总量**:四镜像合计 save ~214MB,与旧单镜像 ~216MB 持平。收益是**升级半径**
  (改前端只传 25MB 而非 216MB),不是流量。
- **research target 仍依赖 Go 编译阶段**(worker/research-run 是 Go 二进制):改 `research/`
  会重跑 `go build`,但**不重跑 node**、gateway/web 不动。D-11 隔离的一处已知收窄。
- **共享 `internal/store`**:改它会重建 web + tools + research。`--target` 只共享 build 阶段,
  不复制编译。
- **gateway 的 nginx 启动路径曾含外网依赖**(issue #55):`nginx:alpine` 自带 entrypoint 会跑
  `10-listen-on-ipv6-by-default.sh`,其中 `apk manifest nginx` 在冷容器里访问 apk 仓库
  (`dl-cdn.alpinelinux.org`) —— 该域名从 lab 解析不稳时脚本卡死,**nginx 永不启动而容器显示
  `Started`**。该脚本对本项目无意义(它只校验**打包自带** `default.conf` 未被改动),故
  Dockerfile 已 `rm -f` 删除。**教训**:基镜像的 entrypoint 钩子在冷启动时**可能走网络**,
  凡「卡住但不报错」的症状都要查 entrypoint。
- **`Dockerfile` 从不在 hash 输入集里**(issue #55):见 §5 第 1 条。任何「只改 Dockerfile」的
  修复,若不先补输入集,都会静默不生效。

- **新增常驻进程**(仓库首个 daemon):`restart: unless-stopped` + 租约回收兜底崩溃。
- **未引入 buildx**:Go 阶段 builder cache 留待后续。

## 9. 明确不做

- ❌ 统一 Postgres 实例 / 共享表 / 跨系统 JOIN(D-7 铁律)
- ❌ K8s / 服务网格 / 逐 cmd 拆镜像
- ❌ web → research 的 HTTP 委托桥(2026-09-12 已证该拓扑在生产必挂)
- ❌ 本期引入 buildx
