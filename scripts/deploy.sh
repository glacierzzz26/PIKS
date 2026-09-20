#!/usr/bin/env bash
# PIKS 生产更新/部署(在 dev 侧执行):本地构建四镜像 → 按需传输到 lab → migrate → 起 web/research/gateway。
# 生产代码变更经此命令生效;lab 不保留代码仓库,镜像为唯一交付物。
#
# ── 四镜像(2026-09-20 容器拆分,issue #47)────────────────────────────────
#   gateway  纯 nginx(SPA 静态 + 反代 /api/*)      ← 前端/nginx.conf 变
#   web      纯 Go JSON API                        ← cmd/web、internal/* 变
#   tools    9 个管线命令 + migrate                 ← 同上 + migrations/prompts
#   research Python 深研运行时 + 队列 worker        ← research/*、internal/research 变
#
# **按镜像判定「是否要重建/传输」**:每个镜像的版本 = 它自己输入的 git tree hash
# (非 HEAD hash)—— 前端改动不会改 web/tools/research 的 tag,于是它们不重建也不传输。
# 这正是拆分要换来的东西:改动的影响半径 = 实际改的那一块。
#
# ⚠️ **干净树门控**:镜像从**工作树**构建却以**版本 tag** 命名。工作树脏时构建出的镜像
# 会被贴上「不含该改动」的 tag(标签与内容不符)。故默认拒绝脏树;确需可用
# PIKS_ALLOW_DIRTY=1 放行(tag 附 -dirty,如实标注)。
# ─────────────────────────────────────────────────────────────────────────
set -euo pipefail
REPO="$(cd "$(dirname "$0")/.." && pwd)"
LAB="${PIKS_LAB:-rguo@192.168.0.202}"
C=/home/rguo/piks

# 版本号(发布纪律,见 CLAUDE.md):**默认恒为 v0.0.0** —— 只有 HEAD 上打了语义化
# tag(正式发版)才取该 tag。不做「改动挺大就造个号」这类推测。
VER="$(git -C "$REPO" describe --tags --exact-match 2>/dev/null || echo v0.0.0)"
GS="$(git -C "$REPO" rev-parse --short HEAD)"
# 国内 pypi.org 时常长读超时(实测本机 15s 无响应),默认走清华镜像;可 PIKS_PYPI_INDEX 覆盖。
# ⚠️ 2026-09-20 实测(issue #56 部署卡住的真因):aliyun 源对**本机**只有 ~20 KB/s
#   (同机 curl 命中其 CDN 可达 4 MB/s,但 Python/pip 的连接恒落坏节点;换 UA 无效),
#   清华源实测 **5.58 MB/s**(同机同时刻)。故默认由 aliyun 改为 tsinghua ——
#   aliyun 会让 pip 层下几十 MB 依赖耗时数十分钟,是部署超时的根因。
#   注意这是**取数链路的环境事实,非代码缺陷**;换线路后需重测,不当作永久结论。
PYPI_INDEX="${PIKS_PYPI_INDEX:-https://pypi.tuna.tsinghua.edu.cn/simple}"

# ── 干净树门控 ────────────────────────────────────────────────────────────
DIRTY=""
if [ -n "$(git -C "$REPO" status --porcelain)" ]; then
  if [ "${PIKS_ALLOW_DIRTY:-0}" = "1" ]; then
    DIRTY="-dirty"
    echo "!! 工作树脏,PIKS_ALLOW_DIRTY=1 放行 —— tag 附 -dirty(内容与 tag 可能不符)"
  else
    echo "!! 工作树有未提交改动:镜像从工作树构建却以版本 tag 命名,会造成「标签与内容不符」。" >&2
    echo "   请先提交,或显式 PIKS_ALLOW_DIRTY=1 放行(仅调试用)。" >&2
    exit 1
  fi
fi

# ── 按镜像的输入算 hash(镜像内容 ↔ 源码状态一一对应)────────────────────
# ⚠️ 输入集必须**覆盖该镜像构建时真正读的每一个文件** —— 少算一个就是「改了文件、tag 不动、
# 于是不重建不传输,生产跑旧像」的静默错误(比多算严重得多)。故:
#   - 前端:整个 frontend/ 的 git tree(递归覆盖 index.html/vite.config.ts/tailwind.config.ts/
#     package-lock.json 等全部构建输入;node_modules/dist 本就 gitignore,不在 tree 里)。
#   - Go:**用 `go list -deps` 取该命令的真实依赖闭包**,而非手列目录 —— 手列会在「新增一个
#     import」时静默漏掉(实测:research-worker 依赖 internal/model,手列清单曾漏它)。
tree_hash() {  # 参数:git 路径(文件或目录);目录递归覆盖其全部跟踪文件
  local args=()
  for p in "$@"; do args+=("HEAD:$p"); done
  git -C "$REPO" rev-parse "${args[@]}" | git hash-object --stdin | cut -c1-7
}

