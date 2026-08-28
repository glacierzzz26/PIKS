# 交易闭环(持仓 AI 诊断 + 交易进周报)归档

> 日期:2026-08-28。范围:**仅 dev 环境验证,不部署 lab**——代码改动仅随未来 lab 镜像重建生效,本次未部署、prod DB 无新迁移、生产行为不变。
> 设计:`docs/phase2/design/trade-loop.md`(已定稿冻结,用户确认「定稿,开始实现」)。上一迭代(交易功能,trades.md)落地录入/解读/持仓后,本迭代把闭环补上:持仓 AI 诊断 + 本周交易/持仓进周报。

## 交付内容

| 文件 | 改动 |
|---|---|
| `migrations/0011_position_reviews.sql` | 新增 `position_reviews`(按 `snapshot_date` UNIQUE 缓存持仓诊断,与 `weekly_summaries` 同模式) |
| `internal/store/position_reviews.go` | 新增 `GetPositionReview` / `UpsertPositionReview`(ON CONFLICT 覆盖) |
| `internal/store/trades.go` | 新增 `LatestPositionsBefore`(周末前最近快照,防未来函数)+ `ListTradesBetween`(周报聚合) |
| `internal/web/trades.go` | 持仓诊断全链路:聚合(Go 算总市值/盈亏/集中度/联动)→ KB grounding(expandQuery + SearchKnowledgeExpanded + 每持仓笔记去重)→ `evWithin` 截断到快照日 → `validatePositionReview`(复用 `filterReviewRefs` 白名单)→ `StartTaskRun("position-review")` 记账 → 落缓存;risks 候选用户确认存笔记(slug=`posrev-{date}-{n}` 幂等);抽取 `filterReviewRefs` 共享 | 
| `internal/web/templates/trades.html` | 持仓快照区新增 AI 诊断卡(正文/模型/tokens/时间 + 引用 chips 可点 + risks 存为笔记 + 重新诊断)/ 空态按钮 |
| `internal/web/weekly.go` | 四路聚合扩为五路:`aggregateWeek` 增交易(`ListTradesBetween`)+ 持仓(`LatestPositionsBefore(end)`);页面字段 + `buildWeekContext` 增「本周交易」「本周持仓」段;nodata 条件扩五类 |
| `internal/web/templates/weekly.html` | 本周沉淀与 AI 综述之间新增「本周交易」「本周持仓(最近快照)」两表;页头副标题更新 |
| `docs/phase2/design/trade-loop.md` | 设计定稿(D-L1~L7:按 snapshot_date 缓存 / 数字 Go 算 / 复用 tradeReview 管线 / risks 单向 harvest / 周报四路聚合 + nodata 五类 / 周末前快照防未来 / 手动触发) |

## 验收证据(dev,真实 Zen reasoning 调用)

环境:dev DB(app_config:base=`https://opencode.ai/zen/go/v1`,key 已配,reasoning=extract=`deepseek-v4-flash`,budget=0 不限)。数据沿用交易功能遗留:5 笔交易 + 3 持仓(2026-08-28 快照:600519/300750/600036)+ 事件/实体已富化。

**1. 持仓诊断生成 + 缓存 + GET 零调用**:点「AI 诊断」→ 生成 2322~3603 tokens 组合诊断(组合盈亏+9760/盈亏率 4.45%/贵州茅台占比 66.2%/前 3 大 100% + 近 14 天交易联动),按 `snapshot_date=2026-08-28` 缓存;连续 3 次 GET `/trades` → `task_runs position-review` 计数不变(零调用)。

**2. 数字聚合在 Go(真 bug 抓到)**:诊断正文数字全部与 Go 聚合一致(总市值/总盈亏/盈亏率/集中度/联动笔数),LLM 未自造。**但首轮诊断中 AI 如实指出「聚合数据中最大持仓占比0.0%与个股明细占比66.2%矛盾」——不是幻觉,是真 bug**:`aggPositions` 里 `sorted` 是 `agg.Rows` 的值拷贝,占比算在 `sorted[i]` 上但抄回的是 `agg.Rows[i].MVShare`,而 `Top1Pct/Top3Pct` 读的是从未赋值的 `sorted[i].MVShare`(恒 0)。修复:占比按 code 映射回 rows,`Top1Pct/Top3Pct` 直接用 `sorted` 计算。修复后重跑:最大占比 66.2%、前 3 大 100%,矛盾风险候选消失。

**3. 引用白名单 + 引用可点**:首轮 refs 为空但正文提到「央行降准」「宁德时代麒麟5.0」(检索上下文里确实存在,非编造)→ 属 flash 模型不填 refs 的引用完整性缺口。强化 prompt(正文每提一个 KB 条目必须进对应 refs)后重跑:refs 含 2 事件 + 3 实体,id 全部真实且经白名单校验;页面引用 chips 可点跳转。

**4. 防未来函数**:插入 `2026-08-29 12:00` 未来事件「贵州茅台宣布回购100亿」→ 重跑诊断 → refs 只含 08-26 历史事件,未来事件 id 零泄漏,正文不提及;删除测试事件复原。

**5. risks 存为笔记幂等**:诊断生成 2 条风险候选(仓位集中度风险/交易集中风险)→「存为笔记」→ `personal_notes(type='mistake', status='hypothesis', slug='posrev-20260828-0')`;重复保存 → 幂等(不重复建,posrev 笔记恒 1 条)。

