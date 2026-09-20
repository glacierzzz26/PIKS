# PIKS 生产镜像:一次构建全部命令 + 前端静态产物 + Python 深研运行时,单镜像交付(dev 编译 → docker save|load lab)。
# 构建(生产 lab):docker build --build-arg GIT_SHORT=$(git rev-parse --short HEAD) -t piks-tools:latest .
# 使用:
#   web:   nginx 网关(:80,发布 :8090)服务 React SPA + 反代 Go(127.0.0.1:8090)与交互页
#   tools: docker compose run --rm tools ./bin/<cmd>
# 相对路径依赖(migrate→migrations/、worker→prompts/extract.md)在 /app 下。
# 依赖不入库:go.sum 校验完整性 + GOPROXY 模块代理下载(首次构建需网络;GOPROXY 可用 --build-arg 覆盖)。
#
# ── 单镜像(2026-09-12 定,替代原 D-2 双镜像)─────────────────────────────────
# 底座 = python:3.12-slim(非 nginx:alpine):web 容器内同时跑 nginx + Go deamon
# + Python 深研运行时。这样 Go 编排(internal/research/runner.go 的 os/exec python3)
# 能在 web 进程内直接触发深研 —— UI「深研」按钮不再需要第二镜像/委托桥。
# 代价:web 容器多背 research 依赖(~300MB)与一次「改 Python 需重启 web」的部署粒度;
# 换来:UI 触发零 Go 改动、无 docker.sock 安全面、加第 N 个分析师不必再建桥。
# 注意:必须用 debian 底座装 nginx;反向(nginx:alpine 上 apk add python3)akshare/pandas
# 要 musl 源码编译,不可行。
# ─────────────────────────────────────────────────────────────────────────

# ---- build:Go 编译(去 vendor)----
FROM golang:1.26-alpine AS build
WORKDIR /src
# 依赖走模块代理(不再 vendor 入库);依赖层单独 COPY + download,业务代码改动不触发重新下载。
# GOPROXY 可用 --build-arg 覆盖(离线构建可传 off 并预热 module cache)。
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download && go mod verify
# ⚠️ 只 COPY Go 编译真正需要的目录(cmd/ internal/),不含 frontend/ 与 docs/。
# 原先 `COPY . .` 会把整个仓库灌进本阶段 —— 改一行前端就让本层失效、
# 13 个二进制全量重编(实测 27s)。收窄后:改前端不再击穿 Go 层。
# 新增含 .go 文件的顶层目录时须同步此处(否则构建时模块解析不到)。
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# -s -w 去符号表/DWARF:单命令 ~10MB → 显著下降(panic 栈仍带函数名)。
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/bin/ ./cmd/...

# 前端静态构建(Vite SPA;dist 是唯一产物)
FROM node:20-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- 最终阶段:nginx 网关 + Go bins + React dist + research(Python 深研运行时)----
FROM python:3.12-slim AS tools
# nginx:网关;git: daily-review/reconcile 在 vault 启用时提交(git 命令);tzdata: TZ 生效。
# 默认 apt 源 deb.debian.org 在国内时常卡死(实测 apt-get install 挂 20+ 分钟无进度),
# 故默认切 aliyun 镜像;APT_MIRROR 可 --build-arg 覆盖(出国/离线环境可传 deb.debian.org)。
ARG APT_MIRROR=mirrors.aliyun.com
RUN sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" \
      /etc/apt/sources.list.d/debian.sources \
    && apt-get update && apt-get install -y --no-install-recommends \
      nginx git ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    # debian nginx 自带 sites-enabled/default(监听 80 default_server),与 conf.d 冲突 → 移除
    && rm -f /etc/nginx/sites-enabled/default
# 容器内无 .git,血缘字段取此烘焙值
ARG GIT_SHORT=unknown
# 版本号:未发版恒为 v0.0.0(见 CLAUDE.md 发布纪律);发版时由 deploy.sh 取 git tag 传入。
ARG PIKS_VERSION=v0.0.0
ENV PIKS_GIT_SHORT=${GIT_SHORT} \
    PIKS_VERSION=${PIKS_VERSION}
WORKDIR /app
# research 依赖层独立缓存:requirements.txt 不变则不重装(akshare 装一次较慢)。
# 镜像源不稳定(files.pythonhosted.org 时长读超时),加重试与超时兜底;
# PYPI_INDEX 可用 --build-arg 覆盖为国内镜像加速(aliyun 实测约 3.5×)。
COPY research/requirements.txt ./research/requirements.txt
ARG PYPI_INDEX=https://pypi.org/simple
RUN pip install --no-cache-dir --retries 10 --timeout 120 \
      -i "${PYPI_INDEX}" -r research/requirements.txt
COPY research/ ./research/
# 编排器定位 Python 源码/解释器(见 internal/research/runner.go);
# 容器内无 .venv,依赖装在系统 site-packages,故解释器即 python3。
ENV PIKS_RESEARCH_DIR=/app/research \
    PIKS_PYTHON_BIN=python3 \
    PYTHONIOENCODING=utf-8
# 运行时文件资源,直接从构建上下文取(Go 编译不需要它们,故不再经 build 阶段中转)
COPY migrations/ /app/migrations
COPY prompts/ /app/prompts
# 与 dev bin/ 布局一致,脚本统一 ./bin/<cmd>
COPY --from=build /out/bin/ /app/bin/
# nginx 网关配置 + 前端静态文件
COPY configs/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=frontend /src/frontend/dist /usr/share/nginx/html
ENTRYPOINT []
