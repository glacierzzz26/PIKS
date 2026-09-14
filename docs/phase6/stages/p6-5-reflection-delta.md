# P6-5 复盘→沉淀修复 + 研究 delta + 收尾 —— 实现与验收

> 阶段:前端决绝重构 P6 第 5 阶段(末阶段)。设计依据 `docs/phase6/design/ux-ia.md` §3.2 / §3.4,收尾见 §4/§5。
> 目标:补 epic 闭环 #3「我错在哪」—— 持仓诊断结论一键沉淀为笔记并**回流**个股页与笔记库;`/stock/:code` 新增「研究变化」对比。**零 schema。**

## 交付

### 1. 复盘→沉淀回流(§3.2 修②,零 schema)
- **断链**:`tradeSaveMistakeCore` / `positionSaveRiskCore` 建了 `personal_notes` 却不建 `relationships` 边,
  结论永不进个股页「我的笔记」(`ListNotesReferencingEntity` 查的是 `(personal_note→entity,'references')`)。
- **修**:新增 `linkSavedNote(ctx, noteID, code, eventID, refs)`(trades.go):
  - 笔记 → 公司实体 `'references'`(code 经 `GetCompanyEntityByCode` 解析;无实体则跳过,不报错);
  - 笔记 → 诊断引用的事件/实体(refs 白名单过滤后的真实论据,去重建边)。
  - **建边失败不回滚笔记**(已入库有效),但如实报错「已存为个人笔记,但关联回流失败」——不假装全成功。
- `tradeSaveMistakeCore` 传该笔交易的 `t.Code`;`positionSaveRiskCore` 组合级无单一 code,只关联 refs 的实体/事件。
- **顺修 (缺陷 #2 的隐藏面)**:`/reviews` 可点**旧期**诊断,原 `positionSaveRiskCore` 恒取最新快照 →
  会按最新期索引取错条目(**静默存错内容**)。改签名收 `snapshot time.Time`(零值=最新,兼容既有调用),
  API 加可选 `?snapshot=YYYY-MM-DD`;前端按行传入该期日期。
- 缺陷 #1(`/api/v1/reviews` 丢弃 mistakes)**此前已修**(`toAPIReview` 映射 Mistakes,P6-1),本阶段无需动。

### 2. `/reviews` 渲染 + 存为笔记(前端)
- `ReviewPointList.tsx`(新):逐条结论 + 「存为笔记」按钮,行内展示 title + content(原页面只显示 title)。
- `reviews.tsx` 重写:风险点 + 复盘点两组各自可存,`pathFor` 带 `?snapshot=` 精确到该期;空态指路 `/trades`。

### 3. 研究 delta(§3.4,纯前端)
- `hooks/useResearchDelta.ts`(新):取该股最近两份 **done** 报告详情(`/research-runs?code=X` summary 无 metrics →
  逐份 GET 详情),`buildDelta` 纯函数 diff `price.{end_price,period_return_pct,return_pct_20d,max_drawdown_pct}`
  + `scorecard.overall` + `risk.overall_level`;`<2 份 done` → `hasPair=false`,不拉取不伪造。
- `StockResearchDelta.tsx`(新):四列「指标 / 上次 / 最新 / 变化」;涨跌 A 股习惯(涨红跌绿),
  评分/风险/回撤中性不着色(同 Fact 区 `COLORED` 约定);缺失显 —;表头对账避免「最新」列实为差值。
- `RISK_LEVEL_LABEL`(constants):low/medium/high → 低/中/高。
- 个股页在「深研报告」前插入「研究变化」区块。

### 4. 收尾(§4/§5 就近拆分)
- `ForceGraph(281 行)` → `useForceSim.ts` hook(模拟 + 交互 + refs),组件只留视图(59 行)。
  `NODE_COLOR` 从 hook re-export 保持 `graph.tsx` import 不变。
- `weekly.tsx(219 行)` → 抽 `SummaryPanel.tsx`(AI 综述面板),页面 176 行。

## 验收

| 项 | 结果 |
|---|---|
| `go build ./...` / `go vet ./...` / `go test ./...` | ✅ 全过 |
| `tsc --noEmit`（`npm run lint`） | ✅ |
| `npm run build` | ✅（JS 972.67 kB / gzip 312.09）|
| **`/reviews` 存一条风险 → 笔记出现在 `/stock/:code`「我的笔记」且 `/notes`** | ✅ `save-risk/1?snapshot=2026-08-28` → 新笔记 + 5 条 references 边(3 实体 + 2 事件)读回;`/stock/600036`「我的笔记」出现该笔记,`/notes` 计数 4→5 |
| 同理 `save-mistake/{n}`(单笔交易)回流 | ✅ 临时造 trade(600036,含 mistakes+event)→ 存为笔记 → `(note→entity 招商银行)+(note→event 麒麟电池)` 建边;/stock/600036 与 /notes 均出现;测后删除 |
| 幂等(同条目重复存) | ✅ 返回「该风险候选已存为笔记,未重复创建」;`/notes` 计数不再增 |
| `?snapshot` 定位旧期(**修静默存错**) | ✅ `snapshot=2026-08-28` 正确命中该期;非法日期 400;`snapshot=1999-01-01` 如实「诊断不存在」 |
| 研究 delta 正确 diff 两个真实 run | ✅ 临时插 2 份 done(000560,scorecard 4→6、end_price 2.30→2.74)→ 渲染综合评分 +2 / 期末价 +0.44 / 区间涨跌 +15.03% / 近20日 +4.00% / 最大回撤 −1.90% / 风险 中→高;测后删除 |
| delta 「<2 份 done」如实空态 | ✅ 无第二份 → 「暂无历史报告可对比」 |
| 图谱隔离无回归 | ✅ 建边前后 `/graph` **9 节点 / 4 边**不变(`references` 非排除类型,但 note 节点不在 `/graph` 投影内) |
| e2e 冒烟 28 通过 / 0 失败 | ✅（本地 PG:5433 + Go:8090 + vite:3100,改动后复跑）|
| Playwright UI 实测(reviews/delta) | ✅ `/reviews` 渲染风险点 + 2 个「存为笔记」;`/stock/:code` 研究变化四列渲染;0 console 错误 |

### 历史数据回填
- 存量为 0 的 `posrev-20260828-0` 类旧笔记此前无实体边;对既有 `trade-88441c17-...-0`(贵州茅台)补了一条
  `(personal_note→entity,'references')`(`source='review-backfill'`),使历史沉淀也回流 —— 幂等(ON CONFLICT DO NOTHING)。

## 偏差与遗留
- **`based_on` 事后补关联**仍未支持(承 P6-4 遗留:只能录入时关联)。
- **delta 只比 top-2 done**:三份以上不显示趋势线;「价值较低可砍」项,按设计保持最小实现。
- **组合级诊断只关联 refs 实体/事件**,不关联「组合」概念(无组合实体,零 schema 下不造)。
- **`positionSaveRiskCore` 签名变更**:内部核心函数,唯一调用点(api_write.go)已同步;无外部契约破坏。
- 部署 lab 待 P6 全部阶段收尾后统一进行。