# go_deps_hash <cmd>:该命令 Go 依赖闭包(内部包)的树 hash。
# 无宿主 go 工具链时**回退到整个 cmd+internal 树**(过近似 = 安全方向:宁可多重建,不可漏)。
go_deps_hash() {
  local deps args=()
  deps="$(cd "$REPO" && go list -deps -f '{{.ImportPath}}' "./cmd/$1" 2>/dev/null \
          | sed -n 's|^piks/||p')"
  if [ -z "$deps" ]; then
    echo "-- 警告:go list 不可用,回退整树 hash($1 会随任何 Go 改动重建)" >&2
    tree_hash cmd internal
    return
  fi
  while IFS= read -r d; do [ -n "$d" ] && args+=("HEAD:$d"); done <<< "$deps"
  git -C "$REPO" rev-parse "${args[@]}" | git hash-object --stdin | cut -c1-7
}

short() { printf '%s' "$1" | git hash-object --stdin | cut -c1-7; }

# ⚠️ 四处都带 `Dockerfile`(整文件树 hash)与 run_deps_hash —— issue #55 的教训:
#   此前 Dockerfile 不在任何输入集里 → 改了 Dockerfile 也不改 tag → build_image 判定
#   「已存在、跳过构建」→ 修好的镜像既不重建也不传输,生产静默跑旧像(修复本身失效)。
#   `HEAD:Dockerfile` 是整文件 hash(粗粒度,改注释也触发重建)—— 这是**故意的**:
#   方向安全(宁可多重建,不可漏),且 Dockerfile 极少改动。
TAG_GATEWAY="${VER}-$(short "$(tree_hash frontend configs/nginx.conf)$(tree_hash Dockerfile)")${DIRTY}"
TAG_WEB="${VER}-$(short "$(tree_hash go.mod go.sum)$(go_deps_hash web)$(tree_hash Dockerfile)")${DIRTY}"
TAG_TOOLS="${VER}-$(short "$(tree_hash go.mod go.sum migrations prompts)$(go_deps_hash migrate)$(tree_hash Dockerfile)")${DIRTY}"
TAG_RESEARCH="${VER}-$(short "$(tree_hash go.mod research)$(go_deps_hash research-run)$(go_deps_hash research-worker)$(tree_hash Dockerfile)")${DIRTY}"
STACK_TAG="${VER}-${GS}${DIRTY}"

echo "== 栈 ${STACK_TAG}(commit ${GS})"
echo "   gateway  → ${TAG_GATEWAY}"
echo "   web      → ${TAG_WEB}"
echo "   tools    → ${TAG_TOOLS}"
echo "   research → ${TAG_RESEARCH}"

# ── 构建(按 tag 已存在则跳过:同一输入集不重复构建)────────────────────
build_image() {
  local role="$1" tag="$2"
  if docker image inspect "piks-${role}:${tag}" >/dev/null 2>&1 && [ "${PIKS_FORCE_BUILD:-0}" != "1" ]; then
    echo "-- ${role} ${tag} 已存在,跳过构建"
    return
  fi
  echo "== build ${role} (${tag})"
  local extra=()
  [ "$role" = "research" ] && extra=(--build-arg PYPI_INDEX="$PYPI_INDEX")
  docker build --target "$role" \
    --build-arg GIT_SHORT="$GS" \
    --build-arg PIKS_VERSION="$VER" \
    "${extra[@]}" \
    -t "piks-${role}:latest" -t "piks-${role}:${tag}" "$REPO"
}

build_image gateway  "$TAG_GATEWAY"
build_image web      "$TAG_WEB"
build_image tools    "$TAG_TOOLS"
build_image research "$TAG_RESEARCH"

# ── 保留回滚镜像(升级前的 latest → rollback-pre-<阶段>)──────────────────
# ⚠️ **只快照一次,绝不覆盖**:回滚点必须指向「本次升级之前」的形态,而它是**一次性**的。
# 若无脑 `docker tag latest rollback-pre-X`,第二次部署时 latest 已是升级后的新镜像 →
# 回滚点被悄悄改写成新镜像,名字还叫 rollback-pre-X(**标签与内容不符**,真回滚时才发现
# 回滚不了)。故存在即跳过,只报「已存在」。
PHASE="${PIKS_PHASE:-split}"
echo "== 在 lab 保留回滚镜像 rollback-pre-${PHASE}(一次性快照,已存在则不覆盖)"
ssh "$LAB" "for i in gateway web tools research; do \
  if docker image inspect piks-\$i:rollback-pre-${PHASE} >/dev/null 2>&1; then \
    echo \"  \$i: rollback-pre-${PHASE} 已存在,保留不动\"; \
  elif docker image inspect piks-\$i:latest >/dev/null 2>&1; then \
    docker tag piks-\$i:latest piks-\$i:rollback-pre-${PHASE} && echo \"  \$i: 快照 latest → rollback-pre-${PHASE}\"; \
  fi; \
done; true"
# 拆分前的**旧单镜像**再单独留一份(回滚到拆分前形态需要;它含 nginx+Go+Python 三件套)。
# 同样只快照一次:仅在 tag 不存在时从 lab 上**现存的 934MB 单镜像**打 —— 从 v0.0.0-b996864
# 这一已知的拆分前 tag 取,而非 latest(拆分后 latest 已是 piks-tools 新像,不含 nginx)。
PRE_SPLIT_REF="${PIKS_PRE_SPLIT_REF:-piks-tools:v0.0.0-b996864}"
ssh "$LAB" "if docker image inspect piks-tools:rollback-pre-${PHASE}-single >/dev/null 2>&1; then \
    echo '  tools(single): rollback-pre-${PHASE}-single 已存在,保留不动'; \
  elif docker image inspect ${PRE_SPLIT_REF} >/dev/null 2>&1; then \
    docker tag ${PRE_SPLIT_REF} piks-tools:rollback-pre-${PHASE}-single && echo \"  tools(single): 快照 ${PRE_SPLIT_REF} → rollback-pre-${PHASE}-single\"; \
  else \
    echo \"  ⚠️ 未找到拆分前单镜像 ${PRE_SPLIT_REF},rollback-pre-${PHASE}-single 未建立\" >&2; \
  fi"

