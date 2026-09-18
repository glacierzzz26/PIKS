#!/usr/bin/env bash
# PIKS 生产更新/部署(在 dev 侧执行):本地编译镜像 → 传输加载到 lab → 起 postgres/web → migrate。
# 生产代码变更经此命令生效;lab 不保留代码仓库,镜像为唯一交付物。
#
# 单镜像(2026-09-12):piks-tools 自带 Python 深研运行时(nginx + Go + research),
# 一次 build/save/load 即含全部能力 —— UI「深研」按钮与 CLI 深研共用同一镜像。
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
LAB="${PIKS_LAB:-rguo@192.168.0.202}"
GS="$(git -C "$REPO" rev-parse --short HEAD)"
# 版本号(发布纪律,见 CLAUDE.md):**默认恒为 v0.0.0** —— 只有 HEAD 上打了语义化
# tag(正式发版)才取该 tag。不做「改动挺大就造个号」这类推测。
VER="$(git -C "$REPO" describe --tags --exact-match 2>/dev/null || echo v0.0.0)"
IMAGE_TAG="${VER}-${GS}"
# 国内 pypi.org 时常长读超时(实测本机 15s 无响应),默认走 aliyun 镜像;可 PIKS_PYPI_INDEX 覆盖。
PYPI_INDEX="${PIKS_PYPI_INDEX:-https://mirrors.aliyun.com/pypi/simple}"

echo "== build image (version=$VER hash=$GS -> tag=$IMAGE_TAG, pypi=$PYPI_INDEX)"
docker build \
  --build-arg GIT_SHORT="$GS" \
  --build-arg PIKS_VERSION="$VER" \
  --build-arg PYPI_INDEX="$PYPI_INDEX" \
  -t piks-tools:latest \
  -t "piks-tools:${IMAGE_TAG}" \
  "$REPO"

echo "== transfer to lab (docker save | ssh docker load)"
# 两个 tag 都要传:latest 供 compose 起服务,版本 tag 供事后核对「生产跑的是哪个版本」。
docker save piks-tools:latest "piks-tools:${IMAGE_TAG}" | ssh "$LAB" docker load

# web 容器 command(nginx 网关 + Go 127.0.0.1)由 compose 定义,先同步再起服务
echo "== sync prod compose to lab"
scp "$REPO/configs/docker-compose.prod.yml" "$LAB:/home/rguo/piks/docker-compose.yml"

# 顺序:postgres → migrate → web。web 启动即读 app_config(ApplyAppConfig),
# 必须先迁移建表再起 web,否则首启会因表缺失 fatal 崩溃循环。
# web 重建前一并 up -d web(recreate 拿新镜像)。
echo "== postgres up → migrate → web up"
ssh "$LAB" 'docker compose -f /home/rguo/piks/docker-compose.yml up -d postgres && docker compose -f /home/rguo/piks/docker-compose.yml run --rm tools ./bin/migrate && docker compose -f /home/rguo/piks/docker-compose.yml up -d web'

echo "deploy done: image $IMAGE_TAG loaded on lab, migrate ok"
