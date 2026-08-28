# 周报 AI 综述设计文档(Web 适配 iter4 D26)

> 状态:**草案(待定稿门确认,2026-08-28)**。范围:兑现 iter4 设计 D26 可选「高智档综述」,落地到 Web `/weekly`(iter4 原设计为 `cmd/weekly-report` 写 vault,已被 5-2 下线替代)。
> **约束:只做 dev 验证,不部署 lab**——代码改动仅随未来 lab 镜像重建生效,本次未部署、prod DB 无新迁移、生产行为不变。
> 契约依据:`iter4-learning-loop.md` D26/§4(AI 成本策略)、`web-app.md` §4(周报页)、`internal/web/weekly.go`(现状聚合)、`internal/ai/ai.go`(`StructuredOutput`)。

---

## 1. 背景与现状

`/weekly` 现为**纯规则聚合页**(每请求重算,零持久化、零 AI):
- 本周行情快照(`ListMarketSnapshots` 过滤)+ 本周事件(`ListEventsBetween`)+ 本周沉淀(`ListPersonalNotesBetween`)。
- 页面三段:本周行情 / 本周事件 / 本周沉淀。**无 AI 综述段**。

iter4 D26 已设计综述能力,但绑在已下线的 `cmd/weekly-report`(写 vault 文件)上。Web 形态需要:综述**每周一次、高智档(贵)** → 必须持久化缓存,不能每次 GET 都调 LLM。

## 2. 方案:缓存表 + 手动触发生成

### 2.1 新表 `weekly_summaries`(migration 0009)

```sql
CREATE TABLE weekly_summaries (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  week       TEXT NOT NULL UNIQUE,          -- ISO 周标签,如 2026-W35
  summary    TEXT NOT NULL,                 -- 综述正文(高智档生成)
  model      TEXT NOT NULL,                 -- 生成所用模型(如实标注)
  tokens     BIGINT NOT NULL DEFAULT 0,     -- 本次生成 token(task_runs 同记,双份留痕)
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- **week UNIQUE** 防同周重复行;重新生成 = UPSERT 覆盖(updated_at 刷新)。
- 零关系依赖,独立小表。

### 2.2 页面与触发

- `/weekly` 聚合不变;「本周沉淀」后新增「AI 综述」section:
  - **有缓存** → 直接展示综述(标注模型 + 生成时间 + 「重新生成」按钮)。
  - **无缓存** → 空态占位「本周暂无 AI 综述」+ 「生成 AI 综述」按钮(诚实,不自动生成)。
- 生成 = **POST `/weekly`(action=generate)** → 聚合本周数据 → 调高智档 → 入库 → 302 回 `/weekly`。**GET 永不触发 LLM**(缓存命中零成本,与幂等/防 churn 一致)。

### 2.3 生成管线

1. 读 app_config(每请求重读):base/key/model。
2. **模型**:`ai_model_reasoning`(高智档,iter0 D2);未配置 → 回退 `ai_model_extract`;两者都空 → 如实提示「AI 未配置」,不调用。
3. **预算护栏**(iter4 §4):读 `ai_daily_token_budget`;今日已用(`TokensSince` 本地午夜)≥ 预算 → 如实提示「今日 AI 预算已用尽,综述暂缺」,不调用;否则生成后记 `task_runs(command='weekly-summary', ai_tokens)` 使日账本全局一致。
4. **调用**:`StructuredOutput`(temperature 0,JSON mode)要求输出 `{"summary":"..."}`。
   - **prompt 护栏**(D26):严格只总结上方已列出的数据(行情表 + 事件 + 沉淀),禁止新增事实/推断;≤300 字;中文。
   - 输入 = 与页面同源的聚合上下文(紧凑拼接,不含脆弱长文本)。
5. **解析失败/空** → 如实失败(页面标注),不入库。
6. **入库**:`UpsertWeeklySummary`(同周覆盖)。

### 2.4 失败与降级(数据诚实)

| 情形 | 行为 |
|---|---|
| 未配置 AI | 按钮点击后页面提示「AI 未配置,综述暂缺」,不调用不编造 |
| 预算已用尽 | 提示「今日 AI 预算已用尽,综述暂缺」,不调用 |
| LLM 调用失败 | 提示「综述生成失败(暂缺)」,不入库 |
| 解析为空/非 JSON | 同上 |

## 3. 契约

### 3.1 store(`internal/store/weekly_summaries.go`)

```go
type WeeklySummary struct {
    ID string `db:"id"`; Week string `db:"week"`; Summary string `db:"summary"`
    Model string `db:"model"`; Tokens int64 `db:"tokens"`
    CreatedAt time.Time `db:"created_at"`; UpdatedAt time.Time `db:"updated_at"`
}

