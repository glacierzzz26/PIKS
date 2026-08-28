# 交易功能(截图识别录入 + 知识库带引用解读)归档

> 日期:2026-08-28。范围:**仅 dev 环境验证,不部署 lab**——代码改动仅随未来 lab 镜像重建生效,本次未部署、prod DB 无新迁移、生产行为不变。
> 设计:`docs/phase2/design/trades.md`(已定稿冻结,用户确认「全做:录入+解读+持仓」)。用户需求:每天自交易提供进来,知识库解读,库不具备则完善数据;录入方式为同花顺「今日交易」/「持仓」截图。

## 交付内容

| 文件 | 改动 |
|---|---|
| `migrations/0010_trades.sql` | 新增 `trades`(交易事实,唯一去重键语义=同天同code同side同qty)+ `positions`(持仓快照,V1 只存只展示) |
| `internal/store/trades.go` | 新增 `InsertTrades`(tx 批量)/`ListTrades`/`GetTrade`/`SetTradeReview`/`TradeExists`(预览去重)/`InsertPositions`/`LatestPositions` |
| `internal/store/entities.go` | 新增 `EnsureCompanyEntity`:按名称查 → detail->>code 查 → 都没有才建,`detail.source='trade-import'` 标注(来源可审计,description 不编造) |
| `internal/store/personal_notes.go` | 新增 `ListPersonalNotesByText`(交易解读引用笔记,ILIKE title/content,非 archived 优先) |
| `internal/ai/ai.go` + `openai_compat.go` | `StructuredRequest` 增 `Image *ImagePart`;`StructuredOutput` 带图时 user content 拼 `[{text},{image_url data URI}]` + `response_format=json_object`(探针 G7 验证 vision 兼容) |
| `internal/web/trades.go` | 4 块:预览确认入库(截图→vision→预览可编辑→勾选确认)、手动录入兜底、交易记录(解读卡 + 引用 chips 可点 + mistake「存为笔记」)、持仓快照;`tradeReview` 走 KB grounding(expandQuery + SearchKnowledgeExpanded + ListPersonalNotesByText)+ 防未来函数(`evWithin` 截断到 trade_date)+ 引用白名单校验;预算护栏 + task_runs 记账;note 状态机(g=added/imported/positions_imported/reviewed/mistake_saved) |
| `internal/web/templates/trades.html` | 4 个 section + base.html 导航加「交易」(笔记↔周报之间) |
| `docs/phase2/design/trades.md` | 设计定稿(D-T1~T7:独立表/预览确认/复盘视角带引用/实体自动补/用户确认 harvest/StructuredOutput 加 Image/positions 不调 AI) |

## 验收证据(dev,合成同花顺截图 + 真实 Zen 视觉/推理调用)

环境:dev DB(app_config:base=`https://opencode.ai/zen/go/v1`,key 已配,`ai_model_vision=deepseek-v4-flash-vision-exp`,extract/reasoning=`deepseek-v4-flash`,budget=0 不限)。合成截图用 Pillow + Noto Sans CJK SC 生成(同花顺风格深色标题栏 + 表格行)。

**1. 今日交易截图导入(主路径)**:上传含 4 行(贵州茅台/宁德时代/招商银行/平安银行,含代码列+日期)的截图 → vision 抽取 → 预览 4 行全对(日期/代码/名称/方向/价格/数量/金额)→ 勾选确认 → 4 笔 `source='screenshot'` 落库 + 实体补全(贵州茅台/招商银行/平安银行新建 `trade-import`,宁德时代**已存在按名称复用不动**)。

**2. 持仓截图导入**:3 行(含成本价/现价/市值/盈亏)→ 预览 → 确认 → `positions` 落库 3 行 `/trades` 展示最近快照。

**3. 手动录入兜底**:中国平安 601318 买入 → `source='manual'` + 实体补全。

**4. 去重**:重传同图 → 预览 4 行全标「已存在」且默认不勾选(`TradeExists` 同天同code同side同qty)。

**5. AI 解读(带引用 + 防未来函数)**:点「AI 解读」→ 生成 876~1124 tokens 解读,refs 含事件/实体可点跳转;插一条 2026-08-29 未来测试事件后重跑 → refs 只含 08-26 降准(历史),08-29 事件零泄漏(防未来函数真触发)。解读文本实测:「央行宣布下调存款准备金率0.25个百分点…中国平安作为保险与金融综合集团…知识库中暂无与该股票直接相关的个人笔记,因此无法对照既有信念进行印证…」——严格基于检索上下文,无新增事实。

**6. 无关联如实标注**:茅台解读明说「知识库中无贵州茅台相关的个人笔记或案例…无法印证或违背既有信念」。

