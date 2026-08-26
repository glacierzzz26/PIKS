#!/usr/bin/env bash
# PIKS 每晚备份(D-P9):pg_dump 自定义格式 → /home/rguo/piks/backups/,保留 14 天。
# vault 在 git(GitHub)已天然备份;DB 是唯一需落盘备份的资产。
set -uo pipefail
export TZ=Asia/Shanghai
C=/home/rguo/piks
mkdir -p "$C/backups"
docker compose -f "$C/docker-compose.yml" exec -T postgres \
  pg_dump -U piks -d piks -Fc > "$C/backups/piks-$(date +%F).dump" \
  && echo "backup ok $(date '+%F %T %Z')" \
  || echo "backup FAIL $(date '+%F %T %Z')"
find "$C/backups" -name 'piks-*.dump' -mtime +14 -delete
