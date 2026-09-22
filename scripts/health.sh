#!/usr/bin/env bash
# PIKS 管线完整性自检(可选,D-P11;issue #76 起按步骤台账判定)。
# 交易日 16:10 后:全部步骤完成 → ok;有步骤未完成/已放弃 → WARN。
# 输出一行到 logs/health.log;可挂 cron 或手动查。
#
# ⚠️ stamp 语义已变(issue #76):以前「任一失败即不生成 `.done`」⇒ 单个确定性失败会
# 触发全链 15 分钟重跑。现在 `.done` 仍是「全链跑完」凭证,但另有**步骤台账目录**
# `pipeline-$TODAY.done.d/<step>.done` + `pipeline-$TODAY.fail.<step>`(失败计数)。
# 故「未完成」的判定必须看台账,不能只看 `.done`,否则失败步骤会被静默略过。
set -uo pipefail
export TZ=Asia/Shanghai
C=/home/rguo/piks; LOG=$C/logs
TODAY=$(date +%F); DOW=$(date +%u); HMS=$(date +%H%M)
mkdir -p "$LOG"

# 与 pipeline.sh 的 STEPS 台账名保持同步(顺序无关,只用于「哪些步骤应当完成」)。
STEPS=(migrate collector_all collector_announce hot_topic worker cluster \
       quote_collector entity_build watch_sync market_state daily_review reconcile)

line="ok"
if [ "$DOW" -lt 6 ] && [ "$HMS" -ge 1610 ]; then
  if [ -f "$LOG/pipeline-$TODAY.done" ] && [ ! -d "$LOG/pipeline-$TODAY.done.d" ]; then
    : # 升级过渡日:旧格式 stamp(无台账目录)⇒ 视为历史已跑,不误报
  elif [ -f "$LOG/pipeline-$TODAY.done" ]; then
    # 全链凭证在 → 但台账仍可能有缺失(理论上不该发生),如实核对。
    missing=""
    for s in "${STEPS[@]}"; do
      [ -f "$LOG/pipeline-$TODAY.done.d/$s.done" ] || missing="$missing $s"
    done
    if [ -n "$missing" ]; then
      line="WARN 管线已标记完成但台账缺步骤:$missing"
    elif grep -q 'FAIL\|ABANDON' "$LOG/pipeline-$TODAY.log" 2>/dev/null; then
      line="WARN 管线已完成但日志含失败/放弃记录(见 $LOG/pipeline-$TODAY.log)"
    fi
  else
    # 无全链凭证:区分「未完成」与「已放弃」——放弃是**需要人工介入**的终态。
    missing=""; abandoned=""
    for s in "${STEPS[@]}"; do
      [ -f "$LOG/pipeline-$TODAY.done.d/$s.done" ] && continue
      if [ -f "$LOG/pipeline-$TODAY.fail.$s" ] && \
         [ "$(cat "$LOG/pipeline-$TODAY.fail.$s" 2>/dev/null || echo 0)" -ge "${PIKS_MAX_RETRY:-5}" ]; then
        abandoned="$abandoned $s"
      else
        missing="$missing $s"
      fi
    done
    if [ -n "$abandoned" ]; then
      line="WARN 交易日 $TODAY 有步骤已放弃,需人工介入:$abandoned"
    elif [ -n "$missing" ]; then
      line="WARN 交易日 $TODAY 16:10 后管线未完成,待跑步骤:$missing"
    fi
  fi
fi
echo "$(date '+%F %T %Z') $line" >> "$LOG/health.log"
echo "$line"
