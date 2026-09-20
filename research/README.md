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
#    --run-id 可选:上层编排(Go cmd/research-run)传入稳定幂等键;独立 CLI 不需要。
#    ⚠️ 主体码含冒号(宏观 macro:cn_cpi)时**务必加引号** —— 现代壳里 `macro:` 一般
#    不会触发词分割,但引号能防 zsh 通配与未来扩展;Go 侧走 argv 数组,无此问题。
python3 -m src.cli research <主体码> [--days N] [--profile complete-stock|company|stock|short-term|prebuy|industry|macro] [--out-dir DIR] [--run-id ID]

# 2. 把 LLM 的三段定性渲染进报告 + Number Lint(数字一致性机检)
#    --prior-metrics 可选:既往研报的 metrics JSON(数组,Go 侧落成 {code}_prior_metrics.json),
#    其中的数字并入 lint 的 known 集 —— 让「较上次 +12%」这类对比数字不被误判为编造。
python3 -m src.cli synthesize <产物目录> <代码> --synthesis-file <LLM输出.json> [--prior-metrics <历史metrics.json>]

# 3. 六项 Quality Gate 机检
python3 -m src.cli gate <产物目录> <代码> [--json]
```

> **档案（profiles，7 个）**：`complete-stock`（full 11 节含 industry，最重；兼容保留）·
> `company`（full 8 节，公司研报，P9-4）· `stock`（express 7 节，个股分析，P9-4）·
> `short-term`（express 7 节，仅 2 维评分不含 risk）· `prebuy`（express，去 industry 保留全 6 维评分卡，P7）·
> `industry`（申万行业，P9-3）· `macro`（宏观四维，P9-5）。⚠️ **行为由 `sections` 驱动，`mode` 字段不参与分支。**

> **退出码**:`synthesize` 在 Number Lint 有 issue 时退 2;`gate` 机检未过退 3。
> 两者都**已把结果写进产物文件**——非零表示"结果未通过机检",不是"执行失败";
> Go 侧据此区分处理(§4.4 机检失败处置:markdown 回落骨架报告,lint/gate 原样落库)。

## 产物契约(冻结,`run_meta.json` 是幂等键与契约版本来源)

| 文件 | 产出者 | 内容 |
|---|---|---|
| `run_meta.json` | research | `run_id` / `symbol` / `profile` / `mode` / `as_of` / `sections` / `provider_calls` / **`contract`** |
| `{code}_metrics.json` | research | 指标卡(**数字唯一源 = Fact**)含 Evidence 链;`meta.section_manifest` = 章节清单 `[{title,domains[]}]`(前端目录/三域标签的数据源,D-R8) |
| `{code}_skeleton.md` | research | 骨架报告(模板槽位,数字已渲染) |
| `{code}_synthesis_prompt.txt` | research | 给 LLM 的合成提示(含"只能引用指标卡数字"硬约束) |
| `{code}_final.md` | synthesize | 渲染了三段定性的最终报告 |
| `{code}_lint.json` | synthesize | Number Lint 结果 `{scanned,matched,ignored,passed,issues[]}` |
| `{code}_synthesis.json` | synthesize | LLM 三段定性 `{summary,trend,conclusion}`(= Opinion) |
| `{code}_prior_metrics.json` | Go 编排 | 既往 done 研报的 metrics 数组(Go 写、synthesize 读);仅供 `--prior-metrics` 取 known 数,非 research 产出 |
| `{code}_gate.json` | gate | 六项机检结果 |

> **契约变更规则**:新增字段/新 section **不升** `contract`(Go 忽略未知键);
> **改名/删除/改语义必须升** `contract`,否则 Go 侧会误读。

> **`{code}` 可以是主体码**(P9 起):公司=`600519`,行业=`sw801010`,
> 宏观=`macro:cn_cpi`(P9-5 / #13)。故文件名可能含**冒号**;Linux 下
> 读写/建目录均正常(已实测 Go `os.WriteFile`/`MkdirAll` 与 Python
> `open`/`listdir`)。Go 侧用 `exec.Command` 传 argv 数组(**不经 shell**),
> 无引号注入之虞 —— 改 runner 时**不得**改成 `sh -c`。

> **`as_of` 语义逐主体不同**(D-M3):它是**报告生成日**,与数据边界刻意分开。
>   个股/行业:数据边界在 `meta`/行情指标卡(`price.period_days`)。
>   宏观:数据边界在 `macro.period` —— `period_end`(统计期末)+ `source_lag_days`
>     (生成日 − 期末,如实暴露滞后)。⚠️ **不写 `released_at`**:宏观数据源只有
>     统计期、**没有发布日期**,写发布日期即编造。前端封面据此区分
>     「数据截止」与「报告生成时间」两行。

## 独立迭代

```bash
pytest tests/                                              # 独立测试
docker build --target research -t piks-research:latest ..  # 只跑 Python 阶段(不重建 gateway/web)
docker run --rm piks-research ./bin/research-run 000560 --profile short-term  # 冒烟(CLI 旁路入口)
```

> ⚠️ 2026-09-20 起 `piks-research` 的 `CMD` 是常驻队列 worker(`/app/bin/research-worker`),
> 不再是 CLI 编排入口 —— 故冒烟须显式给 `./bin/research-run`。
> 另:`research` target 仍由共享的 Go 编译阶段派生(worker/research-run 是 Go 二进制),
> 所以改本目录会重跑 `go build`,但**不重跑 node**(前端阶段被绕开),`gateway`/`web` 镜像不动。

## 边界(明确不做)

- 不写数据库:归档交 `cmd/research-run`(PG)。本模块只产出文件产物。
- 不调 LLM:LLM 由 PIKS `ai.Provider` 提供,`synthesize` 从文件/stdin 读其输出。
- 数据源扩展计划见 `docs/plan-data-source-integration.md`(P0/P1/P2 任务卡)。
