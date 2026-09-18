#!/usr/bin/env bash
# 清洗 issue #6 的历史脏实体:涨停池带空格名称(「金 螳 螂」)落库产生的
# 脏实体与同码重复。
#
# 背景:东财涨停池对**历史改名票**返回的名字带空格;entity-build 原样落库,
# 实体主键是 (type,name) → 名称对不上正式名,并与交易截图导入的无空格名
# 分叉成同码重复实体。写入侧已在 PR 修复(store.NormalizeStockName +
# EnsureCompanyEntity 去空白匹配);此脚本订正**存量**。
#
# 处理(仅 type='company' 且名字含空白、去空白后全为汉字的行;**英文多词名不动**):
#   A. 有同名紧凑实体 → 合并:搬 aliases(含空格原词作别名,保住可搜性)/belongs_to/
#      affects 边、补齐 detail.code 与 status,然后**删除**空格行(纯 bug 产物)。
#   B. 无同名紧凑实体 → 直接改名为紧凑形。
# 同时把紧凑实体的脏 code(#2 遗留的「名称当 code」)补成真 6 位代码。
#
# 用法(在生产容器里跑,连的是生产 PG):
#   ./scripts/fix-space-names.sh --dry-run   # 打印将改动的行(默认)
#   ./scripts/fix-space-names.sh --apply     # 真正执行(先自动备份)
#
# ⚠️ 生产执行需人工确认。--apply 会先建备份表 <表>_bak_issue6_<时间戳>,
#    便于回滚;备份表不自动清理,确认无误后自行 DROP。
#
# 幂等:改名/合并在处理后名字不再含空格,重跑零影响;合并是集合运算,不叠加。
set -euo pipefail

APPLY=0
case "${1:-}" in
  --apply)   APPLY=1 ;;
  --dry-run) APPLY=0 ;;
  *) echo "用法: $0 [--dry-run|--apply]" >&2; exit 2 ;;
esac

PSQL="${PIKS_PSQL:-docker exec -i piks-postgres psql -U piks -d piks -v ON_ERROR_STOP=1}"

echo "== 清洗前:含空白的公司实体 =="
$PSQL -c "
SELECT e.name, e.detail->>'code' AS code, e.status,
       (SELECT count(*) FROM entities t
         WHERE t.type='company' AND t.name = regexp_replace(e.name,'\s','','g')) AS compact_twin
FROM entities e
WHERE e.type='company'
  AND e.name ~ '\s'
  AND regexp_replace(e.name,'\s','','g') ~ '^[一-龥]+\$'
ORDER BY e.name;"

# 待执行的订正体:DO 块内逐行判断 A/B 分支。备份在 APPLY 分支单独做。
read -r -d '' CLEAN_SQL <<'SQL' || true
DO $$
DECLARE
  s            RECORD;
  tid          UUID;
  compact      TEXT;
  moved_out    INT;
  moved_in     INT;
  dropped_dup  INT;
