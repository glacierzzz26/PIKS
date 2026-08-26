#!/usr/bin/env bash
# PIKS 更新部署(D-P12):代码(dev 线)→ 重建镜像(GIT_SHORT 烘焙血缘)→ 确保 postgres → migrate。
# 之后 pipeline.sh 由 cron 自然触发,或手动跑一次立即生效。
# 回滚:数据=备份 pg_restore --clean;代码=git checkout <旧commit> + 重建镜像 + migrate(迁移只前向)。
set -euo pipefail
C=/srv/piks
cd "$C/piks" && git fetch origin && git pull --ff-only
docker build --build-arg GIT_SHORT="$(git -C "$C/piks" rev-parse --short HEAD)" -t piks-tools:latest "$C/piks"
docker compose -f "$C/docker-compose.yml" up -d postgres
docker compose -f "$C/docker-compose.yml" run --rm tools ./bin/migrate
echo "deploy done $(date '+%F %T %Z')"