// GetWeeklySummary 按周取综述;无缓存返回 nil。
func (s *Store) GetWeeklySummary(ctx context.Context, week string) (*WeeklySummary, error)

// UpsertWeeklySummary 写/覆盖某周综述。
func (s *Store) UpsertWeeklySummary(ctx context.Context, week, summary, model string, tokens int64) error
```

### 3.2 web(`internal/web/weekly.go` + `templates/weekly.html` + `server.go` 路由不变)

```go
// WeeklyPage 增:
Summary    *WeeklySummary  // 非空=展示;nil=空态
SummaryNote string         // 降级/未配置/预算提示(如实)
SummaryModel string        // 展示用(model + 时间)

// handleWeekly:
//   POST action=generate → generateWeeklySummary(ctx, week) → redirect(302)
//   GET → 聚合 + GetWeeklySummary(缓存) → render

// generateWeeklySummary:
//   cfgMap := store.ListAppConfig → 预算检查 → 聚合上下文 → ai.NewOpenAICompat(base,key,reasoningModel)
//     .StructuredOutput(system 护栏, user=聚合上下文) → 解析 {"summary"} → StartTaskRun/FinishTaskRun 记账
//     → UpsertWeeklySummary → 返回
```

## 4. 验收清单(dev-only)

- [ ] 生成:有 AI 配置时点「生成 AI 综述」→ 综述入库并展示(标注模型 + 时间);内容严格只总结当周行情/事件/沉淀
- [ ] 缓存:生成后刷新 /weekly → 直接展示缓存,不触发 LLM(GET 零调用)
- [ ] 重新生成:按钮 → 覆盖更新(updated_at 刷新),不新增行(week UNIQUE)
- [ ] 未配置 AI:清空 model 配置 → 点击后如实提示「AI 未配置」,不调用不编造
- [ ] 预算耗尽:临时设预算=今日已用 → 点击后如实提示预算已尽,不调用
- [ ] 空态诚实:无缓存 → 占位 + 按钮,不自动生成
- [ ] task_runs 记账:生成后 command='weekly-summary' 行含 ai_tokens
- [ ] 生产未动:未部署 lab、prod 无新迁移/无新代码
- [ ] 数据诚实:页面标注如实反映(模型/时间/失败/降级)

## 5. 涉及文件

- 新增:`migrations/0009_weekly_summaries.sql`、`internal/store/weekly_summaries.go`。
- 修改:`internal/web/weekly.go`(POST 生成 + 综述段数据)、`internal/web/templates/weekly.html`(综述 section)、`internal/web/server.go`(路由 `GET/POST /weekly` 分流,现有 `HandleFunc` 已支持 POST)。
- 复用:`ai.StructuredOutput`(temp0)、`ListAppConfig`、`TokensSince`、`StartTaskRun/FinishTaskRun`、`weekRange`。
- 测试:`internal/web` 无现成测试(web 测试未建),验收以 dev 实测 + `go vet`/`go test ./...` 全过为准。

## 6. 边界(明确不做)

- ❌ 不做自动生成(手动按钮,避免 GET 慢调用与预算意外消耗)。
- ❌ 不做多周批量生成/历史综述列表/跨周对比(后续可选)。
- ❌ 不做综述内容进知识库检索(独立话题,勿与 merged-grounding 混)。
- ❌ 不做生产部署:不重建 lab 镜像、不触发 prod 管线。

## 7. 决策留痕

| # | 决策 | 理由 |
|---|---|---|
| D-W1 | 新表 `weekly_summaries` 而非塞 personal_notes/app_config | personal_notes 是用户沉淀,塞生成综述会污染「本周沉淀」;app_config 是 KV 配置 |
| D-W2 | 手动按钮触发而非 GET 自动生成 | GET 触发 LLM = 每次看页都烧预算 + 慢;手动显式、可控、诚实 |
| D-W3 | 高智档走 `ai_model_reasoning`(未配置回退 extract) | iter0 D2 周报/深度复盘 → 高智档;dev 上 reasoning=flash 可跑通,lab 可换深档 |
| D-W4 | 预算沿用 AIDailyTokenBudget + task_runs 记 token | iter4 §4 明确;与 cmd/* 命令共用日账本,全局护栏一致 |
| D-W5 | 覆盖式 UPSERT(week UNIQUE) | 重新生成不产生脏数据;updated_at 留痕 |
