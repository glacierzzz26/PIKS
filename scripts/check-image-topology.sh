#!/usr/bin/env bash
# 镜像拓扑 CI 检查(2026-09-20 容器拆分,issue #47)。
#
# 不变量(拆分的全部意义所在 —— 破了就等于没拆):
#   1. 四个 target 都存在,且各自的底座不互相污染:
#      gateway 无 Go 无 Python;web 无 nginx 无 Python;tools 无 Python;
#      只有 research 含 Python 运行时与 pip 依赖。
#   2. tools 只含 11 个管线命令 —— 不该出现 web/research-*/probe
#      (逐条 COPY 的意义就是让这一点可断言;整目录 COPY 会混入 14 个二进制)。
#   3. gateway 不含 Go 二进制;web 不含前端 dist 与 nginx(静态文件归 gateway)。
#   4. 「web 里没有 python3」是本拆分的**存在理由**:2026-09-12 单镜像时期
#      UI 深研必挂的根因就是 .py 执行依赖;若 web 再次出现 Python,说明拆分回退了。
#
# ⚠️ 所有段落检查都**先剥掉注释**再看 —— Dockerfile 里的注释常写「不用 X」「已去掉 Y」,
# 直接 grep 原文会把「说明自己没做的事」误判成「做了这件事」。
#
# 只查 Dockerfile 文本(不依赖 docker daemon,CI 可跑);镜像实际内容由 P3 实测另验。
set -uo pipefail
cd "$(dirname "$0")/.."

DOCKERFILE=Dockerfile
fail=0

die() { echo "✗ $1" >&2; fail=1; }

[ -f "$DOCKERFILE" ] || { echo "✗ 找不到 $DOCKERFILE" >&2; exit 1; }

# section <target>:输出该 target 段落中**去掉注释行**后的指令行。
section() {
  awk -v s="FROM .* AS $1\$" '
    $0 ~ "^"s { f=1; next }
    /^FROM /  { f=0 }
    f && $0 !~ /^[[:space:]]*#/ && $0 !~ /^[[:space:]]*$/ { print }
  ' "$DOCKERFILE"
}

# ── 1. 四个 target 都在 ────────────────────────────────────────────────────
for t in gateway web tools research; do
  if ! grep -qE "^FROM [^ ]+ AS ${t}\$" "$DOCKERFILE"; then
    die "Dockerfile 缺 target: $t(应为 'FROM <base> AS ${t}')"
  fi
done

# ── 2. 底座断言(每个 target 的 FROM 行)──────────────────────────────────
base_of() { grep -E "^FROM [^ ]+ AS $1\$" "$DOCKERFILE" | awk '{print $2}'; }

g_base="$(base_of gateway)"
w_base="$(base_of web)"
t_base="$(base_of tools)"
r_base="$(base_of research)"

case "$g_base" in nginx:*) ;; *) die "gateway 底座应为 nginx:*(纯网关,不带 Go/Python),实际 $g_base";; esac
case "$w_base" in alpine:*) ;; *) die "web 底座应为 alpine:*(纯 Go 静态二进制),实际 $w_base";; esac
case "$t_base" in alpine:*) ;; *) die "tools 底座应为 alpine:*(管线命令,不含 Python),实际 $t_base";; esac
# research 必须留 debian slim:akshare/pandas 无法在 musl 上构建。
case "$r_base" in python:*) ;; *) die "research 底座应为 python:*(akshare/pandas 需 glibc),实际 $r_base";; esac

# ── 3. 只有 research 装 Python 依赖 ───────────────────────────────────────
for seg in gateway web tools; do
  if section "$seg" | grep -qE '(pip install|apk add .*python|apt-get install .*python)'; then
    die "$seg target 出现 Python 安装 —— 只有 research 该含 Python 运行时"
  fi
done
if ! section research | grep -q 'pip install'; then
  die "research target 未见 pip install(深研运行时缺依赖)"
fi

# ── 4. tools 只含 11 个管线命令,不含 web/research-*/probe ────────────────
tools_body="$(section tools)"
if echo "$tools_body" | grep -qE 'COPY --from=build /out/bin/? /app/bin/?$'; then
  die "tools 用整目录 COPY /out/bin/ —— 会把 web/research-*/probe 一并塞入(应逐条列 11 个管线命令)"
fi
for c in web research-run research-worker probe; do
  if echo "$tools_body" | grep -qE "COPY --from=build /out/bin/${c}\b"; then
    die "tools 段混入非管线命令: $c"
  fi
done
# 正向:11 个管线命令应一个不少。
for c in migrate collector hot-topic watch-sync worker cluster quote-collector entity-build market-state daily-review reconcile; do
  if ! echo "$tools_body" | grep -qE "COPY --from=build /out/bin/${c}\b"; then
    die "tools 段缺管线命令: $c"
  fi
done

# ── 5. 前端 dist 只进 gateway;Go 二进制只进它该进的 target ───────────────
dist_copies="$(grep -cE '^\s*COPY --from=frontend' "$DOCKERFILE")"
if [ "$dist_copies" -ne 1 ]; then
  die "前端 dist 应只被 COPY 一次(进 gateway),实际 $dist_copies 次"
fi
if ! section gateway | grep -q 'COPY --from=frontend'; then
  die "前端 dist 未进 gateway(SPA 静态文件无处服务)"
fi
# gateway 不该含任何 Go 二进制。
if section gateway | grep -q 'COPY --from=build'; then
  die "gateway 出现 COPY --from=build —— 网关照理只服务静态 + 反代,不含 Go"
fi

# ── 6. web 不该含 nginx / 前端静态文件(那是 gateway 的职责)──────────────
web_body="$(section web)"
if echo "$web_body" | grep -qE 'nginx'; then
  die "web target 混入 nginx(拆分的意义是 web 只剩 Go API)"
fi
if echo "$web_body" | grep -qE 'frontend/dist'; then
  die "web target 混入前端 dist(静态文件归 gateway)"
fi
if ! echo "$web_body" | grep -q 'COPY --from=build /out/bin/web'; then
  die "web target 未 COPY bin/web"
fi

if [ "$fail" -eq 0 ]; then
  echo "✓ 镜像拓扑检查通过:gateway(nginx)/ web(纯 Go)/ tools(11 管线命令)/ research(Python)"
fi
exit "$fail"
