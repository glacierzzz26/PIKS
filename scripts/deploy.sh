#!/usr/bin/env bash
# PIKS 生产更新/部署(在 dev 侧执行):本地编译镜像 → 传输加载到 lab → 起 postgres/web → migrate。
# 生产代码变更经此命令生效;lab 不保留代码仓库,镜像为唯一交付物。
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
LAB="${PIKS_LAB:-rguo@192.168.0.202}"
GS="$(git -C "$REPO" rev-parse --short HEAD)"

echo "== build images ($GS)"
# piks-research 的 ENTRYPOINT 用 Go 编排二进制,但它属于构建上下文而非 golang 阶段 —— 先编出来,
# 使 `--target research` 完全不触发 golang/node(D-2/§4.10 G3)。改 Python 无需重编此二进制(§4.3.1)。
( cd "$REPO" && go build -o bin/research-run ./cmd/research-run )
docker build --build-arg GIT_SHORT="$GS" -t piks-tools:latest "$REPO"
docker build --target research -t piks-research:latest "$REPO"

echo "== transfer to lab (docker save | ssh docker load)"
docker save piks-tools:latest piks-research:latest | ssh "$LAB" docker load

# web 容器 command(nginx 网关 + Go 127.0.0.1)由 compose 定义,先同步再起服务
echo "== sync prod compose to lab"
scp "$REPO/configs/docker-compose.prod.yml" "$LAB:/home/rguo/piks/docker-compose.yml"

# 顺序:postgres → migrate → web。web 启动即读 app_config(ApplyAppConfig),
# 必须先迁移建表再起 web,否则首启会因表缺失 fatal 崩溃循环。
echo "== postgres up → migrate → web up"
ssh "$LAB" 'docker compose -f /home/rguo/piks/docker-compose.yml up -d postgres && docker compose -f /home/rguo/piks/docker-compose.yml run --rm tools ./bin/migrate && docker compose -f /home/rguo/piks/docker-compose.yml up -d web'

echo "deploy done: image $GS loaded on lab, migrate ok"
