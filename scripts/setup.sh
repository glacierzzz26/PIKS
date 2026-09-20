#!/usr/bin/env bash
# PIKS 生产机 lab 一次性初始化(在 dev 侧执行,幂等可重跑)。
# 前置:rguo@192.168.0.202 免密 ssh;dev 可 docker 构建;/home/rguo/piks/.env 已按 configs/.env.prod.example 填真实值。
# 模型:dev 本地分镜像构建 → 传输 lab(docker save|load);lab 不保留代码仓库。
#
# 2026-09-20(issue #47):镜像由 1 个拆为 4 个(gateway/web/tools/research)。
# 「起 postgres + migrate + 上线四容器」现已内聚在 deploy.sh 的 rollout 段(顺序是硬约束),
# 故原先的 5/9、6/9 两步并入 4/9,不再重复执行。
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
LAB="${PIKS_LAB:-rguo@192.168.0.202}"
C=/home/rguo/piks

echo "== 1/7 目录 + 传输 compose/脚本"
ssh "$LAB" "mkdir -p $C/backups $C/logs $C/scripts"
scp "$REPO/configs/docker-compose.prod.yml" "$LAB:$C/docker-compose.yml"
scp "$REPO/scripts/pipeline.sh" "$REPO/scripts/backup.sh" "$REPO/scripts/health.sh" "$LAB:$C/scripts/"
ssh "$LAB" "chmod +x $C/scripts/*.sh"

echo "== 2/7 .env(必须已填真实值,无 CHANGE_ME 占位)"
ssh "$LAB" "test -f $C/.env && ! grep -q CHANGE_ME $C/.env || { echo '!! 先创建 $C/.env(见 configs/.env.prod.example)再跑'; exit 1; }; chmod 600 $C/.env"

echo "== 3/7 compose 插件(从 dev 拷贝,免网络下载)"
scp /usr/libexec/docker/cli-plugins/docker-compose "$LAB:/tmp/docker-compose"
ssh "$LAB" "mkdir -p ~/.docker/cli-plugins && mv /tmp/docker-compose ~/.docker/cli-plugins/docker-compose && chmod +x ~/.docker/cli-plugins/docker-compose && docker compose version"

echo "== 4/7 四镜像构建 + 按需传输 + 上线(postgres → migrate → web/research → gateway)"
"$REPO/scripts/deploy.sh"

echo "== 5/7 crontab(幂等追加)"
CRON_LINE="*/15 * * * * /home/rguo/piks/scripts/pipeline.sh >> /home/rguo/piks/logs/cron.log 2>&1"
BACKUP_LINE="59 23 * * * /home/rguo/piks/scripts/backup.sh >> /home/rguo/piks/logs/cron.log 2>&1"
ssh "$LAB" "{ crontab -l 2>/dev/null | grep -v -F '$CRON_LINE' | grep -v -F '$BACKUP_LINE'; echo '$CRON_LINE'; echo '$BACKUP_LINE'; } | crontab -"

echo "== 6/7 时间核对(容器内应为北京时间)"
ssh "$LAB" "docker compose -f $C/docker-compose.yml run --rm tools date"

echo "== 7/7 初始数据同步由部署者单独执行(pg_dump dev→lab,设计 §8.3)"
echo
echo "setup done.四容器:gateway(对外 :8090)/ web / research / postgres。"
echo "深研 CLI 旁路(容器内):docker compose -f $C/docker-compose.yml exec research ./bin/research-run <code>"