BEGIN
  FOR s IN
    SELECT id, name, aliases, detail, status
    FROM entities
    WHERE type='company'
      AND name ~ '\s'
      AND regexp_replace(name,'\s','','g') ~ '^[一-龥]+$'
    ORDER BY name
  LOOP
    compact := regexp_replace(s.name, '\s', '', 'g');

    SELECT id INTO tid FROM entities
     WHERE type='company' AND name = compact LIMIT 1;

    IF tid IS NULL THEN
      -- B. 无紧凑同名 → 直接改名
      UPDATE entities SET name = compact, updated_at = now() WHERE id = s.id;
      RAISE NOTICE '改名: % → %', s.name, compact;
    ELSE
      -- A. 有紧凑同名 → 合并进紧凑实体,再删空格行
      -- A1. aliases:并集(含空格原词,保留搜索命中;紧凑名不重复入别名)
      UPDATE entities t SET aliases = COALESCE((
        SELECT jsonb_agg(DISTINCT a) FROM (
          SELECT jsonb_array_elements_text(t.aliases) AS a
          UNION SELECT jsonb_array_elements_text(s.aliases)
          UNION SELECT s.name WHERE s.name <> t.name
        ) u WHERE a IS NOT NULL AND a <> t.name
      ), '[]'::jsonb)
      WHERE t.id = tid;

      -- A2. detail.code:紧凑实体缺码或是脏码(非 6 位数字)时,用空格行的正确码补
      UPDATE entities t SET detail = jsonb_set(t.detail, '{code}', to_jsonb(s.detail->>'code'), true)
      WHERE t.id = tid
        AND (s.detail->>'code') ~ '^[0-9]{6}$'
        AND COALESCE((t.detail->>'code'), '') !~ '^[0-9]{6}$';

      -- A3. status:任一为 watch 则 watch(watch > active > archived)
      UPDATE entities t SET status = 'watch'
      WHERE t.id = tid AND s.status = 'watch' AND t.status <> 'watch';

      -- A4. 搬边:先搬不撞唯一键的,再删撞掉的重复行
      UPDATE relationships r SET from_id = tid
      WHERE r.from_type='entity' AND r.from_id = s.id
        AND NOT EXISTS (SELECT 1 FROM relationships x
                         WHERE x.from_type=r.from_type AND x.from_id=tid
                           AND x.to_type=r.to_type AND x.to_id=r.to_id
                           AND x.rel_type=r.rel_type);
      GET DIAGNOSTICS moved_out = ROW_COUNT;
      DELETE FROM relationships r
      WHERE r.from_type='entity' AND r.from_id = s.id
        AND EXISTS (SELECT 1 FROM relationships x
                     WHERE x.from_type=r.from_type AND x.from_id=tid
                       AND x.to_type=r.to_type AND x.to_id=r.to_id
                       AND x.rel_type=r.rel_type AND x.id <> r.id);
      GET DIAGNOSTICS dropped_dup = ROW_COUNT;

      UPDATE relationships r SET to_id = tid
      WHERE r.to_type='entity' AND r.to_id = s.id
        AND NOT EXISTS (SELECT 1 FROM relationships x
                         WHERE x.from_type=r.from_type AND x.from_id=r.from_id
                           AND x.to_type=r.to_type AND x.to_id=tid
                           AND x.rel_type=r.rel_type);
      GET DIAGNOSTICS moved_in = ROW_COUNT;
      DELETE FROM relationships r
      WHERE r.to_type='entity' AND r.to_id = s.id
        AND EXISTS (SELECT 1 FROM relationships x
                     WHERE x.from_type=r.from_type AND x.from_id=r.from_id
                       AND x.to_type=r.to_type AND x.to_id=tid
                       AND x.rel_type=r.rel_type AND x.id <> r.id);

      DELETE FROM entities WHERE id = s.id;
      RAISE NOTICE '合并: % → %(搬出 % 边 / 搬入 % 边 / 去重删 % 行)', s.name, compact, moved_out, moved_in, dropped_dup;
    END IF;
  END LOOP;
END $$;
SQL

if [ "$APPLY" -eq 1 ]; then
  TS="$(date +%Y%m%d_%H%M%S)_$$"   # 加 PID:同一秒内连跑两次也不会撞表名
  echo "== 备份(回滚用:INSERT INTO entities SELECT * FROM entities_bak_issue6_${TS}) =="
  $PSQL -c "
    CREATE TABLE entities_bak_issue6_${TS}      AS SELECT * FROM entities;
    CREATE TABLE relationships_bak_issue6_${TS} AS SELECT * FROM relationships;"
  echo "== 执行订正 =="
  echo "$CLEAN_SQL" | $PSQL
  echo "== 清洗后复查(应为 0 行) =="
  $PSQL -c "
    SELECT name, detail->>'code' AS code FROM entities
    WHERE type='company' AND name ~ '\s'
      AND regexp_replace(name,'\s','','g') ~ '^[一-龥]+\$';"
  echo "== 回滚:见上方备份表名(entities_bak_issue6_${TS} / relationships_bak_issue6_${TS}) =="
fi

if [ "$APPLY" -eq 0 ]; then
  echo "== DRY-RUN:以下是将执行的订正逻辑(未写库) =="
  echo "$CLEAN_SQL" | sed 's/^/  /'
  echo
  echo "(确认无误后加 --apply 执行;--apply 会先建备份表)"
fi
