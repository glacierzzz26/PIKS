# 交易功能设计文档(截图识别录入 + 知识库带引用解读)

> 状态:**草案(待定稿门确认,2026-08-28)**。范围:把「我做了什么」(每日自交易)接入知识库,补全个人学习闭环——交易记录 → AI 复盘解读(带引用)→ 沉淀成笔记。录入以**同花顺今日交易/持仓截图 → 视觉模型抽取**为主路径,手工表单兜底;知识库缺该股票数据则**自动补实体(完善数据)**。
> **约束:只做 dev 验证,不部署 lab**——代码改动仅随未来 lab 镜像重建生效,本次未部署、prod DB 无新迁移、生产行为不变。
> 契约依据:`iter4-learning-loop.md`(个人学习闭环/单向 harvest 语义)、`web-app.md` §4(/chat 截图 + 引用)、`internal/web/chat.go`(截图上传/视觉调用)、`internal/store/search.go`(G8 检索)、`internal/model/model.go`(Event/Entity/PersonalNote 真实 DTO)、`internal/ai/*`(StructuredOutput 现状)。

---

## 1. 背景与现状

系统已有「我懂什么」(笔记 Belief/Case/Mistake)、「市场发生什么」(事件/实体/行情)、「AI 解读」(/chat 带引用 + 周报综述),但**缺「我做了什么」**。用户(2026-08-28)提出:提供每日自交易,知识库解读;知识库不具备则完善数据。录入形态:用户提供**同花顺 App 今日交易 + 持仓截图**(图片输入,非手工逐条录入)。

可复用能力(真实契约):
- **截图上传/视觉**:`/chat` 已验证(png/jpeg/webp/gif,5MB,`saveUpload` 落盘 + attachments 表 + `ai.ImagePart`),视觉模型 `ai_model_vision`(`deepseek-v4-flash-vision-exp`)接受 image_url(G7 探针实测)。
- **知识库检索**:`SearchKnowledgeExpanded`(G8 同义扩展 + 关键词,事件/实体按 n-gram 文本命中打分)。
- **个人笔记**:`personal_notes`(belief/case/mistake/note),`CreatePersonalNote`;iter4 单向 harvest 语义(AI 提议,用户确认入库)。
- **记账/预算**:`task_runs` + `TokensSince` + `ai_daily_token_budget`(全局日账本)。
- **实体补全**:`entities` 单表(type='company',detail={code}),UNIQUE(type,name)。

现状缺口:**`StructuredOutput` 不支持图片**(仅文本 JSON mode)。截图 → 结构化交易需要「视觉模型 + JSON mode」,需扩展 `StructuredRequest` 支持 `Image`(探针验证 vision+json_object 兼容,失败回退 Chat 带图 + JSON 解析)。

## 2. 方案

### 2.1 新表 `trades`(migration 0010)+ 新表 `positions`(持仓快照)

```sql
-- 交易记录(同花顺今日交易截图抽取 / 手动录入;结构化事实,非认知沉淀)
CREATE TABLE trades (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  trade_date  DATE NOT NULL,              -- 交易日期
  code        TEXT NOT NULL,              -- 证券代码(6 位)
  name        TEXT NOT NULL,              -- 证券名称
  side        TEXT NOT NULL,              -- buy / sell
  price       NUMERIC(12,3) NOT NULL,     -- 成交价
  qty         INT NOT NULL,               -- 数量(股)
  amount      NUMERIC(16,2) NOT NULL,     -- 成交金额(截图冗余/可算)
  source      TEXT NOT NULL DEFAULT 'manual',  -- manual / screenshot
  attachment_id UUID,                     -- 来源截图附件(id → attachments)
  note        TEXT,                       -- 自评(可选,手动/导入后可补)
  review      JSONB NOT NULL DEFAULT '{}'::jsonb, -- AI 复盘(见 2.4:{review,refs,model,tokens,generated_at})
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_trades_date ON trades(trade_date DESC, created_at);
CREATE INDEX idx_trades_code ON trades(code);

-- 持仓快照(同花顺持仓截图;仅存只展示,V1 不做 AI 复盘)
CREATE TABLE positions (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  snapshot_date DATE NOT NULL,            -- 截图日期(持仓快照日)
  code        TEXT NOT NULL,
  name        TEXT NOT NULL,
  qty         INT NOT NULL,               -- 持有数量
  cost_price  NUMERIC(12,3),              -- 成本价
  price       NUMERIC(12,3),              -- 现价
  market_value NUMERIC(16,2),             -- 市值
  pl          NUMERIC(16,2),              -- 盈亏
  source      TEXT NOT NULL DEFAULT 'screenshot',
  attachment_id UUID,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_positions_date ON positions(snapshot_date DESC);
```

