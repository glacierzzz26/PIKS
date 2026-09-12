# PIKS 生产镜像:一次构建全部命令 + 前端静态产物,单镜像交付(dev 编译 → docker save|load lab)。
# 构建(生产 lab):docker build --build-arg GIT_SHORT=$(git rev-parse --short HEAD) -t piks-tools:latest .
# 使用:
#   web:   nginx 网关(:80,发布 :8090)服务 React SPA + 反代 Go(127.0.0.1:8090)与交互页
#   tools: docker compose run --rm tools ./bin/<cmd>
# 相对路径依赖(migrate→migrations/、worker→prompts/extract.md)在 /app 下。
# 依赖不入库:go.sum 校验完整性 + GOPROXY 模块代理下载(首次构建需网络;GOPROXY 可用 --build-arg 覆盖)。
# 独立构建:`docker build --target research -t piks-research:latest .`
#   → 只跑本阶段(9 步,全 Python),golang/node 阶段不触发(§4.10 G3 构建隔离已实测)。
#   ⚠️ 反向(`--target tools` 不跑 Python)需 BuildKit:经典 builder 会构建 target 之前
#      的所有阶段。本机 docker 未装 buildx,DOCKER_BUILDKIT=1 会静默退回经典 builder。
#      构建成本只体现在时间(层缓存命中则几乎免费),产物隔离不受影响 ——
#      两镜像的最终内容已实测互不含对方的运行时(piks-tools 无 python3,piks-research 无 nginx/go)。
# 独立迭代:`research/` 改了只需重建本镜像,主镜像 piks-tools 不动(§4.3.1)。
# 前置:ENTRYPOINT 用的 Go 编排二进制取自构建上下文,须先 `go build -o bin/research-run ./cmd/research-run`。
#   这样 Python 侧迭代不必重编 Go(piks-research 不带 golang 阶段);
#   代价是全新 clone 上必须先编一次 Go 二进制 —— 见 README「深研构建」。
FROM python:3.12-slim AS research
# tzdata: as_of / 交易日判定依赖本地时区
RUN apt-get update && apt-get install -y --no-install-recommends tzdata ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
# 依赖层独立缓存:requirements.txt 不变则不重装(akshare 装一次较慢)
COPY research/requirements.txt ./research/requirements.txt
RUN pip install --no-cache-dir -r research/requirements.txt
COPY research/ ./research/
COPY bin/research-run /app/bin/research-run
# 编排器定位 Python 源码/解释器(见 internal/research/runner.go);
# 容器内无 .venv,依赖装在系统 site-packages,故解释器即 python3。
ENV PIKS_RESEARCH_DIR=/app/research \
    PIKS_PYTHON_BIN=python3 \
    PYTHONIOENCODING=utf-8
# 容器即命令:`docker run --rm piks-research 000560 --profile short-term`
ENTRYPOINT ["/app/bin/research-run"]

# ---- build:Go 编译(去 vendor)----
FROM golang:1.26-alpine AS build
WORKDIR /src
# 依赖走模块代理(不再 vendor 入库);依赖层单独 COPY + download,业务代码改动不触发重新下载。
# GOPROXY 可用 --build-arg 覆盖(离线构建可传 off 并预热 module cache)。
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/bin/ ./cmd/...

# 前端静态构建(Vite SPA;dist 是唯一产物)
FROM node:20-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- tools:默认最终阶段(nginx 网关 + Go + 前端 dist),单镜像交付 ----
# 命名以便 `--target tools` 显式指定(与 research 对偶:该 target 不触发 Python 阶段)。
FROM nginx:alpine AS tools
# git: publisher 提交;tzdata: TZ 生效
RUN apk add --no-cache ca-certificates tzdata git
# 容器内无 .git,血缘字段取此烘焙值
ARG GIT_SHORT=unknown
ENV PIKS_GIT_SHORT=${GIT_SHORT}
WORKDIR /app
COPY --from=build /src/migrations /app/migrations
COPY --from=build /src/prompts /app/prompts
# 与 dev bin/ 布局一致,脚本统一 ./bin/<cmd>
COPY --from=build /out/bin/ /app/bin/
# nginx 网关配置 + 前端静态文件
COPY configs/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=frontend /src/frontend/dist /usr/share/nginx/html
ENTRYPOINT []
