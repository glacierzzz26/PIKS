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

# 全链:新闻→抽取→聚类→行情→实体→快照→复盘→对账。失败步骤记录不阻断(幂等,可重试)。
# 迭代 5-2:vault/GitHub 下线(Web 直读 PG),daily-review/reconcile 在 vault 禁用时跳过写盘+git。
# (原 publisher 命令已删除;渲染逻辑 trace 在 internal/publish,由 daily-review/reconcile/web 复用。)
# 事件类多源(issue #43 T1):`collector -driver all` 依次跑 6 个独立机构源
# (东财/金十/财联社/新浪/同花顺/富途),每源落各自机构名 sources 行;单源失败不阻断其余源。
# 公告(issue #50 T4):`collector -driver cninfo-announce` 单独跑巨潮全市场个股公告
# (~1200 条/日),落 source_type='announcement' / status='collected' ——
# **不进 LLM 抽取**(worker 只取 status='raw'),也不报对账异常。放在 worker 之前无妨:
# 公告永不入 raw 队列,顺序不敏感;显式列在与快讯相邻处便于阅读。
# file 驱动仅迭代0 保底,生产不用。
# ⚠️ 多源后单日入库量升至 ~180 条(原东财单源 ~50),worker 默认 -limit 50 会恒追不上、
# 积压 raw 永不清零 → 显式抬高到 -limit 300(覆盖一日量;token 护栏仍是 ai_daily_token_budget)。
# 日期敏感命令显式 -date $TODAY:quote-collector / market-state / daily-review / reconcile。
ok=1
for c in migrate "collector -driver all" "collector -driver cninfo-announce" "worker -limit 300" cluster "quote-collector -date $TODAY" entity-build "market-state -date $TODAY" "daily-review -date $TODAY" "reconcile -date $TODAY"; do
  # shellcheck disable=SC2086   # $c 含参数时按空格拆分为独立参数
  if run $c; then echo "== ok $c" >> "$L"; else echo "== FAIL $c" >> "$L"; ok=0; fi
done

if [ "$ok" -eq 1 ]; then
  touch "$LOG/pipeline-$TODAY.done"
  echo "pipeline done $(date '+%F %T %Z')" >> "$L"
else
  echo "pipeline FAILED steps above; will retry next tick" >> "$L"
fi
