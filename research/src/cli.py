#!/usr/bin/env python3
"""Investment Research Agent CLI
研究一只股票并在终端输出报告。

用法：
    python -m src.cli 600519
    python -m src.cli 600519 --days 60
    python -m src.cli 600519 --profile short-term
"""
import argparse
import json
import sys
from datetime import date
from pathlib import Path

from .workflow import load_profile, PlanGenerator, WorkflowEngine
from .report.synthesis import (
    build_synthesis_prompt,
    parse_synthesis,
    render_synthesis,
    validate_synthesis,
)
from .analysis.number_lint import lint_text, collect_numbers_from_json

# 产物契约版本(§4.10 G2):Go 侧读 run_meta.json 校验,高于其支持版本即如实失败。
# 变更规则:新增字段/新 section 不升版本;改名/删除/改语义必须升。
CONTRACT_VERSION = 1


def _save_artifacts(out_dir: str, symbol: str, result, profile_name: str, profile_mode: str = "") -> Path:
    """保存研究产物（骨架报告 + 指标卡 + 合成提示）到目录"""
    root = Path(out_dir)
    root.mkdir(parents=True, exist_ok=True)

    as_of = ""
    if isinstance(result.json_report, dict):
        as_of = (result.json_report.get("meta") or {}).get("as_of", "") or ""

    md_path = root / f"{symbol}_skeleton.md"
    json_path = root / f"{symbol}_metrics.json"
    prompt_path = root / f"{symbol}_synthesis_prompt.txt"
    meta_path = root / "run_meta.json"

    md_path.write_text(result.markdown_report or "", encoding="utf-8")
    json_path.write_text(
        json.dumps(result.json_report or {}, ensure_ascii=False, indent=2, default=str),
        encoding="utf-8",
    )
    # run_meta.json:上层编排(Go cmd/research-run)读此文件做幂等键与契约校验。
    # contract 为产物契约版本:只做加法(新增字段不升版本),改名/删字段/改语义必须升。
    meta_path.write_text(
        json.dumps(
            {
                "run_id": result.run_id or "",
                "symbol": symbol,
                "profile": profile_name,
                "mode": profile_mode,
                "as_of": as_of,
                "sections": list(result.sections or []),
                "provider_calls": dict(result.provider_calls or {}),
                "contract": CONTRACT_VERSION,
            },
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )

    if result.json_report:
        prompt = build_synthesis_prompt(symbol, result.json_report)
        prompt_path.write_text(prompt, encoding="utf-8")
    else:
        prompt_path.write_text("指标卡不可得，无法生成合成提示\n", encoding="utf-8")

    return root


def run_research(args) -> None:
    """主研究流程"""
    # 1. 加载 Profile
    try:
        profile = load_profile(args.profile)
    except Exception as e:
        print(f"❌ 加载 Profile 失败: {e}")
        sys.exit(1)

    print(f"🎯 开始研究: {args.symbol}")
    print(f"📋 Profile: {profile.name} ({profile.mode})")

    # 用户可覆盖 display 窗口中的 price days
    if args.days is not None and "price" in profile.period.display:
        profile.period.display["price"] = f"{args.days}d"
        profile.period.display["volume"] = f"{args.days}d"
        profile.period.display["turnover"] = f"{args.days}d"

    # 2. 生成 Plan
    plan = PlanGenerator.generate(profile)
    print(f"📋 生成执行计划: {len(plan.tasks)} 个任务")
    for t in plan.tasks:
        opt = " (可选)" if t.optional else ""
        print(f"   - {t.name}: {t.task_type.value}{opt}")

    # 3. 执行 Workflow
    print("\n🔧 执行工作流...")
    engine = WorkflowEngine(
        symbol_code=args.symbol,
        profile=profile,
        plan=plan,
        as_of=date.today(),
        skip_capital=args.no_lhb,
        run_id=getattr(args, "run_id", None),
    )

    try:
        result = engine.run()
    except Exception as e:
        print(f"❌ 工作流执行失败: {e}")
        sys.exit(1)

    # 4. 输出结果
    if result.errors:
        print("\n⚠️ 部分任务失败:")
        for name, err in result.errors.items():
            print(f"   - {name}: {err}")

    if result.evidence_store:
        print(f"\n📊 生成 {len(result.evidence_store.entries)} 条 Evidence")

    if result.lint_report:
        print(f"\n🔍 {result.lint_report.summary()}")
        for issue in result.lint_report.issues:
            print(f"   {issue}")

    if result.markdown_report:
        print("\n" + "=" * 60)
        print(result.markdown_report)
        print("=" * 60)

    if args.json and result.json_report:
        print("\n--- JSON ---")
        print(json.dumps(result.json_report, ensure_ascii=False, indent=2, default=str))

    # 5. 保存研究产物（供下一步 AI Synthesis 使用）
    if args.out_dir:
        root = _save_artifacts(args.out_dir, args.symbol, result, args.profile, profile.mode)
        print(f"\n💾 研究产物已保存到 {root}/")  
        for p in sorted(root.iterdir()):
            print(f"   - {p.name}")

    print("\n✅ 完成")


def run_synthesize(args) -> None:
    """AI 综合研判：把 LLM 定性段落渲染进报告 + Number Lint"""
    artifact_dir = Path(args.dir)
    prompt_path = artifact_dir / f"{args.symbol}_synthesis_prompt.txt"
    metrics_path = artifact_dir / f"{args.symbol}_metrics.json"
    skeleton_path = artifact_dir / f"{args.symbol}_skeleton.md"

    for p in (prompt_path, metrics_path, skeleton_path):
        if not p.exists():
            print(f"❌ 缺少产物文件: {p}")
            sys.exit(1)

    # 读取 LLM 输出（stdin 或文件，JSON 格式）
    if args.synthesis_file:
        text = Path(args.synthesis_file).read_text(encoding="utf-8")
    else:
        text = sys.stdin.read()

    blocks = parse_synthesis(text)
    errors = validate_synthesis(blocks, {})
    if errors:
        print("❌ 合成内容校验失败:")
        for slot, err in errors.items():
            print(f"   - {slot}: {err}")
        sys.exit(1)

    # 渲染最终报告
    skeleton = skeleton_path.read_text(encoding="utf-8")
    final_md = render_synthesis(skeleton, blocks, include_section=True)

    # Number Lint 事后扫描 LLM 文本
    metrics = json.loads(metrics_path.read_text(encoding="utf-8"))
    known = collect_numbers_from_json(metrics)
    lint = lint_text(final_md, known_values=known)
    print(f"\n🔍 {lint.summary()}")
    for issue in lint.issues:
        print(f"   {issue}")

    # 归档交上层(Go cmd/research-run 写 PG);此处只落文件产物。
    out_path = artifact_dir / f"{args.symbol}_final.md"
    out_path.write_text(final_md, encoding="utf-8")
    lint_path = artifact_dir / f"{args.symbol}_lint.json"
    lint_path.write_text(
        json.dumps(
            {
                "scanned": lint.scanned,
                "matched": lint.matched,
                "ignored": lint.ignored,
                "passed": lint.passed,
                "issues": [
                    {
                        "number": i.number,
                        "value": i.value,
                        "line_no": i.line_no,
                        "context": i.context,
                        "reason": i.reason,
                        "severity": i.severity,
                    }
                    for i in lint.issues
                ],
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )
    # synthesis 段落也落盘(Go 侧读它存 Opinion 分域)
    synth_path = artifact_dir / f"{args.symbol}_synthesis.json"
    synth_path.write_text(
        json.dumps(blocks, ensure_ascii=False, indent=2), encoding="utf-8"
    )

    print(f"\n💾 最终报告已保存: {out_path}")
    print(f"💾 机检结果已保存: {lint_path}")
    print("\n" + "=" * 60)
    print(final_md)
    print("=" * 60)

    # Lint 未通过时以非零码退出（供上层感知）
    if lint.issues:
        sys.exit(2)


def run_gate(args) -> None:
    """M6 Quality Gate：对产物目录机检六项规则(不再依赖 SQLite 归档)"""
    from .quality_gate import run_quality_gate, fingerprint_report

    artifact_dir = Path(args.dir)
    symbol = args.symbol
    metrics_path = artifact_dir / f"{symbol}_metrics.json"
    meta_path = artifact_dir / "run_meta.json"
    lint_path = artifact_dir / f"{symbol}_lint.json"

    if not metrics_path.exists():
        print(f"❌ 缺少产物文件: {metrics_path}")
        sys.exit(1)

    meta = {}
    if meta_path.exists():
        try:
            meta = json.loads(meta_path.read_text(encoding="utf-8"))
        except Exception:
            meta = {}

    print(f"🏁 M6 Quality Gate: {meta.get('run_id') or symbol}")
    print(f"   symbol={meta.get('symbol') or symbol} profile={meta.get('profile', '')}")

    sections = list(meta.get("sections") or [])
    json_report = json.loads(metrics_path.read_text(encoding="utf-8"))
    evidence = json_report.get("evidence") if isinstance(json_report, dict) else None

    # Lint 结果(来自 synthesize 产出的 {symbol}_lint.json;缺则视为通过)
    lint_passed = True
    lint_issues = 0
    if lint_path.exists():
        try:
            lint = json.loads(lint_path.read_text(encoding="utf-8"))
            lint_passed = bool(lint.get("passed", True))
            lint_issues = len(lint.get("issues") or [])
        except Exception:
            pass

    # 指纹重算（同数据同代码重跑一致性）
    stored_fp = meta.get("report_fingerprint")
    computed_fp = fingerprint_report(json_report) if json_report else None

    gate = run_quality_gate(
        profile_sections=sections,
        json_report=json_report,
        evidence=evidence,
        lint_passed=lint_passed,
        lint_issue_count=lint_issues,
        stored_fingerprint=stored_fp,
        computed_fingerprint=computed_fp,
    )

    print(f"\n{gate.summary()}")
    for c in gate.checks:
        mark = "✅" if c.passed else "❌"
        print(f"  {mark} {c.name:24s} | {c.detail}")

    if gate.issues:
        print("\n问题清单:")
        for iss in gate.issues:
            print(f"   - {iss}")

    # 机检结果落产物目录(Go 侧读取入库)
    gate_path = artifact_dir / f"{symbol}_gate.json"
    gate_path.write_text(
        json.dumps(gate.to_dict(), ensure_ascii=False, indent=2), encoding="utf-8"
    )
    print(f"\n💾 机检结果已保存: {gate_path}")

    if args.json:
        print("\n--- Gate JSON ---")
        print(json.dumps(gate.to_dict(), ensure_ascii=False, indent=2))

    if not gate.passed:
        sys.exit(3)


def main():
    parser = argparse.ArgumentParser(description="个股研究 Agent")
    sub = parser.add_subparsers(dest="command")

    # 研究命令
    p_research = sub.add_parser("research", help="研究一只股票")
    p_research.add_argument("symbol", help="股票代码，如 600519")
    p_research.add_argument("--days", type=int, default=None, help="研究周期（交易日），默认由 Profile 决定")
    p_research.add_argument("--profile", type=str, default="complete-stock", help="研究 Profile")
    p_research.add_argument("--json", action="store_true", help="同时输出 JSON 结构化数据")
    p_research.add_argument("--no-lhb", action="store_true", help="跳过龙虎榜采集（加快速度）")
    p_research.add_argument("--out-dir", type=str, default=None, help="保存研究产物（骨架报告+指标卡+合成提示）")
    # 上层编排(Go cmd/research-run)传入稳定 run_id 作幂等键;独立 CLI 不需要。
    p_research.add_argument("--run-id", type=str, default=None, help="覆盖 run_id（上层编排幂等键）")

    # 综合研判命令
    p_synth = sub.add_parser("synthesize", help="AI 综合研判：渲染 LLM 定性段落进报告")
    p_synth.add_argument("dir", help="研究产物目录")
    p_synth.add_argument("symbol", help="股票代码")
    p_synth.add_argument("--synthesis-file", type=str, default=None, help="LLM 输出的 JSON 文件（默认从 stdin 读）")

    # M6 Quality Gate 命令
    p_gate = sub.add_parser("gate", help="M6 Quality Gate：对产物目录进行机器可判定的六项机检")
    p_gate.add_argument("dir", help="研究产物目录（含 {symbol}_metrics.json / run_meta.json / {symbol}_lint.json）")
    p_gate.add_argument("symbol", help="股票代码")
    p_gate.add_argument("--json", action="store_true", help="输出机检结果 JSON")

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        print("\n用法：python -m src.cli research <代码> | synthesize <产物目录> <代码> | gate <产物目录> <代码>")
        sys.exit(1)

    if args.command == "synthesize":
        run_synthesize(args)
    elif args.command == "gate":
        run_gate(args)
    else:
        run_research(args)


if __name__ == "__main__":
    main()
