# PIKS 生产镜像(2026-09-20 容器拆分,issue #47):单 Dockerfile、多 target、四镜像。
#
#   docker build --target gateway  -t piks-gateway:latest  .
#   docker build --target web      -t piks-web:latest      .
#   docker build --target tools    -t piks-tools:latest    .
#   docker build --target research -t piks-research:latest .
#   (不传 --target → 默认末阶段 research,见文末注)
#
# 为什么一个文件而非四个:Go 编译阶段与前端构建阶段在四镜像间完全共享。拆成四个
# Dockerfile 会把 go mod download + go build 复制 4 份,且 GO 版本/ldflags/GOPROXY
# 出现漂移时无单一真源收敛。legacy builder(本机 Docker 29,无 buildx/无 --mount)
# 支持 --target,**按 target 只跑到所需阶段** —— 故 --target gateway 不会碰 golang/node 阶段。
# ⚠️ 不要引入 `# syntax=` 指令(需 BuildKit,本机 legacy builder 会硬失败)。
#
# ── 拆分动机(取代 2026-09-12 单镜像 D-2)──────────────────────────────────
# 单镜像 piks-tools(934MB)同时背四职责:nginx 网关 + Go web + 12 个批处理 CLI +
# Python 深研运行时。后果是升级半径不可分:改前端 → 整镜像重建;改 Python → 整镜像
# 重建 + 重启 web。且 Dockerfile 曾用 `COPY . .` 把整个仓库灌进 Go 阶段,改一行前端就
# 击穿 Go 层、13 个二进制全量重编(实测 27s,P1 已修)。
#
# 拆后:改前端只重建 gateway;改 Go 只重建 web/tools/research;改 Python 只重建 research。
# 深研触发随之从「web 进程内 os/exec python3」改为「web 建 pending 行 + research 容器
# 内常驻 worker 认领」(见 migrations/0015、cmd/research-worker)。
# ─────────────────────────────────────────────────────────────────────────

# 版本标识(四镜像共烘焙):容器内无 .git,血缘字段取构建时传入的烘焙值。
# ⚠️ 必须声明在**首个 FROM 之前**(全局作用域)—— 在内层阶段声明的 ARG 只对该阶段可见,
# 后续阶段 `ARG GIT_SHORT` 拿不到它的默认值(不传 --build-arg 时会是空串而非 unknown)。
# 各目标阶段仍须各自 `ARG` 重申一次才能取用(见下)。
ARG GIT_SHORT=unknown
ARG PIKS_VERSION=v0.0.0

# ---- build:Go 编译(四镜像共享)----
FROM golang:1.26-alpine AS build
WORKDIR /src
# 依赖走模块代理(不再 vendor 入库);依赖层单独 COPY + download,业务代码改动不触发重新下载。
# GOPROXY 可用 --build-arg 覆盖(离线构建可传 off 并预热 module cache)。
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download && go mod verify
# ⚠️ 只 COPY Go 编译真正需要的目录(cmd/ internal/),不含 frontend/ 与 docs/。
# 原先 `COPY . .` 会让前端改动击穿本层、全部二进制重编(实测 27s)。收窄后不成立。
# 新增含 .go 文件的顶层目录时须同步此处(否则构建时模块解析不到)。
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# -s -w 去符号表/DWARF:单命令 ~10MB → 显著下降(panic 栈仍带函数名)。
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/bin/ ./cmd/...

# ---- frontend:React SPA 静态构建(仅 gateway 用)----
FROM node:20-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ============================================================================
# gateway:纯 nginx 网关(唯一对外入口)。服务 React 静态文件 + 反代 /api/* → web。
# 不含 Go、不含 Python —— 前端改动静止时本镜像零变动。
# ============================================================================
FROM nginx:alpine AS gateway
ARG GIT_SHORT
ARG PIKS_VERSION
ENV PIKS_GIT_SHORT=${GIT_SHORT} \
    PIKS_VERSION=${PIKS_VERSION} \
    PIKS_IMAGE_ROLE=gateway
# nginx:alpine 自带 entrypoint + `CMD nginx -g daemon off;`,compose 无需覆盖 command。
#
# ⚠️ 删掉 10-listen-on-ipv6-by-default.sh(issue #55,2026-09-20 生产 P0):
# 该脚本对 Alpine 执行 `apk manifest nginx`,而 apk 在**冷容器**里会去连 apk 仓库
# (dl-cdn.alpinelinux.org,Fastly)。该域名从 lab 解析不稳定 → 脚本卡死 → entrypoint
# 阻塞 → **nginx 永不启动**,而容器状态仍显示 Started(静默挂死)。
# 它的唯一作用是:校验**打包自带**的 default.conf 未被改动后自动补 `listen [::]:80;`。
# 我们的 default.conf 是下面这行自己 COPY 进去的(configs/nginx.conf,已显式 listen 80;),
# 永远不等于打包版 → 该校验必然走「differs from packaged」分支,纯属白跑。
# 删掉即移除启动路径上的外网依赖,零功能损失。
RUN rm -f /docker-entrypoint.d/10-listen-on-ipv6-by-default.sh
COPY configs/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=frontend /src/frontend/dist /usr/share/nginx/html

