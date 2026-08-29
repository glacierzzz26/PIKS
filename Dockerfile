# PIKS 生产镜像:一次构建全部命令 + 前端静态产物,单镜像交付(dev 编译 → docker save|load lab)。
# 构建(生产 lab):docker build --build-arg GIT_SHORT=$(git rev-parse --short HEAD) -t piks-tools:latest .
# 使用:
#   web:   nginx 网关(:80,发布 :8090)服务 React SPA + 反代 Go(127.0.0.1:8090)与交互页
#   tools: docker compose run --rm tools ./bin/<cmd>
# 相对路径依赖(migrate→migrations/、worker→prompts/extract.md)在 /app 下。
FROM golang:1.26-alpine AS build
WORKDIR /src
# 自包含构建(go mod vendor,免模块下载,不受 proxy 可达性影响)
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -o /out/bin/ ./cmd/...

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
