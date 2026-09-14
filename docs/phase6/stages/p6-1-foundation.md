# P6-1 地基与诚实 —— 实现与验收

> 阶段:前端决绝重构 P6 第 1 阶段。设计依据 `docs/phase6/design/ux-ia.md` §4/§5/§7。
> 目标:把设计系统收敛为单系统 + 修掉会被新首页原样焊进去的数据诚实/死重缺陷。

## 交付

### 设计系统重建（`frontend/src/globals.css` 1,735 → ~1,150 行）
- 确立规则:**Tailwind 只做布局,视觉一律走 `globals.css` 语义类**。文件抬头写明。
- **圆角 3 token**:`--radius-card:16px / --radius-md:10px / --radius-sm:6px`;`tailwind.config.ts` 映射 `{ sm:6px, DEFAULT:10px, lg:16px }`。
- **删 20 个零引用死类**:`badge-warm card-title chip-accent chip-up chip-down chip-amber chip-dim expo-warn-line form-grid g-label gauge-sub import-banner num-left rv-point side-buy side-sell side-tag snap-grid topbar-glass`。用 className token 提取精确判定(非字符串误报);原计划 19 个,实测多出 `g-label`(1 处命中为 `text-faint` 误报,实为死类)。
- **修 8 处硬编码浅色渐变/色 → 主题感知**:`.hero`/`.import-banner`(已删)/`.pct-bar i`/`.expo-track i.lead`/`.expo-track i.ok`/`.wk-head .btn`/`.btn-save`/`.side-logo`。新增 `--brand-a/--brand-b`(暗色为 #33518f→#4a6fd0,保白字可读)、`--amber-a/--amber-b`。
- **类型标签 `.t-*` 改 CSS 变量**(`--tag-*-fg/bg`),修 `.t-mix/.t-idx/.t-ev/.t-bond` 在暗色下失真;`.t-gold` 前景改变量(原硬编码 #a97f2c 暗色不可读)。
- **新增 `.txt-faint`**(取代散落的 `style={{color:"var(--ink-faint)"}}`)。
- **表格对齐提权**:`.table th.text-left/right/center` 覆盖 `.table thead th` 的默认右对齐(0,1,2 > 0,1,0),使 Tailwind 对齐类能作用于单元格。
- **去 framer-motion**:抽屉改纯 CSS keyframes(`.drawer-scrim`/`.drawer-panel`,淡入 + 右滑)。

### 数据诚实
| # | 缺陷 | 修复 |
|---|---|---|
| 1 | `/api/v1/reviews` 用 risks+**mistakes** 算 state 却丢弃 mistakes | Go 加 `apiReview.Mistakes`;抽出纯函数 `toAPIReview` 便于回归;前端 `ReviewRow.mistakes` + `/reviews` 渲染「复盘点」（金色标记）|
| 4 | ladder 的 `turnover/float_mv/first_time/reason` 源数据未采集,UI 却渲染 `0.0%`/`0 亿` | `/ladder` 表按 `>0`/非空判定,无值显 `—`（Go 注释自证「源数据未采集 → 如实零值」）|
| 5 | `DOC_TYPE_LABEL` 缺 `belief`/`case` | 补「信念」「案例」（后端 type 枚举确为 note/belief/case/mistake）|
| 3 | Go 用户可见字符串 ~30 个 `⚠️`/`✅` emoji | `internal/web/{trades,chat,api_write}.go` 全量去除,保留纯文本 |

### 死重清理
- 删 3 死依赖:`@tanstack/react-table`、`@xyflow/react`（零 import）、`framer-motion`（1 处已改 CSS）。
- **echarts 按需注册**:`echarts/core` + `BarChart`/`GridComponent`/`TooltipComponent`/`LabelLayout`/`CanvasRenderer`,导出 `PiksChartOption` 替代 `EChartsOption`。
- **拆超 150 行文件**:`settings.tsx`(217)→ 46 行页 + `AiServiceForm`(98) + `Field`(20) + `useAiSettingsForm` hook(75);`dashboard/widgets.tsx`(173)→ `Gauge.tsx`(136) + `Pipeline.tsx`(39)。
- 内联样式:约 40 处 `style={{color/textAlign}}` → `txt-faint`/`text-up`/`text-down`/`text-amber`/`text-left` 类。

### 文档
- `CLAUDE.md`:技术栈改为实测（ECharts 5 按需、自绘 SVG 力导、原生 table、纯 CSS 动画、无表格/图库）;设计令牌改为实测值（含暗色）+ 3 档圆角规格。

## 验收

| 项 | 结果 |
|---|---|
| `go build ./...` / `go vet ./...` / `go test ./...` | ✅ 全过（新增 `internal/web/reviews_test.go` 2 例,缺陷 #1 回归）|
| `tsc --noEmit`（`npm run lint`） | ✅ |
| `npm run build` | ✅ |
| bundle 体积（前 → 后） | JS **1,633.44 kB → 947.83 kB**（gzip 528.50 → 303.31，−42%）;CSS 42.96 → 40.98 kB |
| ladder 无数据字段显 `—` | ✅（代码级;运行验收待本地 PG/服务）|
| 笔记筛选出「信念/案例」 | ✅（`DOC_TYPE_LABEL` 补全）|
| `/api/v1/reviews` 出 mistakes | ✅（`toAPIReview` 单测断言,旧实现必失败）|
| 死类零残留 / 无硬编码浅色渐变 | ✅（grep 断言）|

## 偏差与遗留
- 运行验收（`curl /api/v1/reviews`、ladder 页面）需本地 PG(:5433)+ 服务;本阶段以纯函数单测 + 构建断言覆盖,联调并入 P6 端到端走查。
- `globals.css` 行数 1,735 → ~1,150（非设计预估的 400）:保留了全部活类与 `.pipe/.gauge/.spark` 等系统页可视化词表（删它们会破坏 `dashboard` 页）,只删真正死类。视觉统一的落点在 token 归一与渐变改造,不在行数。
- 内联样式未清零:保留少数语义型内联（如 `gauge-val` 的 `color`、`panels` 的 `chg` 分色）,属动态计算值,不宜类化。