# ============================================================================
# web:纯 Go JSON API。不再含 nginx、不含 Python —— 不再有「改 Python 需重启 web」。
# ⚠️ 深研不再在 web 进程内跑(拆分后此处无 python3),编排由 research 容器的 worker 认领。
# ============================================================================
FROM alpine:3.20 AS web
# ca-certificates:LLM 调用走 HTTPS(自签网关 CA 经 SSL_CERT_DIR 注入);
# tzdata:TZ=Asia/Shanghai 生效(time.Now().Truncate(24h) 的日界按北京时间 —— 缺了会漂到 UTC)。
# ⚠️ apk 源换国内镜像:默认 dl-cdn.alpinelinux.org 从构建机实测**极不稳**
#   (463KB 索引 ~11KB/s,且会中途挂死),apk 层可耗时数分钟乃至被 SIGKILL
#   (2026-09-21 实测 exit 137 即此)。经实测 aliyun 在容器内可达且稳定
#   (dl-cdn / 清华 在容器内均不可达)。可用 --build-arg APK_MIRROR=… 覆盖。
ARG APK_MIRROR=mirrors.aliyun.com
RUN sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata
# ⚠️ 版本 ARG/ENV 必须排在 **apk add 之后**(2026-09-21 修,与 research 阶段 #57 同因):
#   GIT_SHORT 每次提交都变,若排在 apk 之前,该层缓存**每次升级都被击穿** →
#   每次都要重装 apk(恰好撞上上面那个不稳的 CDN)。挪到 apk 之后即恒定命中缓存。
ARG GIT_SHORT
ARG PIKS_VERSION
ENV PIKS_GIT_SHORT=${GIT_SHORT} \
    PIKS_VERSION=${PIKS_VERSION} \
    PIKS_IMAGE_ROLE=web
WORKDIR /app
COPY --from=build /out/bin/web /app/bin/web
EXPOSE 8090
# 容器内监听 0.0.0.0:8090(私网内由 gateway 反代);不发布宿主端口,故不对局域网暴露。
CMD ["/app/bin/web", "-listen", "0.0.0.0:8090"]

# ============================================================================
# tools:12 个批处理管线命令 + 运行时文件资源。compose run --rm 跑一次即退,非常驻。
# (watch-sync 常驻但复用本镜像 —— 常驻只体现在 compose 的 command/restart。)
# ============================================================================
FROM alpine:3.20 AS tools
# git:daily-review/reconcile 在 vault 启用时提交(当前 vault 下线,保留以免突发);
# ca-certificates:管线命令调 LLM 走 HTTPS;tzdata:非交易日判定/复盘日期用北京时间。
# ⚠️ apk 源换国内镜像(同 web 阶段,2026-09-21):默认 CDN 极不稳会挂死/被 SIGKILL。
ARG APK_MIRROR=mirrors.aliyun.com
RUN sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories && \
    apk add --no-cache git ca-certificates tzdata
# ⚠️ 版本 ARG/ENV 排在 apk 之后,避免每次提交击穿 apk 层缓存(同 web 阶段 / #57)。
ARG GIT_SHORT
ARG PIKS_VERSION
ENV PIKS_GIT_SHORT=${GIT_SHORT} \
    PIKS_VERSION=${PIKS_VERSION} \
    PIKS_IMAGE_ROLE=tools
