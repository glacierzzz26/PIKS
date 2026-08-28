# 交易闭环:持仓 AI 诊断 + 交易进周报

> 状态:待定稿。范围:**仅 dev 验证,不部署 lab**(与交易功能一致,改动随未来 lab 镜像重建生效)。
> 前置:`docs/phase2/design/trades.md`(已定稿,migration 0010 `trades`+`positions` 已落地)。本设计把刚建好的交易数据接进既有闭环:**持仓区从"只展示"升级为"AI 诊断"**,**周报综述上下文纳入本周交易/持仓**。

## 1. 背景与现状

- `/trades` 已支持截图/手动录入交易与持仓,`positions` 落库但**只存只展示**(D-T7),持仓区无任何分析。
- 交易解读管线 `tradeReview` 已具备完整可复用基建:expandQuery(同义扩展)+ `SearchKnowledgeExpanded`(事件/实体)+ `ListPersonalNotesByText`(笔记)+ 防未来函数(`evWithin` 截断到 trade_date)+ 引用白名单校验(`validateTradeReview`)+ 预算护栏 + task_runs 记账。
- 周报综述 `generateWeeklySummary` 上下文仅含行情/事件/沉淀,`aggregateWeek` 三路聚合;nodata 判定为三类全空。
- 用户每日流程:截图录入交易 → AI 解读单笔;每周:看周报 + AI 综述。**缺口:持仓无组合视角解读;周报综述不知道我本周买了什么**。

## 2. 方案

### 2.1 持仓 AI 诊断(/trades 持仓区)

**新表 `position_reviews`(migration 0011)**:按 `snapshot_date` 缓存诊断结果(一天一份,与 weekly_summaries 按 week 缓存同模式)。

```sql
CREATE TABLE IF NOT EXISTS position_reviews (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  snapshot_date DATE NOT NULL UNIQUE,
  review JSONB NOT NULL DEFAULT '{}'::jsonb,   -- 契约见 2.1.2
  model TEXT, tokens INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_position_reviews_date ON position_reviews(snapshot_date DESC);
```

**交互**:持仓快照区(展示最近快照)下方:
- 已诊断 → 展示诊断卡(正文 + 模型/tokens/时间 + 引用 chips 可点 + risks「存为笔记」+ 重新诊断按钮);
- 未诊断 → 空态占位 + 「AI 诊断」按钮(POST `/trades/positions/review`)。
- GET 永不调 LLM(缓存命中零成本)。新快照上传(snapshot_date 变化)或点按钮才重生成。

**诊断数据聚合(Go 计算,LLM 不编造数字)**:
1. 组合整体:持仓股数、总市值、总盈亏、盈亏比(合计 pl / 合计市值)、盈亏股票数分布(盈利 N 只/亏损 M 只)。
2. 仓位集中度:按市值占比降序,前 1/前 3 大持仓占比;单一持仓最大占比。
3. 与近期交易联动:近 14 天 `trades`(买入/卖出)中涉及当前持仓股的笔数;对每只被交易的持仓股,标注「近 14 天买/卖 + 现盈亏」(如:贵州茅台 08-28 买入 → 现 +0.35%)。
4. 知识库 grounding:取持仓股名集合 → `expandQuery` 同义扩展 → `SearchKnowledgeExpanded`(事件/实体)+ 每只持仓 `ListPersonalNotesByText`;防未来函数截断到 snapshot_date(诊断只含快照时点及之前语境)。

这些聚合数字全部由 Go 从 `positions`/`trades` 表算出并拼进 LLM 上下文,LLM 只负责解读(风险点、与笔记信念的印证/违背),禁止输出买卖建议或预测涨跌。

**诊断 JSONB 契约**(`position_reviews.review`):
```json
{
  "review": "...",        // ≤300 字中文诊断正文
  "refs": {"events": [{"id","title"}], "entities": [{"id","title"}], "notes": [{"id","title"}]},
  "risks": [{"title": "...", "content": "..."}],   // 可复盘点候选,空数组如实
  "model": "...", "tokens": 123, "generated_at": "..."
}
```
引用 id 必须来自 LLM 上下文标注(E:/N:/P: 前缀),`validateTradeReview` 复用(白名单过滤,防自造)。

**risks → 存为笔记**:POST `/trades/positions/save-risk/{n}` → `personal_notes(type='mistake', slug='posrev-{snapshot_date}-{n}', status='hypothesis')`,复用 `GetPersonalNoteBySlug` 幂等去重。iter4 单向 harvest:AI 提议、用户确认。

