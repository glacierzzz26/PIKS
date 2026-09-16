#!/usr/bin/env bash
# 清洗 issue #2 的历史脏数据:code 列被写成股票名称的行,修正为真 6 位代码。
#
# 背景:实体库页把 detail.code(曾为名称)传给深研;截图/录入兜底 code=name,
# 把名称写进了 trades / positions / entities 的 code 列。此脚本一次性订正。
#
# 用法(在生产容器里跑,连的是生产 PG):
#   ./scripts/fix-dirty-codes.sh --dry-run     # 仅打印将改动的行,不写库
#   ./scripts/fix-dirty-codes.sh --apply       # 真正执行(建议先做一次备份)
#
# 幂等:WHERE code='<名称>' 精确匹配;重跑只影响仍未订正的行。
# 映射来源:各名称的权威 6 位代码已逐条核对(entities 已有码的行 + 东财行情接口实证)。
# 只改 code 列;name 列本就正确,不动。不动 entities 的 id/name/status —— 只订正 detail.code。
set -euo pipefail

APPLY=0
case "${1:-}" in
  --apply)   APPLY=1 ;;
  --dry-run) APPLY=0 ;;
  *) echo "用法: $0 [--dry-run|--apply]" >&2; exit 2 ;;
esac

PSQL='docker exec -i piks-postgres psql -U piks -d piks -v ON_ERROR_STOP=1'

# 名称 → 6 位代码(逐条已核实,勿改错)
declare -A MAP=(
  [海南橡胶]=601118
  [黄河旋风]=600172
  [金瑞矿业]=600714
  [浙江世宝]=002703
  [沃特股份]=002886
  [恩捷股份]=002812
  [金螳螂]=002081
  [华菱线缆]=001208
  [南京港]=002040
)

echo "== 清洗前 =="
$PSQL -c "SELECT 'trades' t, count(*) FROM trades    WHERE code !~ '^[0-9]{6}\$'
          UNION ALL SELECT 'positions', count(*) FROM positions WHERE code !~ '^[0-9]{6}\$'
          UNION ALL SELECT 'entities',  count(*) FROM entities WHERE type='company' AND (detail->>'code') !~ '^[0-9]{6}\$';"

# 构造 SQL:每张表按映射逐条 UPDATE(名称精确匹配 code 列)。
SQL="BEGIN;"
for name in "${!MAP[@]}"; do
  code="${MAP[$name]}"
  SQL+="
UPDATE trades    SET code='$code', updated_at=now() WHERE code='$name';
UPDATE positions SET code='$code' WHERE code='$name';
UPDATE entities  SET detail = jsonb_set(detail, '{code}', '\"$code\"', true), updated_at=now()
  WHERE type='company' AND detail->>'code'='$name';"
done
SQL+="
-- 复查:列出剩余 code 非 6 位数字的行(应为 0)
SELECT 'trades' t, code, count(*) FROM trades    WHERE code !~ '^[0-9]{6}\$' GROUP BY code
UNION ALL SELECT 'positions', code, count(*) FROM positions WHERE code !~ '^[0-9]{6}\$' GROUP BY code
UNION ALL SELECT 'entities', (detail->>'code'), count(*) FROM entities
  WHERE type='company' AND (detail->>'code') !~ '^[0-9]{6}\$' GROUP BY detail->>'code';
"

if [ "$APPLY" -eq 1 ]; then
  echo "== 执行订正 =="
  echo "$SQL" | $PSQL
  echo "== 完成 =="
else
  echo "== DRY-RUN:以下是将执行的 SQL(未写库) =="
  echo "$SQL" | sed 's/^/  /'
  echo
  echo "(确认无误后加 --apply 执行)"
fi
