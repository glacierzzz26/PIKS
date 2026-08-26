# PIKS 生产工具镜像:一次构建全部命令,静态链接,运行时不需 Go。
# 构建(生产 lab):docker build --build-arg GIT_SHORT=$(git rev-parse --short HEAD) -t piks-tools:latest .
# 使用:docker compose run --rm tools ./bin/<cmd>
# 相对路径依赖(migrate→migrations/、worker→prompts/extract.md)在 /app 下。
FROM golang:1.26-alpine AS build
WORKDIR /src
# 自包含构建(go mod vendor,免模块下载,不受 proxy 可达性影响)
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -o /out/bin/ ./cmd/...

FROM alpine:3.20
# git: publisher 提交;tzdata: TZ 生效
RUN apk add --no-cache ca-certificates git tzdata
# 容器内无 .git,血缘字段取此烘焙值
ARG GIT_SHORT=unknown
ENV PIKS_GIT_SHORT=${GIT_SHORT}
WORKDIR /app
COPY --from=build /src/migrations /app/migrations
COPY --from=build /src/prompts /app/prompts
# 与 dev bin/ 布局一致,脚本统一 ./bin/<cmd>
COPY --from=build /out/bin/ /app/bin/
ENTRYPOINT []
