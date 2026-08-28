# 周报 AI 综述(Web 适配 iter4 D26)归档

> 日期:2026-08-28。范围:**仅 dev 环境验证,不部署 lab**——代码改动仅随未来 lab 镜像重建生效,本次未部署、prod DB 无新迁移、生产行为不变。
> 设计:`docs/phase2/design/weekly-ai-summary.md`(已定稿冻结,2026-08-28 用户确认)。前置:迭代 5 Web 平台 + iter4 D26 综述设计(原 `cmd/weekly-report` 已下线,本项为 Web 形态落地)。

## 交付内容

| 文件 | 改动 |
|---|---|
| `migrations/0009_weekly_summaries.sql` | 新增 `weekly_summaries` 表(week UNIQUE 防同周重复行,UPSERT 覆盖不新增) |
| `internal/store/weekly_summaries.go` | 新增 `WeeklySummary` 模型 + `GetWeeklySummary`(无缓存返回 nil)+ `UpsertWeeklySummary`(ON CONFLICT 覆盖) |
| `internal/web/weekly.go` | 新增 POST `action=generate` 分流;聚合逻辑抽为 `aggregateWeek`(页面渲染与综述上下文同源);新增 `generateWeeklySummary`(模型回退 + 预算护栏 + 记账 + 入库)+ `buildWeekContext` + `genNote`(g= 状态如实提示) |
| `internal/web/templates/weekly.html` | 「本周沉淀」后新增「AI 综述」section:有缓存 → 综述卡(正文 + 模型/tokens/时间 + 重新生成);无缓存 → 空态占位 + 生成按钮 |
| `docs/phase2/design/weekly-ai-summary.md` | 设计定稿(缓存表 + 手动触发 + 预算护栏 + prompt 护栏 + 降级矩阵) |

`server.go` 路由不变(`/weekly` 已 `HandleFunc` 支持 GET/POST)。复用:`ai.StructuredOutput`(temp0)、`ListAppConfig`、`TokensSince`、`StartTaskRun/FinishTaskRun`、`weekRange`。

> 附:本次 `go run ./cmd/migrate` 一并补齐了此前已建未落地的 `0007_ai_model_vision`、`0008_chat`(dev 此前的迁移记录只到 0006),0009 为本次新增;全部按序应用成功。

## 验收证据(dev,真实数据 + 真实 Zen 调用)

环境:dev DB(app_config:base=`https://opencode.ai/zen/go/v1`,key 已配,`ai_model_reasoning=deepseek-v4-flash`,budget=0 不限)。

**1. 生成(真实 LLM 调用)**:

```
POST /weekly action=generate → 303 /weekly?offset=0&g=ok
```

入库行 `2026-W35`:model=`deepseek-v4-flash`,tokens=563,正文 224 字。内容严格基于当周数据(实测节选):「本周仅8月26日一个交易日,市场情绪为Strong,涨停52家、跌停0家,成交额与个人判断暂无数据。当日事件密集:央行宣布降准0.25个百分点…宁德时代与星河新能源…另有两起异常事件(甲、乙)…个人笔记仅针对异常事件甲有所沉淀…」——无一处超出已列数据(降准/星河/宁德/异常甲乙均出自事件表,8/26 数据出自快照表)。

**2. 缓存(GET 零 LLM)**:生成后刷新 GET `/weekly` → 直接展示综述卡(标注模型 + 生成时间),task_runs `weekly-summary` 计数保持 1 不变。

**3. 重新生成(覆盖不新增行)**:再 POST → `g=ok`,`updated_at` 05:21:16 → 05:21:40,`weekly_summaries` 仍 1 行(week UNIQUE)。

**4. 预算耗尽(如实跳过)**:临时 `ai_daily_token_budget=100`(当日已用 ≥100)→ POST → `g=budget`,GET 提示「今日 AI 预算已用尽,综述暂缺(预算恢复后重试)」,task_runs 计数保持 2 不变(零调用)。测试后 budget 复原 0。

**5. 未配置(如实降级)**:临时清空 `ai_model_reasoning`+`ai_model_extract` → POST → `g=noconfig`,GET 提示「AI 未配置(请到 /settings 填写服务地址与密钥),综述暂缺」。测试后模型复原 `deepseek-v4-flash`。

**6. 空态诚实 + nodata**:无缓存 → 空态占位「本周暂无 AI 综述。」+「生成 AI 综述」按钮,GET 不自动生成;空周 W34(`/weekly?offset=1`,无行情/事件/沉淀)→ POST → `g=nodata`,提示「本周暂无行情/事件/沉淀数据,暂无可综述。」。

**7. 记账**:task_runs 行 `command='weekly-summary'`,status=success,meta `{"week":"2026-W35","model":"deepseek-v4-flash","ai_tokens":563}`(日账本全局一致,与 cmd/* 共用)。

## 验收清单

- [x] 生成:有 AI 配置时点「生成 AI 综述」→ 综述入库并展示(模型 + 时间);内容严格只总结当周行情/事件/沉淀(实测零新增事实)
- [x] 缓存:生成后刷新 /weekly → 直接展示缓存,不触发 LLM(GET 零调用,task_runs 计数不变)
- [x] 重新生成:按钮 → 覆盖更新(updated_at 刷新),不新增行(week UNIQUE,实测仍 1 行)
- [x] 未配置 AI:清空 model 配置 → 点击后如实提示「AI 未配置」,不调用不编造
- [x] 预算耗尽:临时设预算=当日已用 → 点击后如实提示预算已尽,不调用
- [x] 空态诚实:无缓存 → 占位 + 按钮,不自动生成
- [x] nodata:空周生成 → 如实提示「本周暂无数据」,不入库不报错
- [x] task_runs 记账:生成后 command='weekly-summary' 行含 ai_tokens
- [x] 生产未动:未部署 lab、prod 无新迁移/无新代码
- [x] 数据诚实:页面标注如实反映(模型/时间/失败/降级);`go build`/`go vet`/`go test ./...` 全过

## 已知权衡与说明(数据诚实)

1. **手动按钮触发,不自动生成**:GET 永不调 LLM(缓存命中零成本);自动生成(定时/跨周)列为后续可选,避免被动烧预算。
2. **模型回退链**:`ai_model_reasoning`(高智档)→ 未配置回退 `ai_model_extract` → 两者都空则「AI 未配置」;dev 上两者同为 flash,lab 可换深档(reasoning)提质。
3. **综述内容不回溯检索**:仅基于页面同源聚合数据,不跨知识库检索/引用(与 merged-grounding 无关,独立话题)。
4. **无历史综述列表/跨周对比**:本期仅单周综述卡 + 覆盖式重新生成;批量/历史/对比列为后续可选。
5. **nodata 与 budget 单测**:web 层无现成单测(web 测试未建),验收以 dev 实测为准;budget/noconfig 均用真实配置临时变更驱动,验证后已复原。

## 遗留/后续(不做)

- 综述内容进知识库检索 / 可引用标注(独立于 merged-grounding,进度总表已知遗留范畴)。
- 自动触发生成(定时/换周首访)、历史综述列表、跨周对比(可选增强)。
- 多模型对照生成(A/B 出稿),后续可选。