**7. mistake → 存为笔记**:注入一条 review mistake 候选 → 「存为笔记」→ `personal_notes(type='mistake', status='hypothesis', slug='trade-{id}-{n}', content 透传)`;再点 → 幂等提示「已存为笔记,未重复创建」(GetPersonalNoteBySlug 去重)。

**8. 降级(如实,零调用)**:① 清空 `ai_model_vision` → 导入提示「AI 未配置或视觉模型未配置…可用手动录入兜底」;② `ai_daily_token_budget=1`(当日已用超)→ 提示「今日 AI 预算已用尽,导入暂缓」;③ 纯色非交易图 → 视觉返回空数组 → 提示「未识别到交易/持仓,请检查截图…或手动录入」,不渲染预览表。测试后配置全部复原。

**9. 记账**:task_runs `trade-import` ×5(含 position kind,vision 模型,762~1035 tokens)、`trade-review` ×3(1124/821/876 tokens)全 success。

**10. 生产未动**:未部署 lab、prod 无新迁移、无新代码。

## 验收清单(design §4 全过)

- [x] 截图导入:今日交易截图 → 视觉抽取 → 预览表(名称/代码/买卖/价格/数量)→ 勾选确认入库;task_runs 记 `trade-import`
- [x] 持仓截图导入:positions 落库并在 /trades 展示最近快照
- [x] 手动录入兜底:表单提交一条交易
- [x] 去重:同天同 code 同 side 同 qty 已存在 → 预览标「已存在」默认取消勾选
- [x] 实体补全:交易股票不在 entities → 自动建 company 实体(detail.source='trade-import');已有则复用不动
- [x] AI 解读:点「AI 解读」→ 高智档带引用复盘(review + refs 可点跳事件/实体/笔记),内容严格基于上下文
- [x] 防未来函数:历史交易解读不含 trade_date 之后事件(插 08-29 未来事件实测零泄漏)
- [x] 无关联:知识库无该股票 → 解读如实标注,不编造
- [x] mistake 候选 → 「存为笔记」→ personal_notes(type=mistake, status=hypothesis)
- [x] 降级:未配置/视觉未配置/预算耗尽/识别为空 → 如实提示,不调用不编造
- [x] 记账:review/import 均记 task_runs(含 ai_tokens)
- [x] 生产未动:未部署 lab、prod 无新迁移/无新代码
- [x] 数据诚实:页面标注如实反映;`go build`/`go vet`/`go test ./...` 全过

## 验收中发现并修复的真 bug

1. **position 预览反序列化丢字段**:`buildImportPreview` position 分支用 `model.Position`(只有 `db:` 标签)反序列化 vision 返回 JSON → encoding/json 无法把 `cost_price`/`market_value` 这类 snake_case 键匹配到 `CostPrice`/`MarketValue` 字段,静默丢弃。修复:改带 `json:"cost_price"` 等标签的内联 struct(与 trade 路径一致)。修复后成本价/市值正确入预览与落库。
2. **mistake 表单模板渲染崩溃**:`templates/trades.html` 的「存为笔记」表单在嵌套 range(`.Review.Mistakes`)里用 `{{$.ID}}`(根=TradesPage 无 ID)→ 整页渲染中断(持仓快照区整段消失,此前 mistakes 为空从未暴露)。修复:外层 range 顶部捕获 `{{$trade := .}}`,表单用 `{{$trade.ID}}`。修复后 5 张交易卡 + 持仓表 + mistake 按钮全渲染,点击闭环幂等提示正常。

## 已知权衡与说明(数据诚实)

1. **截图识别是「预览确认」不是自动**:识别有误是常态(名称/代码缺列、方向色误判),人工核对后才入库(D-T2)。
2. **positions V1 只存只展示,不调 AI**(D-T7):持仓是状态非事件,解读价值在交易;后续可做持仓诊断(成本/盈亏对照)。
3. **mistake 候选生成偏保守**:实测两次真实解读 mistakes 均空(模型不强行生成),「存为笔记」以用户确认为准(D-T5 单向 harvest)。
4. **trades/positions 数据不进「本周沉淀」周报聚合**(D-T1 独立表):交易是结构化事实,不污染个人笔记聚合。
5. **解读引用范围 = 检索召回(expandQuery 同义扩展)+ 交易时点截断**:引用精度依赖知识库关联质量;茅台解读曾出现「检索混入无关实体」的如实自述,属知识库精度话题非本功能缺陷。

## 遗留/后续(不做)

- 交易/持仓数据接入周报聚合或收益测算(超出本次「录入+解读+持仓」范围)。
- 持仓诊断(成本/盈亏对照)、交易对账单导出、历史持仓走势(可选增强)。
- 多笔交易的批量复盘、买卖配对(T+1 成交假设)等回测类能力(设计 §6 边界,未做)。
- 截图 OCR 本地降级(web-app §4.3 备选):G7 视觉模型实测通过,未走此路。