**6. 周报含交易/持仓 + 综述引用**:`/weekly`(2026-W35)页面显示「本周交易」5 笔、「本周持仓(最近快照)」3 行;生成 AI 综述 → 正文提及「买入中国平安、招商银行、贵州茅台,卖出平安银行与宁德时代;持仓快照仍显示茅台、宁德、招行」并复盘「笔记刚警示买入即重仓,茅台单笔金额显著高于其他交易,是否再次违背纪律」——上下文含交易/持仓且与笔记信念对照,复盘视角无买卖建议。

**7. nodata 扩展(仅交易周)**:W30(07-20~07-26,原全空)插入一笔 07-22 工商银行买入 → 生成综述 → 如实「本周无行情、事件、个人笔记及持仓快照数据,仅有一笔交易:07-22买入工商银行100股」,不编造;测试后删除复原。

**8. 降级(如实,零调用)**:① 清空 `ai_api_key` → POST 诊断返回页面提示「AI 未配置,诊断暂缺(请到 /settings 配置)」;② `ai_daily_token_budget=1`(当日已用超)→ 提示「今日 AI 预算已用尽」;③ 备份后清空 `positions` → 提示「暂无持仓快照,先导入持仓再诊断」。三种场景 `task_runs position-review` 计数均不变,配置/数据全部复原。

**9. 记账**:`task_runs position-review` 全 success,meta 含 `{"snapshot":"2026-08-28","model":"deepseek-v4-flash","ai_tokens":...}`;`weekly-summary` 4 条不受影响。

**10. 生产未动**:未部署 lab、prod 无新迁移、无新代码。

## 验收清单(design §4 全过)

- [x] 持仓诊断:有持仓 → 点「AI 诊断」→ 生成诊断(组合盈亏/集中度/交易联动/引用可点),按 snapshot_date 缓存;GET 零调用(计数不变)
- [x] 数字聚合在 Go:总市值/盈亏/集中度/联动笔数由代码算出进上下文,LLM 正文不出现未提供的数字
- [x] 引用白名单:refs id 全部来自上下文;无关联如实标注
- [x] 防未来函数:诊断不含 snapshot_date 之后事件(插未来事件实测零泄漏)
- [x] risks 存为笔记:type=mistake, slug=posrev-*, 幂等去重
- [x] 周报含交易:本周交易/持仓在 /weekly 页面显示,且 AI 综述上下文含之;综述能提及本周交易/持仓
- [x] nodata 扩展:只有交易无行情/事件/沉淀的周也能生成综述
- [x] 降级:无持仓 / AI 未配置 / 预算耗尽 → 如实提示
- [x] 记账:task_runs `position-review` 含 ai_tokens;`weekly-summary` 不受影响
- [x] 生产未动:未部署 lab、prod 无新迁移
- [x] 数据诚实:`go build`/`go vet`/`go test ./...` 全过

## 验收中发现并修复的真 bug

1. **集中度占比恒 0**(`aggPositions`):`sorted` 拷贝 `agg.Rows` 后,占比赋在 `agg.Rows[i]` 上,但 `Top1Pct/Top3Pct` 读 `sorted[i].MVShare`(拷贝时恒 0)→ 上下文「最大持仓占比 0.0%」与个股占比 66.2% 自相矛盾,被 AI 如实指出。修复:占比按 code 算在 `sorted` 上并映射回 rows,`Top1Pct/Top3Pct` 用 `sorted`。修复后数字全部自洽(这是「数字必须 Go 算」设计初衷的价值实证:LLM 一眼看出矛盾并报告)。
2. **flash 模型 refs 引用缺失**:deepseek-v4-flash 在较大上下文中不填 refs(正文提到事件但数组空)→ 违反「引用可点」。修复:prompt 强化「正文每提一个 KB 条目必须进对应 refs;不得提及未列入 refs 的条目」。修复后 refs 完整(2 事件 + 3 实体,白名单校验通过)。tradeReview 同款 prompt 沿用同样强化,未回归。

## 已知权衡与说明(数据诚实)

1. **持仓诊断只读快照 + 近 14 天交易联动,不做损益测算/T+1 配对**(design §6 边界):收盘快照与当日交易共存时,AI 可能自行注意到「卖出 300 股后仍显示持仓 300」这类口径疑点并如实标注为风险候选(实测出现),属预期行为而非缺陷——持仓是快照(可能先卖后买/大持仓减半),交易与持仓口径由用户核对。
2. **诊断缓存按 `snapshot_date` 一天一份**:同日重复点「重新诊断」覆盖旧缓存;GET 永不调用(GET 零成本,D-L1/D-L7)。
3. **引用精度依赖检索召回**:与交易解读同源(expandQuery 同义扩展 + evWithin 截断),知识库关联质量高则引用准,低则白名单兜底不产生假 id。
4. **risks 候选仅 AI 提议、用户确认才入库**(iter4 单向 harvest):提示词要求「发现组合风险点才提议」,候选可能为空,不强行生成。

## 遗留/后续(不做)

- 持仓收益测算、买卖配对(T+1 成交假设)、多笔批量复盘等回测类能力(design §6 边界,未做;进度总表可选增强)。
- 持仓自动/定时诊断(手动触发,防被动烧预算,D-L7)。
- G3/G4/G5 数据源补全(独立迭代,本迭代不碰)。
- 真向量检索(需 embedding 源;G8 已以同义扩展方案落地)。