- **trades 独立表而非塞 personal_notes**:交易是结构化事实(日期/代码/买卖/价量),笔记是认知沉淀,分开才干净(与 D-W1 同理)。
- **positions 只存只展示**:持仓是「当前状态」非交易事件;给复盘提供上下文,但 V1 不调 AI 解读(边界见 §6)。
- 零关系依赖;attachment_id 弱引用 attachments(不建 FK,截图可清理)。

### 2.2 录入:截图 → 视觉抽取 → 预览确认 → 入库(主路径)+ 手动表单(兜底)

**A. 截图导入(POST /trades/import,multipart `file` + `type=trade|position`)**
1. 复用 `/chat` 上传校验(图片类型/5MB,`allowedImageType`),`saveUpload` 落盘 + attachments 元数据。
2. 读 app_config(每请求重读):base/key/`ai_model_vision`。vision 未配置 → 如实提示「视觉模型未配置(请到 /settings 配置),可用手动录入」,不调用。
3. 预算护栏:今日已用 ≥ `ai_daily_token_budget` → 如实提示,不调用。
4. **视觉结构化抽取**:`StructuredOutput`(temperature 0 + json_object)**扩展 Image 支持**(见 3.3 契约 + 探针)。prompt 护栏:严格只抽取截图中明确出现的条目;名称/代码缺失标 `null` 由用户补;价格/数量不做推断;若图片非交易/持仓页 → 返回空数组,如实提示「未识别到交易,请检查截图或手动录入」,不编造。
   - 交易 schema:`{"trades":[{"date":"2026-08-28","code":"600519","name":"贵州茅台","side":"buy","price":1450.5,"qty":100,"amount":145050}]}`
   - 持仓 schema:`{"positions":[{"code":"600519","name":"贵州茅台","qty":100,"cost_price":1400,"price":1450.5,"market_value":145050,"pl":5050}]}`
5. **预览确认,不自动入库**:抽取结果列表(每行 勾选/可编辑 名称/代码/买卖/价格/数量)+ 原图缩略 → 用户核对后「导入勾选项」。**防误识别污染数据**(同花顺截图可能缺代码列,识别有误是常态)。与既有交易(同 trade_date+code+side+qty)匹配的行默认取消勾选并标注「已存在」,防重复导入。
6. 确认后批量入库 + 实体补全(见 2.3)。task_runs 记账 `command='trade-import'`(ai_tokens=抽取 token)。

**B. 手动录入(POST /trades/add,表单)**:trade_date/code/name/side/price/qty/note。兜底路径(视觉未配置/识别失败/补漏单笔)。

### 2.3 完善数据:交易股票 → 自动补实体(用户「知识库不具备就完善」)

确认导入/录入时,对每只交易股票 `EnsureCompanyEntity(code, name)`:
- 查 entities:`type='company' AND (name=$name OR detail->>'code'=$code)`。
- 缺 → `INSERT entities(type='company', name=$name, aliases='[]', detail={code:$code, source:'trade-import'})`,返回新 id。
- `detail.source='trade-import'` 标注来源可审计;description 留空(不编造)。
- 已有 → 返回既有 id,不动。

这样知识库随交易自动长全(实体卡/图谱/检索都能看到交易过的公司),且来源透明。

### 2.4 AI 解读:交易 + 知识库 grounding → 高智档带引用复盘

触发:交易卡「AI 解读」按钮(POST `/trades/{id}/review`),**手动触发,不做自动解读**。

1. 读 app_config:base/key/`ai_model_reasoning`(高智档;未配置回退 `ai_model_extract`;都空 → 「AI 未配置」)。预算护栏同上。
2. **KB grounding**(与 /chat 同源):
   - `expandQuery` 同义扩展(股票名 + 代码 → 扩展词),`SearchKnowledgeExpanded(q=名称+代码, extra, events=8, entities=8)`。
   - 关联个人笔记:`ListPersonalNotesByText(名称, limit=8)`(新 store 函数,title/content ILIKE 名称)。
   - **防未来函数**:历史交易解读,事件过滤 `occurred_at/created_at ≤ trade_date`(今日交易则当日全含)——解读只用交易时点及之前信息,与回测纪律一致。
3. `StructuredOutput`(reasoning,temperature 0)输出:
   ```json
   {"review":"复盘文字(≤300字,复盘视角非建议)","refs":{"events":["E:id"],"entities":["N:id"],"notes":["P:id"]},"mistakes":[{"title":"...","content":"..."}]}
   ```
   prompt 护栏:复盘「这笔交易与哪些知识相关、值得复盘什么」,**不输出买卖建议**;严格只基于上下文;知识库无关联 → 如实标注「知识库无该股票相关事件/实体」;mistake 只作候选。
