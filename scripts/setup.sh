#!/usr/bin/env bash
# PIKS 生产机 lab 一次性初始化(幂等,可重跑)。真实密钥先填 /home/rguo/piks/.env 再跑(见 configs/.env.prod.example)。
# 前置:rguo 免密 SSH、docker 组可用、出站可达 github/opencode/eastmoney。无 sudo 依赖。
set -euo pipefail
export TZ=Asia/Shanghai
C=/home/rguo/piks
ARCH=$(uname -m); [ "$ARCH" = "x86_64" ] && PLAT=linux-x86_64 || PLAT=linux-aarch64

echo "== 1/12 目录"
mkdir -p "$C"/{vault,backups,logs,scripts}

echo "== 2/12 代码(dev 线)"
if [ ! -d "$C/piks/.git" ]; then
  git clone -b dev https://github.com/glacierzzz26/PIKS.git "$C/piks"
else
  git -C "$C/piks" fetch origin && git -C "$C/piks" pull --ff-only
fi

echo "== 3/12 compose 实例化"
[ -f "$C/docker-compose.yml" ] || cp "$C/piks/configs/docker-compose.prod.yml" "$C/docker-compose.yml"

echo "== 4/12 .env(真实值)"
if [ ! -f "$C/.env" ]; then
  cp "$C/piks/configs/.env.prod.example" "$C/.env"; chmod 600 "$C/.env"
  echo "!! 请先填 $C/.env 真实值(PIKS_DB_PASSWORD / PIKS_AI_API_KEY / PIKS_VAULT_GIT_TOKEN),再重跑"
  exit 1
fi
chmod 600 "$C/.env"
set -a; source "$C/.env"; set +a
grep -q 'CHANGE_ME' "$C/.env" && { echo "!! $C/.env 仍有 CHANGE_ME 占位,先填真实值"; exit 1; }

echo "== 5/12 compose 插件(用户级,免 sudo)"
if ! docker compose version >/dev/null 2>&1; then
  mkdir -p "$HOME/.docker/cli-plugins"
  curl -sSL "https://github.com/docker/compose/releases/download/v2.39.3/docker-compose-$PLAT" \
    -o "$HOME/.docker/cli-plugins/docker-compose"
  chmod +x "$HOME/.docker/cli-plugins/docker-compose"
fi

echo "== 6/12 镜像"
docker build --build-arg GIT_SHORT="$(git -C "$C/piks" rev-parse --short HEAD)" -t piks-tools:latest "$C/piks"

echo "== 7/12 postgres"
docker compose -f "$C/docker-compose.yml" up -d postgres
until docker compose -f "$C/docker-compose.yml" exec -T postgres pg_isready -U piks -d piks >/dev/null 2>&1; do sleep 2; done

echo "== 8/12 migrate"
docker compose -f "$C/docker-compose.yml" run --rm tools ./bin/migrate

echo "== 9/12 vault 工作仓(origin=GitHub 私有仓,credential helper 只从 env 读 token)"
if [ ! -d "$C/vault/.git" ]; then
  git clone "$PIKS_VAULT_REMOTE" "$C/vault"
  git -C "$C/vault" config credential.helper '!f(){ echo username=glacierzzz26; echo password=${PIKS_VAULT_GIT_TOKEN}; }; f'
fi

echo "== 10/12 脚本"
cp "$C/piks/scripts/"*.sh "$C/scripts/"
chmod +x "$C/scripts/"*.sh

echo "== 11/12 crontab(幂等追加)"
CRON_LINE="*/15 * * * * /home/rguo/piks/scripts/pipeline.sh >> /home/rguo/piks/logs/cron.log 2>&1"
BACKUP_LINE="59 23 * * * /home/rguo/piks/scripts/backup.sh >> /home/rguo/piks/logs/cron.log 2>&1"
{ crontab -l 2>/dev/null | grep -v -F "$CRON_LINE" | grep -v -F "$BACKUP_LINE"; echo "$CRON_LINE"; echo "$BACKUP_LINE"; } | crontab -

echo "== 12/12 时间核对(容器内应为北京时间)"
docker compose -f "$C/docker-compose.yml" run --rm tools date

echo "setup done。剩余:初始数据同步(dev→lab pg_dump)+ 验收 §15,由部署者执行。"