# ── 传输:只发 lab 上还没有的 tag ─────────────────────────────────────────
TO_SEND=()
for pair in "gateway:$TAG_GATEWAY" "web:$TAG_WEB" "tools:$TAG_TOOLS" "research:$TAG_RESEARCH"; do
  role="${pair%%:*}"; tag="${pair#*:}"
  if ssh "$LAB" "docker image inspect piks-${role}:${tag}" >/dev/null 2>&1; then
    echo "-- ${role} ${tag} lab 已有,跳过传输"
  else
    TO_SEND+=("piks-${role}:latest" "piks-${role}:${tag}")
  fi
done
if [ "${#TO_SEND[@]}" -gt 0 ]; then
  echo "== transfer ${#TO_SEND[@]} tags (docker save | ssh docker load)"
  docker save "${TO_SEND[@]}" | ssh "$LAB" docker load
else
  echo "== 四镜像 lab 均已具备,无传输"
fi

# ── 同步 compose ─────────────────────────────────────────────────────────
echo "== sync prod compose to lab"
scp "$REPO/configs/docker-compose.prod.yml" "$LAB:$C/docker-compose.yml"
DC="docker compose -f $C/docker-compose.yml"

# ── 同步 lab 侧运维脚本 ───────────────────────────────────────────────────
# ⚠️ 这是编排漂移的根因修复:此前只同步 compose,scripts/ 从不同步 →
#   lab 上的 pipeline.sh 长期停在旧版(实测仍跑 `collector -driver dongcai` +
#   `worker` 默认 limit 50,而仓库早已是 `-driver all` + `-limit 300` 六机构源),
#   于是「仓库改了采集编排」在生产上从未生效。脚本随部署同步,杜绝再次漂移。
# 只同步**在 lab 上运行**的脚本(管线/备份/自检/安装);check-* 与 fix-* 是 dev 侧工具,不上 lab。
ssh "$LAB" "mkdir -p $C/scripts"
scp "$REPO/scripts/pipeline.sh" "$REPO/scripts/backup.sh" \
    "$REPO/scripts/health.sh" "$REPO/scripts/setup.sh" \
    "$LAB:$C/scripts/"
ssh "$LAB" "chmod +x $C/scripts/*.sh"

# ── 上线:postgres → migrate → web/research → gateway ─────────────────────
# ⚠️ 顺序是硬约束:
#   1. migrate 必须先于 web —— cmd/web/main.go 启动即读 app_config,缺表会 fatal 崩溃循环。
#   2. gateway 必须最后 —— 否则 nginx 会短暂反代到半启动的 web。
# 另:compose 的 gateway depends_on web(service_healthy),故 web 起来后才可能起 gateway。
echo "== rollout: postgres → migrate → web/research → gateway"
ssh "$LAB" "$DC up -d postgres"
ssh "$LAB" "until $DC exec -T postgres pg_isready -U piks -d piks >/dev/null 2>&1; do sleep 2; done"
ssh "$LAB" "$DC run --rm tools ./bin/migrate"
ssh "$LAB" "$DC up -d web research"
ssh "$LAB" "$DC up -d gateway"

# ── 栈清单(「生产在跑什么」的单一答案)───────────────────────────────────
MANIFEST="$(printf '{"stack_tag":"%s","commit":"%s","built_at":"%s","images":{"gateway":"%s","web":"%s","tools":"%s","research":"%s"}}' \
  "$STACK_TAG" "$GS" "$(date -Is)" "$TAG_GATEWAY" "$TAG_WEB" "$TAG_TOOLS" "$TAG_RESEARCH")"
echo "$MANIFEST" > /tmp/piks-stack-manifest.json
scp /tmp/piks-stack-manifest.json "$LAB:$C/stack-manifest.json"
rm -f /tmp/piks-stack-manifest.json

echo
echo "deploy done."
echo "生产跑栈 ${STACK_TAG}(dev 提交 ${GS}):"
echo "  gateway ${TAG_GATEWAY} / web ${TAG_WEB} / tools ${TAG_TOOLS} / research ${TAG_RESEARCH}"
echo "核对:curl -fsS http://${LAB#*@}:8090/api/v1/dashboard"
