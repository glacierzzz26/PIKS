#!/usr/bin/env bash
# D-11 独立性 CI 检查(design research-merge.md §4.10 G1/G3,验收 §5.6)。
#
# 不变量:Go 与前端**不依赖 research 的源码**;二者只经「CLI 参数 + 产物文件」
# 这一冻结接口(§4.10 G4)交互。破坏它 = 独立迭代失效(改 Python 要动 Go/前端)。
#
# 允许的耦合(白名单,即冻结接口本身):
#   - internal/research/runner.go: 探测/执行 `research/src/cli.py`(CLI 入口路径)
#   - internal/research/artifacts.go: 读 `run_meta.json` 等产物文件名(契约表)
#   - 任意注释/文档字符串里的 "research/"
# 禁止的耦合:
#   - Go 用 go:embed / 读 research 的 .py 源文件
#   - 前端 import research 的代码或读其源文件
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0

# 1) Go 侧:不得 embed 或读 .py 源文件。
#    (exec 调用 CLI 不算——那是运行时接口,不是构建期依赖。)
if grep -rn 'go:embed.*research' internal/ cmd/ 2>/dev/null; then
  echo "✗ Go 侧出现 go:embed research(构建期依赖,违反 G3)" >&2
  fail=1
fi
# 例外:`src/cli.py` 就是冻结的 CLI 入口(§4.10 G4),runner.go 探测它是接口而非源码依赖。
if grep -rn '\.py"' internal/ cmd/ 2>/dev/null \
    | grep -v '_test.go' \
    | grep -v '"cli\.py"' \
    | grep -v '^\s*//'; then
  echo "✗ Go 侧引用 .py 文件名(应只经 CLI 与产物契约)" >&2
  fail=1
fi

# 2) 前端:不得以任何形式引用 research/ 源码或产物。
if grep -rn 'research/src\|research/requirements\|\.venv' frontend/src/ 2>/dev/null; then
  echo "✗ 前端引用 research 源码/依赖(违反 G1 零共享状态)" >&2
  fail=1
fi

# 3) 契约文件名冻结面:Go 侧读的产物名必须与 research/README.md 契约表一致。
#    只做存在性校验(改了名字却没同步文档 → 报错)。
for f in run_meta.json metrics.json skeleton.md final.md lint.json synthesis.json gate.json; do
  if ! grep -q "$f" research/README.md 2>/dev/null; then
    echo "✗ 产物 $f 未登记在 research/README.md 契约表(契约面变更须同步文档)" >&2
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "✓ research 独立性检查通过:Go/前端仅经 CLI + 产物契约交互"
fi
exit "$fail"
