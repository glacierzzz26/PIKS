#!/usr/bin/env bash
# PIKS raw_documents 保留期清理(issue #83 分期 P-5)。
#
# 触发判据(照 issue P3「是周日 且 距上次清理 ≥28 天」,幂等、无需锚点):
#   - 仅周日(本脚本由 crontab `0 2 * * 7` 拉起,02:00 避开 23:59 的 backup.sh);
#   - 距上次**成功**清理 ≥28 天 —— 时间戳落 $LOG/cleanup.last,**只在成功后更新**
#     (失败/中断则不动,下个周日自然重试)。
#
# 做什么:`docker compose run --rm -T tools ./bin/cleanup`(默认 -days 28)。
# 🔴 只清「到期 + 无事件引用 + 非转载组代表」的 raw 行(判据见 internal/store 的
#   cleanupWhere)。**已抽取行永久保留**(事件溯源),故「28 天」不是「全体 raw 的 28 天」。
set -uo pipefail
C=/home/rguo/piks; LOG=$C/logs
mkdir -p "$LOG"
export TZ=Asia/Shanghai

# ① 单实例锁:上一轮未结束则本 tick 退出(与 pipeline.sh 同法,锁放 $LOG 免 sudo)。
exec 9>"$LOG/cleanup.lock"
if ! flock -n 9; then
  echo "$(date '+%F %T %Z') 另一 cleanup 实例仍在运行,本 tick 跳过" >> "$LOG/cron.log"
  exit 0
fi

# ② 周日闸:非周日直接退出(crontab 已限周日,此处双保险,防手动/误配触发)。
DOW=$(date +%u)
[ "$DOW" -ne 7 ] && { echo "$(date '+%F %T %Z') 非周日(weekday=$DOW),跳过" >> "$LOG/cron.log"; exit 0; }

# ③ 距上次成功 ≥28 天:cleanup.last 是**上次成功**的 epoch 秒;缺失视为「首次」(立即清)。
INTERVAL_DAYS="${PIKS_CLEANUP_INTERVAL_DAYS:-28}"
NOW=$(date +%s)
if [ -f "$LOG/cleanup.last" ]; then
  LAST=$(cat "$LOG/cleanup.last" 2>/dev/null || echo 0)
  case "$LAST" in ''|*[!0-9]*) LAST=0 ;; esac
  AGE_DAYS=$(( (NOW - LAST) / 86400 ))
  if [ "$AGE_DAYS" -lt "$INTERVAL_DAYS" ]; then
    echo "$(date '+%F %T %Z') 距上次清理仅 $AGE_DAYS 天(<$INTERVAL_DAYS),跳过" >> "$LOG/cron.log"
    exit 0
  fi
fi

# ④ 跑清理。失败**不**更新时间戳(下次再试),如实记日志(#64 教训:失败不静默)。
OUT=$(docker compose -f "$C/docker-compose.yml" run --rm -T tools ./bin/cleanup 2>&1); RC=$?
echo "== $(date '+%F %T %Z') cleanup rc=$RC" >> "$LOG/cleanup.log"
echo "$OUT" >> "$LOG/cleanup.log"
if [ "$RC" -eq 0 ]; then
  echo "$NOW" > "$LOG/cleanup.last"
  echo "cleanup ok $(date '+%F %T %Z')" >> "$LOG/cron.log"
else
  echo "cleanup FAIL rc=$RC $(date '+%F %T %Z')" >> "$LOG/cron.log"
fi
exit "$RC"
