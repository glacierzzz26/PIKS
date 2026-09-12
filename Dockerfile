# PIKS 生产镜像:一次构建全部命令 + 前端静态产物,单镜像交付(dev 编译 → docker save|load lab)。
# 构建(生产 lab):docker build --build-arg GIT_SHORT=$(git rev-parse --short HEAD) -t piks-tools:latest .
# 使用:
#   web:   nginx 网关(:80,发布 :8090)服务 React SPA + 反代 Go(127.0.0.1:8090)与交互页
#   tools: docker compose run --rm tools ./bin/<cmd>
# 相对路径依赖(migrate→migrations/、worker→prompts/extract.md)在 /app 下。
# 依赖不入库:go.sum 校验完整性 + GOPROXY 模块代理下载(首次构建需网络;GOPROXY 可用 --build-arg 覆盖)。
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

# 运行时:nginx(网关)+ Go(127.0.0.1:8090)+ 前端 dist,单镜像交付
FROM nginx:alpine
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
