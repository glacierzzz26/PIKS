#!/usr/bin/env bash
# 事件类型枚举「单一真源」CI 检查(issue #61)。
#
# 不变量:**事件类型枚举全仓只有一处定义** —— `internal/model/eventtype.go` 的
# `model.EventTypes`(key + 中文 label)。后端校验集 / JSON Schema / 前端展示与筛选
# 全部由它派生。
#
# 破了会怎样(issue #61 的真实事故):前端曾自存一份 **8 值**枚举,与后端 **9 值**
# 权威枚举漂移 —— 其中 6 个 key 全仓零出现,是前端臆造的。后果是 92% 事件在表格里
# 露英文原值、类型下拉 8 项里 6 项永远筛不出东西。**漂移是静默的**:编译过、构建过、
# 跑得起来,只有用户看见 `industry_event` 这种英文 key 才知道错了。所以要用脚本挡。
#
# 三个断言:
#   1. 真源自身合法:key 非空唯一、label 非空(漏 label 会让前端静默回落英文)。
#   2. 派生是**真派生**:`extract` 不得再手抄一份 enum(key 字面量只能出现于真源)。
#   3. 前端不得再存本地枚举表(禁止臆造 key 重新出现)。
#
# ⚠️ 与 `check-image-topology.sh` 同样的手法:前端断言前**剥掉注释**,
# 否则「说明自己不再存类型表」的注释会被当成「存了类型表」。
set -uo pipefail
cd "$(dirname "$0")/.."

SRC=internal/model/eventtype.go
fail=0

die() { echo "✗ $1" >&2; fail=1; }

[ -f "$SRC" ] || { echo "✗ 找不到真源 $SRC" >&2; exit 1; }

# ── 1. 真源自身合法 ────────────────────────────────────────────────────────
# 只取 {Key: "x", Label: "y"} 形态的行。
keys_and_labels="$(
  grep -oE '\{Key:[[:space:]]*"[^"]*",[[:space:]]*Label:[[:space:]]*"[^"]*"\}' "$SRC"
)"
if [ -z "$keys_and_labels" ]; then
  die "$SRC 里没解析到任何 {Key,Label} 项(枚举被改写了?)"
fi

