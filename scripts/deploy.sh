#!/usr/bin/env bash
# PIKS 生产更新/部署(在 dev 侧执行):本地编译镜像 → 传输加载到 lab → 起 postgres/web → migrate。
# 生产代码变更经此命令生效;lab 不保留代码仓库,镜像为唯一交付物。
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
LAB="${PIKS_LAB:-rguo@192.168.0.202}"
GS="$(git -C "$REPO" rev-parse --short HEAD)"

echo "== build image ($GS)"
docker build --build-arg GIT_SHORT="$GS" -t piks-tools:latest "$REPO"

echo "== transfer to lab (docker save | ssh docker load)"
docker save piks-tools:latest | ssh "$LAB" docker load

# 顺序:postgres → migrate → web。web 启动即读 app_config(ApplyAppConfig),
# 必须先迁移建表再起 web,否则首启会因表缺失 fatal 崩溃循环。
echo "== postgres up → migrate → web up"
ssh "$LAB" 'docker compose -f /home/rguo/piks/docker-compose.yml up -d postgres && docker compose -f /home/rguo/piks/docker-compose.yml run --rm tools ./bin/migrate && docker compose -f /home/rguo/piks/docker-compose.yml up -d web'

echo "deploy done: image $GS loaded on lab, migrate ok"
