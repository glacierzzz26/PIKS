#!/usr/bin/env bash
# PIKS 日管线(lab,生产化 D-P8):交易日收盘后自动跑完整数据链,一日一次。
# 幂等:所有命令去重/upsert/md5 跳写,重跑零副作用;stamp 文件保证一日一次。
# 免疫宿主时区:全部按北京时间判定(cron 用宿主 UTC,写死时刻会错 8h)。
set -uo pipefail
export TZ=Asia/Shanghai
C=/home/rguo/piks; LOG=$C/logs
TODAY=$(date +%F); DOW=$(date +%u); HMS=$(date +%H%M)

[ -f "$LOG/pipeline-$TODAY.done" ] && exit 0      # 今日已跑
[ "$DOW" -ge 6 ] && exit 0                        # 周末
[ "$HMS" -lt 1610 ] && exit 0                     # 未过收盘后(16:10 放行)
mkdir -p "$LOG"
L="$LOG/pipeline-$TODAY.log"

run() {
  echo "== $(date '+%F %T %Z') $*" >> "$L"
  docker compose -f "$C/docker-compose.yml" run --rm -T tools ./bin/"$@" >> "$L" 2>&1
}

# 全链:新闻→抽取→聚类→行情→实体→快照→复盘→发布→对账。失败步骤记录不阻断(幂等,可重试)。
# 生产新闻源 = 东方财富 7x24 快讯(dongcai 驱动);file 驱动仅迭代0 保底,生产不用。
ok=1
for c in migrate "collector -driver dongcai" worker cluster quote-collector entity-build market-state daily-review publisher reconcile; do
  # shellcheck disable=SC2086   # $c 含参数时按空格拆分为独立参数
  if run $c; then echo "== ok $c" >> "$L"; else echo "== FAIL $c" >> "$L"; ok=0; fi
done

if [ "$ok" -eq 1 ]; then
  touch "$LOG/pipeline-$TODAY.done"
  echo "pipeline done $(date '+%F %T %Z')" >> "$L"
else
  echo "pipeline FAILED steps above; will retry next tick" >> "$L"
fi