### 2.2 交易进周报(/weekly)

**aggregateWeek 扩展为四路聚合**(页面渲染与综述上下文同源):
- 新增 `本周交易`:`ListTradesBetween(ctx, start, end)`(trade_date 在周内)→ 每笔:日期/名称/代码/买卖/价格/数量/金额。
- 新增 `本周持仓(快照)`:`LatestPositionsBefore(ctx, end)`(最近 snapshot_date ≤ 周最后一天)→ 每只:代码/名称/数量/成本价/现价/市值/盈亏。
- **防未来函数**:交易按 trade_date 过滤到当周;持仓取"周内或最接近周末的快照",综述 prompt 标注快照日期,LLM 不引用周内之后数据。

**buildWeekContext 增加段**:
```
== 本周交易 ==
08-26 贵州茅台(600519) 买入 100股 @1450.50 金额145050.00
== 本周持仓(截至 08-28 快照) ==
600519 贵州茅台 100股 成本1380.000 现价1450.500 市值145050.00 盈亏+7050.00
```

**system prompt 增规则**:「结合本周交易/持仓,提示值得复盘的点(如:买入是否基于当周事件、持仓集中度、与既有笔记的印证/违背)」——仍是复盘视角,禁止买卖建议。

**nodata 判定扩展**:`行情/事件/沉淀/交易/持仓` 五类全空才 `nodata`(本周只有交易无行情事件也能生成——有实际交易就有可综述)。

**weekly 页面**:新增「本周交易」「本周持仓(最近快照)」两个 section 表格(与行情/事件/沉淀同风格),综述卡下方展示。与综述上下文同源(页面先有,综述才有)。

## 3. 契约

### 3.1 store

| 函数 | 说明 |
|---|---|
| `ListTradesBetween(ctx, start, end)` | `trades` WHERE trade_date ≥ start AND < end,按 trade_date 升序(trades.go 增) |
| `LatestPositionsBefore(ctx, before time.Time)` | 最近 snapshot_date < before 的快照(同 snapshot_date 多行全返回),无则空(trades.go 增) |
| `GetPositionReview(ctx, date)` / `UpsertPositionReview(ctx, date, review, model, tokens)` | 新文件 `internal/store/position_reviews.go`,ON CONFLICT (snapshot_date) 覆盖,仿 weekly_summaries |

### 3.2 web

| 文件 | 改动 |
|---|---|
| `internal/web/trades.go` | `PositionDiagnosis` 聚合函数(组合/集中度/交易联动)+ 生成 handler(POST `/trades/positions/review`,复用 budget/expandQuery/Search/防未来/StructuredOutput/validateTradeReview)+ `saveRisk`(POST `/trades/positions/save-risk/{n}`)+ 页面数据字段 + note 状态 `g=position_reviewed`/`g=risk_saved` |
| `internal/web/templates/trades.html` | 持仓快照区下方:诊断卡(正文/模型/时间/refs chips/risks 存为笔记/重新诊断)或空态 + 按钮 |
| `internal/web/weekly.go` | `WeeklyPage` 增 `Trades []WeeklyTrade`、`Positions []WeeklyPosition`;`aggregateWeek` 四路;`buildWeekContext` 增两段;nodata 扩五类 |
| `internal/web/templates/weekly.html` | 新增「本周交易」「本周持仓(最近快照)」section |
| `internal/web/server.go` | `handleTrade` 增加 positions/review 与 save-risk 分支(路由前缀已在 `/trades/`) |

### 3.3 降级矩阵(数据诚实)

| 场景 | 表现 |
|---|---|
| 无持仓快照 | 诊断按钮不显示/提示「暂无持仓快照,先导入持仓」,不入库不调用 |
| AI 未配置(无 base/key/model) | 如实提示「AI 未配置(请到 /settings 配置)」 |
| 预算耗尽 | 如实提示「今日 AI 预算已用尽」,零调用 |
| 识别/调用失败 | 如实提示失败,不落脏数据 |
| 知识库无相关 | 诊断正文如实说明「知识库无相关事件/实体/笔记」,不编造 |

### 3.4 复用

