#!/usr/bin/env bash
# PIKS 日管线(lab,生产化 D-P8):交易日收盘后自动跑完整数据链,一日一次。
#
# 幂等:所有命令去重/upsert/md5 跳写,重跑零副作用。三层防线(issue #76):
#   ① flock 单实例锁   —— 上一轮未结束时下一 tick 直接退出,消灭并发管线;
#   ② 步骤台账         —— 每完成一步 touch 一个文件,下一 tick 只跑**未完成**的步骤,
#                         不再「任一失败 → 全部重跑」(原先 34 轮/日 的放大器);
#   ③ 失败退避 + 分型  —— 每个 (日期,步骤) 记失败次数,达 PIKS_MAX_RETRY 当日放弃;
#                         确定性错误(SQLSTATE)与额度类(429)立即放弃,不参与高频重试。
# 日期锚:优先取 HTTP 服务器时间(GitHub Date 头,免疫宿主时钟漂移),失败回退系统时钟并告警。
# 全链命令显式传 -date $TODAY,把日期钉在门控交易日(运行中途宿主时钟再漂移也不乱)。
set -uo pipefail
C=/home/rguo/piks; LOG=$C/logs
mkdir -p "$LOG"

# 步骤级覆盖参数(可选环境变量;见 configs/.env.prod.example)
STEP_TIMEOUT="${PIKS_STEP_TIMEOUT:-1800}"   # 单步 wall-clock 上限(秒),默认 30 分钟
MAX_RETRY="${PIKS_MAX_RETRY:-5}"            # 同一步当日连续失败上限,超过则当日放弃
export TZ=Asia/Shanghai

# ── ① 单实例锁:上一轮未结束则本 tick 直接退出,消灭并发管线 ─────────────────────
# 锁放 $LOG(非 /var/lock):rguo 可写、与日志同处、免 sudo。fd 9 随进程退出自动释放。
exec 9>"$LOG/pipeline.lock"
if ! flock -n 9; then
  echo "$(date '+%F %T %Z') 另一管线实例仍在运行(锁被占),本 tick 跳过" >> "$LOG/cron.log"
  exit 0
fi

# 权威日期锚:取 GitHub Date 头(epoch 秒);失败回退系统时钟(醒目告警,不静默)。
EPOCH=""; CLOCK_SRC="system(FALLBACK)"
hdr=$(curl -sI --max-time 5 https://api.github.com 2>/dev/null | tr -d '\r' | grep -i '^date:' | sed 's/^[Dd]ate: //')
if [ -n "$hdr" ] && EPOCH=$(date -d "$hdr" +%s 2>/dev/null) && [ -n "$EPOCH" ]; then
  CLOCK_SRC="github"
else
  EPOCH=$(date +%s); CLOCK_SRC="system(FALLBACK)"
  echo "WARN: 日期锚取 GitHub 时间失败,回退系统时钟(epoch=$EPOCH);若系统时钟漂移,复盘日期会错,请尽快修 NTP" >&2
fi

TODAY=$(date -d "@$EPOCH" +%F); DOW=$(date -d "@$EPOCH" +%u); HMS=$(date -d "@$EPOCH" +%H%M)

[ -f "$LOG/pipeline-$TODAY.done" ] && exit 0      # 今日全链已完成(快速路径)
[ "$DOW" -ge 6 ] && exit 0                        # 周末
[ "$HMS" -lt 1610 ] && exit 0                     # 未过收盘后(16:10 放行)

L="$LOG/pipeline-$TODAY.log"
LEDGER="$LOG/pipeline-$TODAY.done.d"              # 步骤台账目录(每完成一步一个文件)
mkdir -p "$LEDGER"
echo "clock: $CLOCK_SRC anchored TODAY=$TODAY DOW=$DOW HMS=$HMS (北京时间)" >> "$L"

# 步骤清单:name|command。name 只用于台账文件名/日志;顺序即执行顺序 ——
# 台账跳过已完成的步骤,但**保序**遍历(故 migrate 仍先于其余步骤)。
# 新闻→抽取→聚类→行情→实体→快照→复盘→对账。失败步骤记录不阻断(幂等,可重试)。
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
# 积压 raw 永不清零 → 显式抬高;issue #68 C 层盘中每 3 分钟采集后,单日新增进一步放大
# (交易日 ~5.5h / 3min ≈ 110 轮),故再抬到 800 覆盖一日量;token 护栏仍是 ai_daily_token_budget。
# 日期敏感命令显式 -date $TODAY:quote-collector / market-state / daily-review / reconcile。
# 热榜(issue #68 D 层):常驻 `hot-topic`(compose 服务)已覆盖盘中每 30 分钟;此处**再补一
# 发收盘后快照**(16:10 放行时跑,one-shot),用途有二:① 留一条稳定的「当日收盘态」记录;
# ② 常驻进程若挂了/未起,日管线仍保证每日至少一批。独立表,不接事件链,失败不阻断其余步骤。
# 自选同步(issue #87):常驻 `watch-sync`(compose 服务)已覆盖 09:00/12:55/18:00;此处**再补
# 一发 one-shot**(`-once`,slot 记 manual#HH:MM 不去重)。放在 `entity_build` **之后** ——
# 让三级取名的第①级(本地 entities 查名,零外呼)能命中当日新建的实体,少走同花顺 realhead。
# 无凭据时本步会 failed(如实,不静默),首次部署须先在 /settings 填 ths_cookie。
# raw 层转载组回填(issue #83 P-4):把「同一篇稿被多家各落一行」的 raw 文档聚组、定代表,
# 供展示层回答「哪些渠道报了 + 各自链接」(取 raw 全集,事件层是子集)。判据 = P-1 正文指纹 @0.85。
# 排在 `worker` **之前**:先回填来源组,worker 抽取后展示层立即可用;与事件聚类(事件层)解耦。
STEPS=(
  "migrate|migrate"
  "collector_all|collector -driver all"
  "collector_announce|collector -driver cninfo-announce"
  "hot_topic|hot-topic"
  "cluster_raw_link|cluster-raw-link"
  "worker|worker -limit 800"
  "cluster|cluster"
  "quote_collector|quote-collector -date $TODAY"
  "entity_build|entity-build"
  "watch_sync|watch-sync -once"
  "market_state|market-state -date $TODAY"
  "daily_review|daily-review -date $TODAY"
  "reconcile|reconcile -date $TODAY"
)

# ── ③ 失败分型:按步骤日志尾部粗分,纯 shell 判定,不引 daemon ──────────────────
#     deterministic / quota → 立即放弃(重试无意义或加重限流);timeout / transient → 退避重试。
classify() {  # $1 = 步骤输出文件
  if grep -qiE 'SQLSTATE|pq: |constraint|syntax error|does not exist' "$1" 2>/dev/null; then
    echo deterministic
  elif grep -qE '配置缺失|未配置同花顺凭据' "$1" 2>/dev/null; then
    # 缺配置(#87 watch-sync 无凭据):重试 5 次不会变好,而且每次重试都多打一次同花顺
    # 登录(风控风险)。立即放弃,让它红着等人工补 /settings —— 与 #64「失败不静默」同旨。
    echo deterministic
  elif grep -qiE '(^|[^0-9])429([^0-9]|$)|rate limit|too many requests|insufficient balance|额度' "$1" 2>/dev/null; then
    echo quota
  else
    echo transient
  fi
}

# 退出码 → outcome:124/137 = timeout(TERM/KILL)。
outcome_of() {  # $1 = exit code, $2 = 输出文件
  case "$1" in
    0) echo ok ;;
    124|137) echo timeout ;;
    *) classify "$2" ;;
  esac
}