4. **引用防自造**:refs 里的 id 必须在检索结果集合内(extractRefs 同逻辑),解析失败/空 → 如实失败不入库。
5. 结果写 `trades.review`(JSONB,含 review/refs/model/tokens/generated_at);页面渲染带可点引用(事件/实体/笔记跳转)。
6. **mistake 候选**:列表展示,用户点「存为笔记」→ `CreatePersonalNote(type='mistake', status='hypothesis', ...)`(iter4 单向 harvest:A 提议、用户确认)。
7. task_runs 记账 `command='trade-review'`,ai_tokens 计入日账本。

### 2.5 失败与降级(数据诚实)

| 情形 | 行为 |
|---|---|
| AI 未配置(base/key/model 空) | 如实提示,不调用不编造 |
| 视觉模型未配置 | 导入如实提示,可用手动录入 |
| 预算已用尽 | 如实提示「今日 AI 预算已用尽」,不调用 |
| 截图非交易/持仓页、识别为空 | 如实提示「未识别到交易」,不编造条目 |
| LLM 调用/解析失败 | 如实提示,不入库不写 review |
| 知识库无关联 | 解读如实标注「无关联」,不编造 |
| 导入同天重复 | 预览标「已存在」默认取消,不重复入库 |

## 3. 契约

### 3.1 store(`internal/store/trades.go` + `entities.go`/`personal_notes.go` 增)

```go
type Trade struct {
    ID string; TradeDate time.Time; Code, Name, Side string
    Price float64; Qty int; Amount float64
    Source string; AttachmentID *string; Note *string
    Review json.RawMessage; CreatedAt, UpdatedAt time.Time
}
func (s *Store) InsertTrades(ctx, ts []model.Trade) error
func (s *Store) ListTrades(ctx context.Context, limit int) ([]model.Trade, error)   // 按 trade_date DESC
func (s *Store) GetTrade(ctx, id string) (*model.Trade, error)
func (s *Store) SetTradeReview(ctx, id string, review json.RawMessage) error
func (s *Store) TradeExists(ctx, date time.Time, code, side string, qty int) (bool, error) // 去重提示
// positions(只存只展示)
func (s *Store) InsertPositions(ctx, ps []model.Position) error
func (s *Store) LatestPositions(ctx context.Context) ([]model.Position, error) // 最近快照
// 实体补全
func (s *Store) EnsureCompanyEntity(ctx, code, name string) (string, error)    // 缺则建(detail.source='trade-import')
// 解读引用(个人笔记检索)
func (s *Store) ListPersonalNotesByText(ctx, q string, limit int) ([]model.PersonalNote, error)
```

### 3.2 web(`internal/web/trades.go` + `templates/trades.html` + `server.go` + `base.html` 导航)

```go
type TradesPage struct {
    Common
    Trades   []store.TradeView   // 交易 + 去重/可点引用(含 review 解包)
    Positions []store.PositionView // 最近持仓快照(若有)
    Note     string              // 导入/解读结果提示(如实)
}
// 路由(server.go):
mux.HandleFunc("/trades", s.handleTrades)      // GET 列表 + POST add/import/confirm 分流
mux.HandleFunc("/trades/", s.handleTrade)      // /trades/{id}/review(POST 解读)
// handleTrades:
//   GET → ListTrades + LatestPositions → render
//   POST action=add      → 手动录入 → redirect
//   POST action=import   → 视觉抽取 → 预览态(抽取结果暂存内存/表单回显) → confirm
//   POST action=confirm  → 批量入库 + EnsureCompanyEntity → redirect
// handleTrade:
//   POST /trades/{id}/review → generateTradeReview → redirect ?g=
```
- 模板:交易列表卡(日期/名称/代码/买卖/价/量/金额/自评 + 「AI 解读」按钮 + review 卡 + refs chips + mistake 候选「存为笔记」)+ 导入区(上传表单 + 预览表)+ 手动录入表单 + 持仓快照表。
- `base.html` 导航增 `交易`(`/trades`,Active='trades')。
- 状态回显 `?g=ok|noconfig|novision|budget|failed|empty`(如实标注,复用 weekly `genNote` 风格)。

### 3.3 ai(`internal/ai/ai.go` + `openai_compat.go`)