`expandQuery`、`SearchKnowledgeExpanded`、`ListPersonalNotesByText`、`evWithin`、`validateTradeReview`、`tradeMistake`/`tradeRefs`、`StartTaskRun("position-review")`、预算护栏、`genNote` 模式、`orStr/orEmpty`。**不新增 AI 能力,不新数据源,零迁移复杂度(仅 0011)**。

## 4. 验收清单(dev-only)

- [ ] 持仓诊断:有持仓 → 点「AI 诊断」→ 生成诊断(组合盈亏/集中度/交易联动/引用可点),按 snapshot_date 缓存;GET 零调用(计数不变)
- [ ] 数字聚合在 Go:总市值/盈亏/集中度/联动笔数由代码算出进上下文,LLM 正文不出现未提供的数字
- [ ] 引用白名单:refs id 全部来自上下文;无关联如实标注
- [ ] 防未来函数:诊断不含 snapshot_date 之后事件(插未来事件实测零泄漏)
- [ ] risks 存为笔记:type=mistake, slug=posrev-*, 幂等去重
- [ ] 周报含交易:本周交易/持仓在 /weekly 页面显示,且 AI 综述上下文含之;综述能提及本周交易/持仓
- [ ] nodata 扩展:只有交易无行情/事件/沉淀的周也能生成综述
- [ ] 降级:无持仓 / AI 未配置 / 预算耗尽 → 如实提示
- [ ] 记账:task_runs `position-review` 含 ai_tokens;`weekly-summary` 不受影响
- [ ] 生产未动:未部署 lab、prod 无新迁移
- [ ] 数据诚实:`go build`/`go vet`/`go test ./...` 全过

## 5. 涉及文件

```
migrations/0011_position_reviews.sql       (新)
internal/store/position_reviews.go         (新)
internal/store/trades.go                   (增 ListTradesBetween / LatestPositionsBefore)
internal/web/trades.go                     (增诊断/存风险/页面字段)
internal/web/templates/trades.html         (持仓区诊断卡/按钮)
internal/web/weekly.go                     (四路聚合/上下文/页面字段)
internal/web/templates/weekly.html         (本周交易/持仓 section)
internal/web/server.go                     (handleTrade 分支)
docs/phase2/stages/trade-loop.md           (归档,落地后)
```

## 6. 边界(明确不做)

- **不做买卖建议/涨跌预测**:诊断与复盘都是「复盘视角 + 引用」,prompt 明令禁止。
- **不做交易收益测算/T+1 回测/买卖配对**:超出"闭环展示"范围,回测类能力单独立项(进度总表可选增强)。
- **不做持仓自动诊断/定时**:手动按钮触发(与周报一致,防被动烧预算)。
- **不做持仓数据进「本周沉淀」笔记聚合**:持仓是状态非认知,保持 D-T1 分离。
- **不做 G3/G4 数据源**:那是独立迭代(数据补全),本迭代不碰。

## 7. 决策留痕

| # | 决策 | 理由 |
|---|---|---|
| D-L1 | 诊断按 `snapshot_date` 缓存(position_reviews 表 UNIQUE) | 一天一份,与 weekly_summaries 同模式;GET 零调用防烧预算 |
| D-L2 | 数字聚合全部 Go 计算,LLM 只解读 | 数据诚实:LLM 会编数字,聚合数字是事实必须代码出 |
| D-L3 | 诊断复用 tradeReview 全管线 | expandQuery/检索/防未来/白名单/预算/记账已验证,零新 AI 能力 |
| D-L4 | risks → 存为笔记复用 harvest,slug=posrev-* | iter4 单向语义:AI 提议、用户确认;幂等 |
| D-L5 | 周报四路聚合 + nodata 扩五类 | 页面与综述同源(先有页面,综述才引用);只有交易的周也可综述 |
| D-L6 | 持仓取"周末前最近快照"并标注日期 | 防未来函数:综述/诊断不含快照时点之后数据 |
| D-L7 | 手动触发,不自动/定时 | 与周报综述一致:防被动烧预算,需要时点一下 |

## 8. 验收证据(规划)

- 用上一迭代遗留的 dev 合成数据(5 笔交易 + 3 持仓 + 相关实体)→ 点诊断 → 生成组合诊断 + 引用;插未来事件重跑 → 零泄漏;risks 存笔记幂等。
- 周报:当前周含交易/持仓 → 页面显示 + 综述上下文含之;临时造"只有交易无行情"的周 → 综述仍生成。
- 降级与记账同上迭代实测法(真实配置临时变更驱动,验证后复原)。