run() {  # $1 = 命令(含参数);stdout+stderr 落步骤临时文件,供分型与归档
  echo "== $(date '+%F %T %Z') $1" >> "$L"
  local out; out=$(mktemp)
  # shellcheck disable=SC2086   # $1 含参数时按空格拆分为独立参数
  timeout --signal=TERM --kill-after=30s "${STEP_TIMEOUT}s" \
    docker compose -f "$C/docker-compose.yml" run --rm -T tools ./bin/$1 > "$out" 2>&1
  local rc=$?
  cat "$out" >> "$L"
  OUTCOME=$(outcome_of "$rc" "$out"); RC=$rc; OUT_FILE=$out
}

# ── ②③ 主循环:跳过已完成/已放弃,失败按分型决策 ────────────────────────────────
# ⚠️ 边界(如实登记):`timeout` 杀的是 `docker compose run` 客户端,容器本身可能不被回收
#    (实测未复现孤儿容器,但 `docker` CLI 被 TERM 后不保证向容器转发)。加超时的首要目的是
#    **解除对串行链的堵塞**(实测曾堵 2h25m),该目标已达成;若日后观察到孤儿容器,再补
#    `docker compose rm -f` 收尾。此处不预先复杂化。
failed=0; abandoned_any=0; ran=0
for entry in "${STEPS[@]}"; do
  name="${entry%%|*}"; cmd="${entry#*|}"
  mark="$LEDGER/$name.done"
  if [ -f "$mark" ]; then continue; fi                    # 本步今日已完成 → 跳过
  failfile="$LOG/pipeline-$TODAY.fail.$name"
  fails=$(cat "$failfile" 2>/dev/null || echo 0)
  if [ "$fails" -ge "$MAX_RETRY" ]; then
    echo "== ABANDON $cmd(已放弃:$fails 次失败 ≥ $MAX_RETRY)" >> "$L"
    abandoned_any=1; continue
  fi

  ran=$((ran+1)); run "$cmd"
  if [ "$OUTCOME" = ok ]; then
    touch "$mark"; echo "== ok $cmd" >> "$L"; rm -f "$failfile"
  else
    fails=$((fails+1)); echo "$fails" > "$failfile"
    echo "== FAIL $cmd(outcome=$OUTCOME rc=$RC,当日第 $fails 次)" >> "$L"; failed=1
    if [ "$OUTCOME" = deterministic ] || [ "$OUTCOME" = quota ]; then
      echo "$MAX_RETRY" > "$failfile"     # 强制置满 ⇒ 后续 tick 直接 ABANDON,不再高频重打
      echo "== ABANDON $cmd(分类=$OUTCOME,立即放弃:重试无意义或加重限流)" >> "$L"
      abandoned_any=1
    fi
  fi
  rm -f "$OUT_FILE"
done

# 全链都完成(无失败、无放弃)才落 .done —— 语义与旧版一致:stamp = 「今日已跑完」凭证。
if [ "$failed" -eq 0 ] && [ "$abandoned_any" -eq 0 ]; then
  touch "$LOG/pipeline-$TODAY.done"
  echo "pipeline done $(date '+%F %T %Z')(本轮跑 $ran 步)" >> "$L"
elif [ "$ran" -eq 0 ]; then
  echo "无待跑步骤(已完成/已放弃),等待人工处置" >> "$L"
else
  echo "pipeline FAILED/ABANDONED above; will retry pending steps next tick" >> "$L"
fi
