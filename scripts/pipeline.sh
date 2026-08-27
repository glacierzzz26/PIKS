#!/usr/bin/env bash
# PIKS 日管线(lab,生产化 D-P8):交易日收盘后自动跑完整数据链,一日一次。
# 幂等:所有命令去重/upsert/md5 跳写,重跑零副作用;stamp 文件保证一日一次。
# 日期锚:优先取 HTTP 服务器时间(GitHub Date 头,免疫宿主时钟漂移),失败回退系统时钟并告警。
# 全链命令显式传 -date $TODAY,把日期钉在门控交易日(运行中途宿主时钟再漂移也不乱)。
set -uo pipefail
C=/home/rguo/piks; LOG=$C/logs

# 权威日期锚:取 GitHub Date 头(epoch 秒);失败回退系统时钟(醒目告警,不静默)。
EPOCH=""; CLOCK_SRC="system(FALLBACK)"
hdr=$(curl -sI --max-time 5 https://api.github.com 2>/dev/null | tr -d '\r' | grep -i '^date:' | sed 's/^[Dd]ate: //')
if [ -n "$hdr" ] && EPOCH=$(date -d "$hdr" +%s 2>/dev/null) && [ -n "$EPOCH" ]; then
  CLOCK_SRC="github"
else
  EPOCH=$(date +%s); CLOCK_SRC="system(FALLBACK)"
  echo "WARN: 日期锚取 GitHub 时间失败,回退系统时钟(epoch=$EPOCH);若系统时钟漂移,复盘日期会错,请尽快修 NTP" >&2
fi

export TZ=Asia/Shanghai
TODAY=$(date -d "@$EPOCH" +%F); DOW=$(date -d "@$EPOCH" +%u); HMS=$(date -d "@$EPOCH" +%H%M)

[ -f "$LOG/pipeline-$TODAY.done" ] && exit 0      # 今日已跑
[ "$DOW" -ge 6 ] && exit 0                        # 周末
[ "$HMS" -lt 1610 ] && exit 0                     # 未过收盘后(16:10 放行)
mkdir -p "$LOG"
L="$LOG/pipeline-$TODAY.log"
echo "clock: $CLOCK_SRC anchored TODAY=$TODAY DOW=$DOW HMS=$HMS (北京时间)" >> "$L"

run() {
  echo "== $(date '+%F %T %Z') $*" >> "$L"
  docker compose -f "$C/docker-compose.yml" run --rm -T tools ./bin/"$@" >> "$L" 2>&1
}

# 全链:新闻→抽取→聚类→行情→实体→快照→复盘→发布→对账。失败步骤记录不阻断(幂等,可重试)。
# 生产新闻源 = 东方财富 7x24 快讯(dongcai 驱动);file 驱动仅迭代0 保底,生产不用。
# 日期敏感命令显式 -date $TODAY:quote-collector / market-state / daily-review / reconcile。
ok=1
for c in migrate "collector -driver dongcai" worker cluster "quote-collector -date $TODAY" entity-build "market-state -date $TODAY" "daily-review -date $TODAY" publisher "reconcile -date $TODAY"; do
  # shellcheck disable=SC2086   # $c 含参数时按空格拆分为独立参数
  if run $c; then echo "== ok $c" >> "$L"; else echo "== FAIL $c" >> "$L"; ok=0; fi
done

if [ "$ok" -eq 1 ]; then
  touch "$LOG/pipeline-$TODAY.done"
  echo "pipeline done $(date '+%F %T %Z')" >> "$L"
else
  echo "pipeline FAILED steps above; will retry next tick" >> "$L"
fi
