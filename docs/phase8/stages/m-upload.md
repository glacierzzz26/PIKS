# P8 手机截图投递页 `/m/upload` —— 实现与验收

> 阶段:前端增量(单阶段,零后端改动)。起因:用户同花顺在手机上,原先必须先传图到电脑再导入,
> 「导入图片很麻烦」。方案=手机优先的极简投递页,**不做全站移动适配**。

## 交付

**全部为前端改动。零 Go 改动、零 schema、零迁移**(复用既有 `POST /api/v1/trades/import` 与 `/confirm`)。

### 1. 抽共享状态机(消除两份重复逻辑)
- `hooks/useTradeImport.ts`(新):`kind/file/busy/err/msg/preview` + `upload/confirm/reset/patch/toggleRemoveAll`,
  并导出 `isConfigError`。桌面与手机共用同一套语义(含自选「默认勾选移出」)。
- `components/trades/ImportFlow.tsx`(改):改为消费该 hook,**164 → 114 行**(原本已破 150 行红线),
  JSX 与行为不变。桌面端零回归。

### 2. 手机端组件
- `lib/image.ts`(新):`checkImageType` 白名单校验 + `shrinkForVision` canvas 降采样(长边 ≤2000 转 JPEG)。
  **必需项**:后端 `tradeMaxUpload = 5MB`(`internal/web/trades.go:21`),iPhone 截图随手就顶到。
  解码失败(如 HEIC)原样返回,交后端如实报错 —— 不静默吞。
- `components/trades/PhonePick.tsx`(新):类型三选一 +「拍照」(`capture="environment"`)/「从相册选」(无 capture)
  两个入口 + 缩略图 + 开始识别。缩略图 URL 在换图/卸载时 `revokeObjectURL`。
- `components/trades/PhonePreview.tsx`(新):**卡片式**核对(每笔一张卡,行内可编辑 + 勾选)。
  不用 8 列宽表 —— 窄屏下宽表必须横滑、列头与行对应关系丢失,恰是「核对数字」最易漏错位的方式。
  数值输入 `inputMode` 弹数字键盘;`keep` 的自选行无可编辑项(后端对其不做任何事),如实只展示。
- `pages/m/upload.tsx`(新):页面骨架,含成功/出错提示条与自选汇总。

### 3. 路由:无路径 layout route(关键)
- `App.tsx`(改):原方案「并列第二个 `<Routes>`」会让 AppShell 内的 `*` 兜底与移动页**同时命中**,
  手机上会漏出 260px 侧栏。改为 `<Route element={<ShellLayout/>}>` 包住全部桌面页(`<AppShell><Outlet/></AppShell>`),
  `/m/upload` 作为**兄弟路由挂外层** —— v6 全局路由排名中静态段胜出,不依赖声明顺序。
  另加 `/m` → `/m/upload` 重定向(手机上少打路径)。

### 4. 样式:仅此页作用域
- `globals.css`(改):末尾追加 `.m-upload` 前缀块。`100dvh`(非 `100vh`,iOS 地址栏会咬底部)、
  输入框 `font-size:16px`(**防 iOS 聚焦整体放大**)、底部 sticky 操作条不透明 + `env(safe-area-inset-bottom)`、
  触摸目标 ≥44px。**不新增/修改任何全局 media query**;该页在 AppShell 之外,`.side-nav` 一行未动。

### 5. 冒烟脚本
- `scripts/m_upload_check.mjs`(新):iPhone 13 上下文,导入链路用 `page.route` 打桩(后端真实调用受外部
  网关额度影响),独立验证渲染/交互/布局。
- `scripts/e2e_check.mjs`(改):`PAGES` 加一行 `/m/upload` 进全站回归网。

## 验证

- `tsc --noEmit` + `vite build` 通过。
- **手机页冒烟 19/19**:路由隔离(`side-nav=0`)、三态、双入口、缩略图、卡片可编辑、确认后成功提示、
  自选「整组取消移出」、全程无横向滚动、零 console 错误。
- **全站 e2e 回归 29/29**:含 `/trades` 桌面导入页(路由重构无回归)。
- **视觉能力实测**(lab 网关 `deepseek/deepseek-v4.1-flash`):接受 `image_url`,真路径
  (`response_format: json_object`、不设 `max_tokens`)返回合法 JSON、`finish_reason=stop`。
- ⚠️ **本机 dev 真实识别未闭环**:本地 dev 走 zen(`opencode.ai/zen/go/v1`),月额度已耗尽报
  `429 GoUsageLimitError`;前端正确渲染为错误提示条**未白屏**。happy path 用网络 mock 验的前端渲染,
  真机生产端需实跑一次确认。

## 运维要点

- **配置前置**:`/settings` 的「视觉模型」填 `deepseek/deepseek-v4.1-flash`。生产此前 `ai_model_vision`
  为空 → 导入直接 400(`api_write.go:382`)。
- **改完不用重启 web**:截图导入路径每请求现读库(`api_write.go:376` 的 `ListAppConfig`),
  与启动时缓存的 `ApplyAppConfig`(pipeline 用)不同。
- **文案不一致(未修)**:设置页 hint 写「视觉模型留空回退抽取模型」(`AiServiceForm.tsx:77`),
  但 `api_write.go:382` 与 `chat.go:35` 都是当「未配置」直接拒绝、**并无回退**。改动会牵连 `/chat`,暂留待定。
- **入口**:不做二维码/主屏图标。手机浏览器手动输一次 `http://192.168.0.202:8090/m/upload` 后自行收藏。
- 明文 HTTP 可用:file input 的 `capture` 不属 Secure Context 限制(受限的是 `getUserMedia`),无需为移动页配 HTTPS。
