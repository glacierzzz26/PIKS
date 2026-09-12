# research — 个股深研(PIKS 子模块)

> 自 `../investment-research` 并入(2026-09-12,见 `../docs/phase4/design/research-merge.md`)。
> **本目录支持独立迭代**(设计 D-11):改这里的代码只需重建 `piks-research` 镜像,Go 与前端零接触。

## 定位

输入股票代码 → 输出结构化个股深研报告(行情量价 / 财务基本面 / 新闻公告降噪 / 龙虎榜逐日席位 / 风险评分)。
**确定性计算由 Python 负责,LLM 只写定性叙述** —— 报告里所有数字都由程序算出,LLM 不触碰数值。

## 独立性契约(D-11)

| 约束 | 说明 |
|---|---|
| **零共享状态** | 本目录不依赖 `internal/` 任何代码;G1 由 CI `grep` 校验 |
| **产物契约版本化** | `run_meta.json` 的 `contract` 字段;Go 侧版本高于其支持即如实失败(G2) |
| **依赖独立** | `requirements.txt` 独立,不并入 Go/前端依赖链(G3) |
| **接口冻结** | 下列 CLI 三命令与产物文件名冻结;新增能力可加新命令,不改既有语义(G4) |
| **独立测试** | `pytest tests/` 纯 Python + provider mock,不依赖 PIKS 与数据库(G4) |

## CLI 契约(冻结)

```bash
# 1. 采集 + 确定性分析 → 骨架报告 + 指标卡 + 合成提示
python3 -m src.cli research <代码> [--days N] [--profile complete-stock|short-term] [--out-dir DIR]

# 2. 把 LLM 的三段定性渲染进报告 + Number Lint(数字一致性机检)
python3 -m src.cli synthesize <产物目录> <代码> --synthesis-file <LLM输出.json>

# 3. 六项 Quality Gate 机检
python3 -m src.cli gate <产物目录> <代码> [--json]
```

## 产物契约(冻结,`run_meta.json` 是幂等键与契约版本来源)

| 文件 | 产出者 | 内容 |
|---|---|---|
| `run_meta.json` | research | `run_id` / `symbol` / `profile` / `mode` / `as_of` / `sections` / `provider_calls` / **`contract`** |
| `{code}_metrics.json` | research | 指标卡(**数字唯一源 = Fact**)含 Evidence 链 |
| `{code}_skeleton.md` | research | 骨架报告(模板槽位,数字已渲染) |
| `{code}_synthesis_prompt.txt` | research | 给 LLM 的合成提示(含"只能引用指标卡数字"硬约束) |
| `{code}_final.md` | synthesize | 渲染了三段定性的最终报告 |
| `{code}_lint.json` | synthesize | Number Lint 结果 `{scanned,matched,ignored,passed,issues[]}` |
| `{code}_synthesis.json` | synthesize | LLM 三段定性 `{summary,trend,conclusion}`(= Opinion) |
| `{code}_gate.json` | gate | 六项机检结果 |

> **契约变更规则**:新增字段/新 section **不升** `contract`(Go 忽略未知键);
> **改名/删除/改语义必须升** `contract`,否则 Go 侧会误读。

## 独立迭代

```bash
pytest tests/                                              # 独立测试
docker build --target research -t piks-research:latest ..  # 只跑 Python 阶段
docker run --rm piks-research 000560 --profile short-term  # 冒烟
```

## 边界(明确不做)

- 不写数据库:归档交 `cmd/research-run`(PG)。本模块只产出文件产物。
- 不调 LLM:LLM 由 PIKS `ai.Provider` 提供,`synthesize` 从文件/stdin 读其输出。
- 数据源扩展计划见 `docs/plan-data-source-integration.md`(P0/P1/P2 任务卡)。
