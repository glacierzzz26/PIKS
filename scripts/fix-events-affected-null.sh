#!/usr/bin/env bash
# 修 events.affected / facts 中的 **非数组** 值(issue #71)。
#
# 背景:internal/extract 曾用裸 json.Marshal 序列化 []string —— 对 nil 切片返回字面量
#   `null`(err 仍为 nil),于是 events.affected 落成 JSON null。消费方
#   `jsonb_array_elements_text(e.affected)` 遇标量即报 SQLSTATE 22023
#   「cannot extract elements from a scalar」,使 entity-build **每轮必崩**。
#   写入侧已在同 PR 修复(mustJSONArray / emptyToArrayIfScalar);本脚本清理**历史脏行**。
#
# ⚠️ 幂等:WHERE 只命中非数组行,重复执行第二次即 0 行更新。可安全重跑。
# ⚠️ 不新增迁移编号:迁移是 schema 变更通道,数据修复不污染迁移序列。
#
# 用法(在 lab 上,compose 目录内):
#   docker exec piks-postgres psql -U piks -d piks -f - < scripts/fix-events-affected-null.sql
# 或本机导入该 SQL。见 docs/phase11/design/… 与 issue #71。
set -euo pipefail
DC="${PIKS_DC:-docker compose -f /home/rguo/piks/docker-compose.yml}"
SQL="$(cd "$(dirname "$0")" && pwd)/fix-events-affected-null.sql"
echo "== 修复前分布 =="
$DC exec -T postgres psql -U piks -d piks -c \
  "select 'affected='||jsonb_typeof(affected)||' n='||count(*) from events group by 1
   union all
   select 'facts='||jsonb_typeof(facts)||' n='||count(*) from events group by 1
   order by 1"
echo "== 执行修复 =="
$DC exec -T postgres psql -U piks -d piks -f - < "$SQL"
echo "== 修复后分布 =="
$DC exec -T postgres psql -U piks -d piks -c \
  "select 'affected='||jsonb_typeof(affected)||' n='||count(*) from events group by 1
   union all
   select 'facts='||jsonb_typeof(facts)||' n='||count(*) from events group by 1
   order by 1"