```go
// StructuredRequest 增:
Image *ImagePart  // 非空 = 视觉 + json_object(截图结构化抽取)
```
- `OpenAICompat.StructuredOutput`:Image 非空时 user content 变 `[{text},{image_url data URI}]`(与 Chat 一致),response_format 保持 json_object。
- **探针(实现前)**:实测 `deepseek-v4-flash-vision-exp` 是否接受 `response_format=json_object` + image_url 并存(openai-compat 约定)。不兼容 → 回退实现:`Chat(带图)` + 宽容 JSON 提取(剥 ```json 围栏/取首个 `{...}`),解析后仍走同一校验。
- 不兼容不阻塞:两种路径都返回 `StructuredResponse{Data,Usage}`,上层无感。

## 4. 验收清单(dev-only)

- [ ] 截图导入:上传同花顺今日交易截图 → 视觉抽取 → 预览表(名称/代码/买卖/价格/数量)→ 勾选确认入库;task_runs 记 `trade-import`
- [ ] 持仓截图导入:positions 落库并在 /trades 展示最近快照
- [ ] 手动录入兜底:表单提交一条交易
- [ ] 去重:同天同 code 同 side 同 qty 已存在 → 预览标「已存在」默认取消勾选
- [ ] 实体补全:交易股票不在 entities → 自动建 company 实体(detail.source='trade-import');已有则复用不动
- [ ] AI 解读:点「AI 解读」→ 高智档带引用复盘(review + refs 可点跳事件/实体/笔记),内容严格基于上下文
- [ ] 防未来函数:历史交易解读不含 trade_date 之后事件
- [ ] 无关联:知识库无该股票 → 解读如实标注,不编造
- [ ] mistake 候选 → 「存为笔记」→ personal_notes(type=mistake, status=hypothesis)
- [ ] 降级:未配置/视觉未配置/预算耗尽/识别为空/失败 → 如实提示,不调用不编造
- [ ] 记账:review/import 均记 task_runs(含 ai_tokens)
- [ ] 生产未动:未部署 lab、prod 无新迁移/无新代码
- [ ] 数据诚实:页面标注如实反映;`go build`/`go vet`/`go test ./...` 全过

## 5. 涉及文件

- 新增:`migrations/0010_trades.sql`、`internal/store/trades.go`(trades+positions)、`internal/web/trades.go`、`internal/web/templates/trades.html`。
- 修改:`internal/ai/ai.go`(`StructuredRequest` + `Image`)、`internal/ai/openai_compat.go`(`StructuredOutput` 视觉)、`internal/store/entities.go`(`EnsureCompanyEntity`)、`internal/store/personal_notes.go`(`ListPersonalNotesByText`)、`internal/web/server.go`(路由)、`internal/web/templates/base.html`(导航)。
- 复用:`saveUpload`/`allowedImageType`/`ImagePart`/`Chat`、`SearchKnowledgeExpanded`、`expandQuery`、`task_runs`/`TokensSince`/预算、`CreatePersonalNote`、`extractRefs` 思路、`attachment` 表。
- 测试:web 无现成单测(验收以 dev 实测 + `go vet`/`go test ./...` 全过为准);`EnsureCompanyEntity`/refs 防自造等纯逻辑可加单测。

## 6. 边界(明确不做)

- ❌ 不做自动解读/自动导入(GET 永不调 LLM;导入/解读均手动显式触发,防预算意外消耗)。
- ❌ 不做持仓 AI 复盘/持仓历史对比(V1 只存只展示;后续可选)。
- ❌ 不做交易绩效统计(收益率/胜率/持仓盈亏曲线)——独立话题,后续。
- ❌ 不做买卖建议(AI 只复盘,不预测/不荐股;prompt 护栏硬约束)。
- ❌ 不做 CSV/券商对账单导入(截图识别已覆盖主路径;手动表单兜底)。
- ❌ 不做生产部署:不重建 lab 镜像、不触发 prod 管线。

## 7. 决策留痕

| # | 决策 | 理由 |
|---|---|---|
| D-T1 | `trades`/`positions` 独立表而非塞 personal_notes | 交易是结构化事实(日期/代码/价量),笔记是认知沉淀;分开防污染「本周沉淀」 |
| D-T2 | 截图 → 预览确认才入库,不自动入库 | 截图识别有误是常态(名称/代码缺列),防误识别污染;数据诚实 |
| D-T3 | 解读 = 复盘视角 + 带引用 + 防未来函数 | 非买卖建议;引用防 LLM 自造;历史交易只用交易时点及之前信息(回测纪律) |
| D-T4 | 实体补全自动 + `detail.source='trade-import'` 标注 | 知识库缺什么补什么,来源可审计;description 不编造 |
| D-T5 | mistake 候选由用户确认入库 | iter4 单向 harvest 语义:A 提议、用户确认,不自动写认知 |
| D-T6 | `StructuredOutput` 加 Image 支持(探针验证) | 截图结构化抽取复用 json_object;vision+json_object 兼容待探针,不兼容回退 Chat+JSON 解析(上层无感) |
| D-T7 | positions V1 只存只展示,不调 AI | 持仓是状态非事件,解读价值在交易;缩小 V1 AI 面 |
