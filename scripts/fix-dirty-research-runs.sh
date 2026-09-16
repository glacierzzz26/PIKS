#!/usr/bin/env bash
# 清洗 issue #7 的存量脏 run:research_runs 里 code 列不是 6 位数字的行。
#
# 背景:issue #2 时代实体库页把 detail.code(曾为名称)传给深研,编排据此生成
# run_id 形如「金螳螂_complete-stock_20260914_224916」,Python resolve_symbol
# 拿名字 int() 直接炸 → 每次都落一条 status=failed 的垃圾记录。写入侧的入口校验
# 已由 PR #5(ValidStockCode)补上,此脚本清理在此之前产生的存量行。
#
# 用法(在生产容器里跑,连的是生产 PG):
#   ./scripts/fix-dirty-research-runs.sh --dry-run   # 仅打印将删除的行,不写库
#   ./scripts/fix-dirty-research-runs.sh --apply     # 真正执行(建议先做一次备份)
#
# 幂等:WHERE code !~ '^[0-9]{6}$';重跑只影响仍未清理的行,清完为空操作。
# 安全闸:若 relationships 有决策边(based_on 等)指向待删的 run,脚本会中止 ——
#         决策记录依赖 research_runs.id,不能连带删掉有引用的行。
set -euo pipefail

APPLY=0
case "${1:-}" in
  --apply)   APPLY=1 ;;
  --dry-run) APPLY=0 ;;
  *) echo "用法: $0 [--dry-run|--apply]" >&2; exit 2 ;;
esac

PSQL='docker exec -i piks-postgres psql -U piks -d piks -v ON_ERROR_STOP=1'

# 待删行判定:code 不是 6 位数字(名称当代码用留下的行)。
DIRTY="code !~ '^[0-9]{6}\$'"

echo "== 清洗前:待删明细 =="
$PSQL -c "SELECT run_id, status, as_of, created_at
          FROM research_runs WHERE $DIRTY ORDER BY created_at;"

echo "== 安全闸:待删行是否被决策边引用 =="
REFS=$($PSQL -tAc "SELECT count(*) FROM relationships rel
                   WHERE rel.to_id IN (SELECT id FROM research_runs WHERE $DIRTY);")
REFS=$(echo "$REFS" | tr -d '[:space:]')
echo "   引用数 = $REFS"
if [ "$REFS" != "0" ]; then
  echo "中止:有 $REFS 条关系边指向待删 run(决策记录依赖 research_runs.id)。" >&2
  echo "请先人工确认这些边，再决定是改指向还是保留对应行。" >&2
  exit 3
fi

SQL="BEGIN;
DELETE FROM research_runs WHERE $DIRTY;
-- 复查:剩余 code 非 6 位数字的行(应为 0)
SELECT run_id, status FROM research_runs WHERE $DIRTY;
COMMIT;"

if [ "$APPLY" -eq 1 ]; then
  echo "== 执行清洗 =="
  echo "$SQL" | $PSQL
  echo "== 完成:剩余脏行应为 0 =="
else
  echo "== DRY-RUN:以下是将执行的 SQL(未写库) =="
  echo "$SQL" | sed 's/^/  /'
  echo
  echo "(确认无误后加 --apply 执行)"
fi