WORKDIR /app
# 运行时文件资源(migrate 读 migrations/、worker 读 prompts/extract.md),均相对 /app。
COPY migrations/ /app/migrations
COPY prompts/ /app/prompts
# ⚠️ 逐条列出 12 个管线命令,**不用 `COPY --from=build /out/bin/ /app/bin/`**:
# /out/bin/ 含全部 15 个二进制(每个 ~10MB,静态链接各自带一份公共库),整目录复制会把
# web / research-run / research-worker / probe 也塞进 tools —— 既白占体积,又让
# `run --rm tools ./bin/web` 这类误用成为可能。逐条列 = 内容显式可审计,拓扑检查可断言。
# 新增管线命令时须同步此处与 scripts/check-image-topology.sh。
COPY --from=build /out/bin/migrate        /app/bin/migrate
COPY --from=build /out/bin/collector      /app/bin/collector
COPY --from=build /out/bin/hot-topic      /app/bin/hot-topic
COPY --from=build /out/bin/watch-sync     /app/bin/watch-sync
COPY --from=build /out/bin/worker         /app/bin/worker
COPY --from=build /out/bin/cluster        /app/bin/cluster
COPY --from=build /out/bin/cluster-raw-link /app/bin/cluster-raw-link
COPY --from=build /out/bin/quote-collector /app/bin/quote-collector
COPY --from=build /out/bin/entity-build   /app/bin/entity-build
COPY --from=build /out/bin/market-state   /app/bin/market-state
COPY --from=build /out/bin/daily-review   /app/bin/daily-review
COPY --from=build /out/bin/reconcile      /app/bin/reconcile

# ============================================================================
# research:Python 深研运行时 + 队列 worker + CLI 编排入口(常驻)。
# 必须留 debian slim —— akshare/pandas 无法在 musl 上构建(见下 apt 注),反向
# 「alpine 上 apk add python3」要 musl 源码编译,不可行。
# ⚠️ 本 target 依赖 golang 阶段(worker/research-run 是 Go 二进制)。这是 D-11「research
# 构建隔离」的一处收窄:research 单独改动不会重跑 node,但会重跑 go build;已记录为接受的代价。
# ============================================================================
FROM python:3.12-slim AS research
# ⚠️ 默认阶段 = 本阶段(最后一个 FROM):不带 --target 时产出 research 镜像。
# 旧 deploy.sh 不带 --target,而它已改为显式传 --target(P4 起),故这里默认谁并不关键;
# 显式传参是部署纪律。
# ⚠️ apt 源:默认 **USTC**(2026-09-22 实测,取代 aliyun)。
#   容器内实测 `apt-get update`(装 ca-certificates + tzdata 那一步,三轮稳定复现):
#     aliyun ~18.6s / 清华 ~6.6s / deb.debian.org 2.2s / **USTC ~1.8s**。
#   aliyun 慢 10 倍 —— 它是本仓历史的默认值(#57 时代沿用至今),但当时只测了 pip 侧。
#   USTC 的 debian-security 路径与 deb.debian.org 一致(`/debian-security`),sed 替换无需额外处理。
#   ⚠️ 属**环境事实非永久结论** —— 换构建机/换线路需重测。可用 --build-arg APT_MIRROR=… 覆盖。
ARG APT_MIRROR=mirrors.ustc.edu.cn
RUN sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" \
      /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
# research 依赖层独立缓存:requirements.txt 不变则不重装(akshare 装一次较慢)。
# 镜像源不稳定(files.pythonhosted.org 时长读超时),加重试与超时兜底;
# PYPI_INDEX 可用 --build-arg 覆盖为国内镜像加速。
COPY research/requirements.txt ./research/requirements.txt
ARG PYPI_INDEX=https://pypi.org/simple
RUN pip install --no-cache-dir --retries 10 --timeout 120 \
      -i "${PYPI_INDEX}" -r research/requirements.txt
# ⚠️ ARG/ENV 元数据必须排在 **pip install 之后**(2026-09-20 修):GIT_SHORT 每次提交
# 都变,若这层在 pip 之前,则每次部署都让 pip 层缓存失效、重下全部依赖(实测把
# research 镜像构建拖到数十分钟,是部署超时的直接原因)。移到之后 → 只要
# requirements.txt 不变,pip 层永久命中缓存。改这里不得再挪回去。
ARG GIT_SHORT
ARG PIKS_VERSION
ENV PIKS_GIT_SHORT=${GIT_SHORT} \
    PIKS_VERSION=${PIKS_VERSION} \
    PIKS_IMAGE_ROLE=research
COPY research/ ./research/
# 编排器定位 Python 源码/解释器(见 internal/research/runner.go);
# 容器内无 .venv,依赖装在系统 site-packages,故解释器即 python3。
ENV PIKS_RESEARCH_DIR=/app/research \
    PIKS_PYTHON_BIN=python3 \
    PYTHONIOENCODING=utf-8
# research-run(CLI 编排入口,与原单镜像同路径)+ research-worker(常驻队列 worker)。
COPY --from=build /out/bin/research-run /app/bin/research-run
COPY --from=build /out/bin/research-worker /app/bin/research-worker
# exec 形式:worker 成为 PID 1 直接收 SIGTERM(优雅关停等 run 收尾;compose 另设
# stop_grace_period > TimeoutTotal,否则 docker 默认 10s 会 SIGKILL 在跑的 run)。
CMD ["/app/bin/research-worker"]
