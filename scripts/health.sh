#!/usr/bin/env bash
# PIKS 管线完整性自检(可选,D-P11):交易日 16:10 后应有当日 stamp;reconcile 异常进对账报告。
# 输出一行到 logs/health.log;可挂 cron 或手动查。
set -uo pipefail
export TZ=Asia/Shanghai
C=/srv/piks; LOG=$C/logs
TODAY=$(date +%F); DOW=$(date +%u); HMS=$(date +%H%M)
mkdir -p "$LOG"
line="ok"
if [ "$DOW" -lt 6 ] && [ "$HMS" -ge 1610 ] && [ ! -f "$LOG/pipeline-$TODAY.done" ]; then
  line="WARN 交易日 $TODAY 16:10 后管线未完成(stamp 缺失)"
elif [ -f "$LOG/pipeline-$TODAY.done" ] && grep -q 'FAIL' "$LOG/pipeline-$TODAY.log" 2>/dev/null; then
  line="WARN 管线已完成但含失败步骤(见 $LOG/pipeline-$TODAY.log)"
fi
echo "$(date '+%F %T %Z') $line" >> "$LOG/health.log"
echo "$line"