keys="$(printf '%s\n' "$keys_and_labels" | sed -E 's/.*Key:[[:space:]]*"([^"]*)".*/\1/')"
labels="$(printf '%s\n' "$keys_and_labels" | sed -E 's/.*Label:[[:space:]]*"([^"]*)".*/\1/')"

key_n="$(printf '%s\n' "$keys" | grep -c .)"
label_n="$(printf '%s\n' "$labels" | grep -c .)"
if [ "$key_n" -ne "$label_n" ]; then
  die "真源解析出的 key($key_n)与 label($label_n)数量不等 —— 有项漏了 label 或 key"
fi

# 空 label:前端会静默回落英文 key(漂移只修一半)。
if printf '%s\n' "$labels" | grep -qE '^[[:space:]]*$'; then
  die "真源存在空 label 的类型 —— 前端将回落显示英文 key"
fi

# 空 key / 重复 key。
if printf '%s\n' "$keys" | grep -qE '^[[:space:]]*$'; then
  die "真源存在空 key 的类型"
fi
dupes="$(printf '%s\n' "$keys" | sort | uniq -d)"
if [ -n "$dupes" ]; then
  die "真源存在重复 key: $(echo "$dupes" | tr '\n' ' ')"
fi
# key 必须是干净的机器标识(小写字母/数字/下划线)—— 空格或大写会污染 URL query 与 SQL 过滤。
if printf '%s\n' "$keys" | grep -qvE '^[a-z][a-z0-9_]*$'; then
  die "真源存在不规范 key(应为 ^[a-z][a-z0-9_]*\$): $(printf '%s\n' "$keys" | grep -vE '^[a-z][a-z0-9_]*$' | tr '\n' ' ')"
fi

# ── 2. extract 是真派生,不是手抄 ──────────────────────────────────────────
# extract.go 里**不得出现任何**事件类型 key 字面量(连兜底桶 "other" 也不行 ——
# 见 model.EventTypeFallback)。只允许经 model.EventTypeCSV()/EventTypeKeys() 派生。
# 先剥注释:注释里引 key 是为了解释历史,不算硬编码。
# ⚠️ 按**裸 key 词边界**匹配,不是 `"key"` —— 手抄的 enum 常写成 CSV 串
# (`"policy,earnings,…"`),其中 `"policy"` 后面不是引号,带引号的模式会漏掉。
while IFS= read -r k; do
  [ -n "$k" ] || continue
  hit="$(sed -E 's://.*::' internal/extract/extract.go | grep -nE "\b${k}\b" || true)"
  if [ -n "$hit" ]; then
    echo "✗ internal/extract/extract.go 出现事件类型 key \"${k}\" —— 应经 model 派生(含兜底桶):" >&2
    echo "$hit" | sed 's/^/    /' >&2
    fail=1
  fi
done < <(printf '%s\n' "$keys")
# 反向:必须真的引用了真源(否则上面那条「没硬编码」是因为压根没接上)。
if ! grep -q 'model\.EventType\(CSV\|Keys\|Fallback\)' internal/extract/extract.go; then
  die "internal/extract/extract.go 未引用 model.EventType* —— enum 未与真源接上"
fi

# ── 3. 前端不得再存本地枚举表 ──────────────────────────────────────────────
# ⚠️ **不能**简单地「前端出现 key 字面量就报错」—— `industry` / `macro` / `company`
# 等 key 与**其它正交枚举**撞名(实体类型 company/industry、研报 profile、
# subject_type),那几处**本就该**在前端硬编码(它们各有各的后端真源,不在本守卫范围)。
# 前端存「事件类型表」的特征是**把 key 与其事件类型 label 并置同一行** ——
# 那正是漂移的载体(label 是前端臆造的)。故只断言这个组合。
front_hits=0
while IFS='|' read -r k l; do
  [ -n "$k" ] || continue
  hits="$(
    find frontend/src -type f \( -name '*.ts' -o -name '*.tsx' \) -print0 \
      | xargs -0 grep -nE "[\"']${k}[\"']" 2>/dev/null \
      | grep -E "[\"']${l}[\"']" \
      | sed -E 's://.*::' \
      | grep -E "[\"']${k}[\"']" || true
  )"
  if [ -n "$hits" ]; then
    echo "✗ 前端出现事件类型映射表项: ${k} → ${l}(应经 useEventTypes() 消费后端枚举):" >&2
    echo "$hits" | sed 's/^/    /' >&2
    front_hits=1
  fi
done < <(paste -d'|' <(printf '%s\n' "$keys") <(printf '%s\n' "$labels"))
[ "$front_hits" -eq 1 ] && fail=1

# 臆造 key:曾经前端自造过 6 个全仓零出现的 key,是漂移的直接来源。见一个报一个。
for bogus in product_launch supply_agreement industry_event sales_data rumor; do
  hits="$(
    find frontend/src -type f \( -name '*.ts' -o -name '*.tsx' \) -print0 \
      | xargs -0 grep -n "[\"']${bogus}[\"']" 2>/dev/null \
      | sed -E 's://.*::' | grep -E "[\"']${bogus}[\"']" || true
  )"
  if [ -n "$hits" ]; then
    echo "✗ 前端出现臆造的事件类型 key \"$bogus\"(不在真源 EventTypes 中):" >&2
    echo "$hits" | sed 's/^/    /' >&2
    front_hits=1
  fi
done
[ "$front_hits" -eq 1 ] && fail=1

# 反向:前端必须真的消费了接口(否则「没有硬编码」是因为压根没接)。
if ! grep -q 'ENDPOINTS\.eventTypes\|useEventTypes' frontend/src/lib/eventTypes.tsx 2>/dev/null; then
  die "frontend/src/lib/eventTypes.tsx 未消费 /api/v1/event-types —— 前端未与真源接上"
fi

# ── 4. 后端路由在高 ───────────────────────────────────────────────────────
if ! grep -q '"/api/v1/event-types"' internal/web/server.go 2>/dev/null; then
  die "internal/web/server.go 未注册 /api/v1/event-types 路由"
fi

if [ "$fail" -eq 0 ]; then
  echo "✓ 事件类型枚举检查通过:$key_n 类,单一真源=$SRC,extract 派生、前端经接口消费"
fi
exit "$fail"
