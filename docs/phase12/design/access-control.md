# 访问控制（应用层鉴权 + 预算护栏）

> 阶段:**P12 安全加固** · 出处:issue **#78**(公网暴露无鉴权) · 关联:#75(预算护栏)、#76(重跑放大)
> 分支:`feature/issue78` · 上游记录:PR **#79**(公网暴露拓扑,纯文档)、`docs/架构总览.md` §9.5
>
> **本文范围**:给 PIKS 全部 `/api/*` 加**应用层鉴权**(登录 → 会话 → 中间件),并**同一 PR 内**堵住
> 公网状态下可被任意人触发的 LLM 计费洞(预算闸 + 限流)。
> **本文不覆盖**:边缘 Nginx 的 TLS/SNI 配置(在阿里云 `/root/host-infra`,非本仓库)、多用户/权限分级
> (个人系统,单用户)。§9 列出明确的**非目标**。

---

## 0. 一句话结论

在 **Go 应用层**(而非边缘/网关)加一道**单密码登录 + 签名会话 cookie**的鉴权门,覆盖
**全部 `/api/*`**;会话为 **60 分钟滑动续期**(活跃即续,闲置满 60 分钟才失效)。同时给
`/chat`、`/research-runs` 补上缺失的 `ai_daily_token_budget` 检查、
给 LLM 端点加 per-IP 限流,并把生产预算闸从当前的 `0`(护栏关闭)改为非 0。
落点选应用层而非仅边缘,因为 lab 的 `piks-gateway` 是 `0.0.0.0:8090`(**局域网可绕过边缘直连**),
且应用层**进仓库、可版本化、可测试**。

---

## 1. 问题陈述(核实过的事实,非转述)

### 1.1 全链路零鉴权

- `internal/web/server.go` 的 `Routes()` 把全部处理器**裸挂** `http.ServeMux`,无任何中间件;
  Go 侧全仓无 `Authorization` / `Bearer` / `BasicAuth` 处理。
- `cors()` 中间件对 `/api/v1/` 前缀设 `Access-Control-Allow-Origin: *`(`server.go:74`)。
- 公网链路(2026-09-22 建):公网 `https://piks.5home.online` → 阿里云边缘 Nginx(443, SNI)→ frp stcp
  隧道 → lab `piks-gateway:8090`。**边缘 Nginx 是唯一公网入口**(回源口 17010 只绑 loopback),但
  **它和应用层都没有鉴权**。

### 1.2 暴露面比 issue 正文更宽一条

`configs/docker-compose.prod.yml:43` 的 gateway 发布 `0.0.0.0:8090:80` —— **局域网内任何人可直连
lab:8090,绕过阿里云边缘那层**。因此「只在边缘加锁」不足(见 §3.1 决策)。

### 1.3 🔴 比 issue 描述更严重:钱洞(核实)

issue 第 2 条待办写「补鉴权前须确认 `ai_daily_token_budget` 闸已生效」——**该前提当前不成立**:

- 生产 `app_config.ai_daily_token_budget = 0`(2026-09-22 深夜栈 `v0.0.0-e19742a`,只读 `psql` 确认)。
  按 #75 语义 **0 = 护栏关闭**(不是「不限预算」)。→ 闸根本没开。
- **`/chat` 与 `/research-runs` 处理器里压根没有预算检查**:
  - `internal/web/api_research.go` 无 `budget` 任何引用;
  - `internal/web/chat.go` 无 `budget` 任何引用;
  - 只有 `weekly`(`weekly.go:202`)、`trades`(`trades.go:451,820`)、`api_write.go:401` 有。
- 结论:任何人都能反复调 `/chat`(真实 LLM 问答)或 `/research-runs`(**触发整条 Python 深研管线 +
  LLM 合成**)且**无预算上限**。仅靠加鉴权只是把洞收窄(凭据一泄照样烧),**必须同 PR 补预算 + 限流**。

---

## 2. 目标 / 非目标

### 目标
1. 未登录者**无法**读写任何 `/api/*`(读数据、写数据、触发 LLM/管线一律 401)。
2. 登录一次,**活跃期间一直免重复登录**;**闲置满 60 分钟**后失效(滑动续期,见 §3.4)。
3. 覆盖**全部到达路径**:公网(边缘)、局域网直连 :8090、容器私网 —— 因为门在应用层。
4. LLM 与重活端点有**预算上限 + 频率上限**,即便凭据泄露也烧不穿日预算。
5. 登录页**美观、贴合现有设计系统**(`globals.css` 语义类 + token),不引新 UI 库。

### 非目标(明确不做)
- 多用户 / 角色 / 权限分级(个人系统,单用户单密码)。
- 用户名(用户已定:**单密码**)。
- 全站 TLS 终结改造(边缘已 HTTPS,应用层只跑 HTTP,由边缘负责 TLS)。
- 边缘 Nginx 改动(不在本仓库;若后续要纵深防御,见 §9 遗留)。
- 2FA、OAuth、密码找回(单密码,无账户体系)。

---

## 3. 方案决策

### 3.1 落点:应用层,而非仅边缘/网关

| 候选 | 覆盖公网 | 覆盖 LAN 直连 | 进仓库 | 登出/UX | 工作量大 |
|---|---|---|---|---|---|
| **A 应用层 Go 中间件**(选定) | ✅ | ✅ | ✅ | ✅ | 中 |
| B 网关 Basic Auth | ✅ | ✅ | ✅ | ❌ | 小 |
| C 仅边缘 Basic Auth | ✅ | ❌ | ❌(在阿里云) | ❌ | 小 |
| D = A + C 双锁 | ✅ | ✅ | 部分 | ✅ | 中 |

**选 A**。理由:LAN 直连口(§1.2)是选 A 的决定性因素 —— B/C 都盖不住(除非同时改边缘,而 C 的配置
不在仓库)。A 还能顺带收紧 CORS、加限流、给前端一个体面的登录页。

### 3.2 会话凭证:HMAC 签名 token(cookie + Bearer 两用)

- 登录成功后由服务端签发一个 **HMAC-SHA256 签名的 token**,payload = `<exp>|<nonce>`,
  签名密钥 = `PIKS_AUTH_SECRET`(lab `.env`,不进 git)。
- **无服务端会话存储**:验签 + 查 `exp` 即可,天然支持多实例、重启不失联(除密钥轮换)。
- 下发方式:**`HttpOnly` + `Secure` + `SameSite=Lax` cookie**(浏览器),**同值**也接受
  `Authorization: Bearer <token>`(脚本/手机/健康检查)。cookie 与 Bearer 走**同一校验函数**,不分叉。
- **令牌轮换 / 失效**:改 `PIKS_AUTH_SECRET` 即令所有既有会话立即失效(全量登出),作为「一键踢出」手段。
- **为何不用 JWT 库**:payload 仅 `exp|nonce`,自签自验 `HMAC`+`subtle.ConstantTimeCompare` 约 20 行,
  不值当引 JWT 依赖;换成 JWT 无收益。

### 3.3 密码:bcrypt 哈希(用户已定)

- 引 `golang.org/x/crypto/bcrypt`;**库内只存哈希**,明文永不入库/入档。
- `PIKS_AUTH_PASSWORD_HASH`(bcrypt `$2a$...`)存 lab `.env`(0600);校验用
  `bcrypt.CompareHashAndPassword`(自带定长比较)。
- 备一个生成工具命令 `cmd/hashpw`(**仅本地用**,输出 `$2a$...` 供写入 `.env`),不参与部署服务。
- ⚠️ **未配置即拒绝启动**(fail-closed):`PIKS_AUTH_PASSWORD_HASH` 或 `PIKS_AUTH_SECRET` 缺失时
  `cmd/web` **fatal 退出**,绝不「无配置=放行」。这是本设计唯一的硬安全默认。

### 3.4 超时:60 分钟**滑动续期**(用户已定)

- `exp = 签发时刻 + 60min`,写进 token payload;校验时 `now < exp` 才放行。
- **滑动续期**:每个**经鉴权通过**的请求,若 token 剩余寿命 < 阈值(取 `0.5 × TTL = 30min`),
  服务端**收回并重签**一枚 `exp = now + 60min` 的新 token,经 `Set-Cookie` 覆盖浏览器里的旧值
  (响应头即可,无需改 handler 返回值)。→ 活跃用户**永不掉线**;闲置满 60 分钟才需重登。
- **续期节流**:不是每个请求都重签 —— 只在「剩余 < 30min」时才签,故正常使用下约每 30 分钟一次
  `Set-Cookie`,开销可忽略。
- **Bearer 客户端**(脚本/手机):无 cookie 可覆盖,无法自动续期 —— **要么每次重登**,
  **要么**从响应头 `X-Auth-Token` 读回续期后的 token 自行保存(§4.2 约定同时下发,脚本友好)。
  预共享 `PIKS_AUTH_TOKEN`(§4.2)则不受 TTL 约束,供长期脚本用。
- **过期即 401**,前端跳登录页并带 `?next=<原路径>`。token 内 `exp` 用 Unix 秒;允许 ±60s 时钟偏差余量
  (边缘/lab 时钟同源,基本用不上,防御性)。
- ⚠️ **续期不改变「改密钥即全量踢出」**:`PIKS_AUTH_SECRET` 一换,所有旧 token(含续期后的)全失效。

### 3.5 nginx:`Set-Cookie` 透传(核实结论:无需改动)

`configs/nginx.conf` 的 `/api/` location 未设 `proxy_buffering`(默认开)。**核实:nginx 的
`proxy_buffering` 缓冲的是响应 **body**,响应头(含 `Set-Cookie`)始终即时转发**;且未启用
`proxy_cache`。故登录/续期的 `Set-Cookie` 逐字到达浏览器,**无需改 nginx 配置**。
部署验收(§8 #5/#7b)会实测登录响应确含 `Set-Cookie` 以坐实此结论(防「默认行为」判断有误)。

---

## 4. 后端设计

### 4.1 中间件与路由树

```
Routes():
  mux.HandleFunc("/api/v1/healthz", …)            # 免鉴权(见 §4.4)
  # ↓ 以下全部经 requireAuth 包装
  mux.HandleFunc("/api/v1/auth/login", …)          # 免鉴权(登录本身)
  mux.HandleFunc("/api/v1/auth/logout", …)         # 需鉴权(幂等:无 cookie 也 200)
  mux.HandleFunc("/api/v1/auth/me", …)             # 需鉴权(前端探活)
  …既有全部 /api/* …                                # 需鉴权
```

实现取舍:`requireAuth` 包**整个 `/api/*`**(含旧 `/api/graph`、`/api/attachments/`),仅把
`healthz` 与 `auth/login` **登记在包装之外**。这样新增接口默认被保护(白名单式),而非默认裸奔。

### 4.2 `internal/web/auth.go`(新增,单一真源)

职责:
- `requireAuth(next)` —— 解析 cookie 或 `Authorization: Bearer`;命中**预共享** `PIKS_AUTH_TOKEN`
  (`subtle.ConstantTimeCompare`)直接放行;否则验签 + 查 `exp`。失败 `401 {"error":"未登录"}` 且带
  `WWW-Authenticate: Cookie`(便于排查)。
- `signToken(exp) (string, error)` / `verifyToken(tok) (exp, ok)` —— HMAC-SHA256,自签自验
  `subtle.ConstantTimeCompare`。
- **滑动续期**(§3.4):中间件在验签通过后,若 `exp - now < 30min` 则重签并 `Set-Cookie` 覆盖,
  同时写响应头 `X-Auth-Token: <new>`(Bearer 客户端可读回;cookie 客户端忽略)。
- `LoginAPI` —— `POST {password}` → bcrypt 校验 → `Set-Cookie: piks_auth=<tok>; HttpOnly; Secure;
  SameSite=Lax; Path=/; Max-Age=3600` + `X-Auth-Token` → `200 {ok:true}`;失败 `401`(不区分
  「密码错」与「未配置」的外部文案,均「密码错误」,避免探测)。
- `LogoutAPI` —— `Set-Cookie: piks_auth=; Max-Age=0` → `200`(幂等)。
- `MeAPI` —— `200 {authed:true, exp:<unix>}`;未鉴权由中间件拦成 401(不进此函数)。

> **预共享 `PIKS_AUTH_TOKEN`**(可选,不进 git):与 `PIKS_AUTH_SECRET` 同性质的长随机串,供
> **deploy.sh 核对 / e2e 脚本**长期免登录使用(**不受 TTL 约束**,不参与续期)。缺失则该路径关闭,
> 脚本改用「先 login 拿 token」。

### 4.3 与既有 CORS 的冲突处理(关键)

`cors()` 现在的 `Access-Control-Allow-Origin: *`(**server.go:74**)与**带凭据的跨域请求天然冲突**:
浏览器对 `credentials: "include"` 的请求**不接受** `*`(必须回显具体 Origin + `Allow-Credentials: true`)。
但生产是**同源单入口**(nginx 反代 `/api`,浏览器只跟 `piks.5home.online` 说话),dev 走 vite proxy
**也是同源** —— 这个 `*` 是早期 `:3100 → :8090` 跨端口时代的遗留,**现已多余**。

**决策**:**去掉 `*`,改同源**(不再设 `Access-Control-Allow-Origin`)。若 dev 需跨端口,由 vite proxy
复刻同源(现状即如此)。`OPTIONS` 兜底保留(同源下不触发,但无害)。

> 影响面:去掉 `*` 后,**任何从其它 Origin 用浏览器直连 API 的用法会失效** —— 现有代码/脚本无一如此
> (脚本走 curl,不看 CORS)。§5 需 grep 确认无残留依赖。

### 4.4 `healthz`(部署死锁的解药,必须)

**问题**:`configs/docker-compose.prod.yml:69` 的 web healthcheck 打的是
`http://127.0.0.1:8090/api/v1/dashboard`。一旦 `/api/*` 上鉴权,该请求 401 → 容器**永远 not healthy**
→ `gateway` 的 `depends_on: web: condition: service_healthy` **永远不满足 → gateway 起不来 →
部署挂死**(与 #55 gateway 启动挂死同一类事故)。

**对策**:新增 **免鉴权** `GET /api/v1/healthz` → `200 {"ok":true}`(**不查库、不触 LLM**,纯存活探针),
把 compose healthcheck 指过去。`deploy.sh:228` 的核对 curl 同步更新(见 §7)。

### 4.5 预算闸补漏(#78 §1.3 的钱洞)

复用**既有口径**(不得新造语义):读 `cfgMap["ai_daily_token_budget"]`,`> 0` 才检查
`s.store.TokensSince(ctx, config.BeijingMidnight 今日)` ≥ budget 即拒;**`0` 仍 = 护栏关闭**(#75 语义不变)。

| 端点 | 现状 | 改动 |
|---|---|---|
| `/api/v1/chat` | ❌ 无检查 | 补:超预算 → `429 {"error":"今日 AI 预算已用尽"}` |
| `/api/v1/research-runs`(POST 触发) | ❌ 无检查 | 补:同上(触发即入队,拒在入口) |
| `/weekly/generate`、`/trades` 解读 | ✅ 已有 | 不动 |

⚠️ 与 #75 一致:改预算行为须同改 `docs/phase2/design/cluster-quality.md` §3.4 的记账口径描述(如有)。

### 4.6 LLM/重活端点限流(per-IP 令牌桶)

- 一个内存 `map[ip]*bucket`,令牌桶:**每 IP 对 LLM 端点 X req/min**(具体值 §5 标定,初拟
  `chat` 30/min、`research-runs` 10/min),桶满 `429`。
- 另一独立桶给 **`/auth/login`:5 次/分钟**(防在线爆破)。
- 单进程内存即可(web 单实例);不做分布式限流(非目标)。IP 取 `X-Real-IP`(nginx 已设,`nginx.conf`)
  回退 `RemoteAddr`。
- ⚠️ **公网链路上「per-IP」会退化为「全局」**:入站走 **frp stcp 隧道**,`piks-gateway` 只看到隧道
  客户端(同一 loopback 源),故公网来的请求共享一个限流键 —— 30/min 实际是「公网全体共享 30/min」。
  单人使用下可接受(反而更严),但**不要**据此认为「攻击者会被单独限流」。要真正按公网真实 IP 分流,
  需边缘 Nginx 透传真实 IP、frp 侧保真 —— 属未做的加固(§9)。`/auth/login` 的爆破防护**不依赖**此键:
  真正的门槛是 bcrypt + 单口令,限流只是叠加。

### 4.7 配置项汇总(全部在 lab `.env`,不进 git)

| 变量 | 必填 | 作用 |
|---|---|---|
| `PIKS_AUTH_PASSWORD_HASH` | ✅ | bcrypt 哈希,登录比对;缺失 → web fatal |
| `PIKS_AUTH_SECRET` | ✅ | HMAC 签名密钥;缺失 → web fatal |
| `PIKS_AUTH_TTL_MIN` | ❌ | 会话分钟数,默认 `60`(滑动续期窗口) |
| `PIKS_AUTH_TOKEN` | ❌ | 预共享长期 token,供脚本/部署核对;不设=该路径关闭 |

`configs/.env.prod.example` 同步补三项(占位值);`deploy.sh` 无需读(值在 lab `.env`,compose 经
`env_file` 注入)。**改为 `cmd/web` fatal 前须确认 lab `.env` 已写入这两个值**,否则部署后 web 起不来
(§7 顺序约束)。

---

## 5. 前端设计

### 5.1 401 收敛点(单点改造)

`frontend/src/lib/api.ts` 的 `request()` 是**唯一** fetch 出口(全站 `apiGet/apiPost/…` 皆经它):

- 加 `credentials: "include"`(带上 cookie)。
- 响应 `401` → 抛一个**可识别的错误**,并**跳 `/login?next=<当前 path+search>`**(用
  `window.location` 或注入的 navigate;SPA 内用 `react-router` 的 `redirect`,§5.4)。
- **滑动续期对前端透明**:续期靠响应头 `Set-Cookie`,浏览器自动落盘 / 覆盖,前端**无需**拦截或保存
  任何 token;无需改动 `apiGet` 之外的调用点。

### 5.2 鉴权门(首屏判定)

`App.tsx` 顶层包一个 `AuthGate`:首屏 `GET /api/v1/auth/me` →
- `200` → 渲染正常路由;
- `401` → `Navigate to="/login?next=…"`(不闪内容);
- 加载中 → 极简占位(不引 skeleton,遵守「核心数字区禁 skeleton」;此处非数字区)。

`/login` 作为**无壳路由**(与 `/m/upload` 同级,`ShellLayout` 之外),登录页天然无侧栏。

### 5.3 登录页(`frontend/src/pages/login.tsx`,新增)

**美观、贴合现有 UI** 是硬要求(用户明确)。做法:

- **完全复用设计系统**,不引任何新 UI 库:
  - 容器 `.form-card`(圆角 `--radius-card`、`--shadow`、`--line` 边框);
  - 标题 + `.fnote` 副说明;输入 `.frow > input`(带 focus ring `--accent-soft`);
  - 提交按钮 `.btn-save`(**品牌渐变** `--brand-a → --brand-b` + `--shadow-lg`)。
- **品牌感**:页面居中卡片,顶部一枚品牌标记(渐变背景 + Lucide 图标,禁 emoji),标题「PIKS · 登录」,
  副标题白话(如「这是你的个人投研库,先登录再进」)。
- **三态**(遵守规范第 9 条):提交中(按钮禁用 + 「登录中…」)、错误(`401` → 行内错误条,不弹窗)、
  成功(跳 `?next` 或 `/`)。
- **暗色**:全部走 token,`[data-theme="dark"]` 自动生效,零额外样式。
- 视觉细节(圆角/间距/渐变)一律**用现有 token 与语义类**,若确需一处新样式,加进
  `globals.css`(不散落魔法值,遵守「3 档圆角」)。

### 5.4 登出入口

侧栏(`components/layout/SideNav.tsx`)底部加「退出登录」→ `POST /api/v1/auth/logout` → 跳 `/login`。
(或并入设置页;§5 实现时择一,倾向侧栏底部常驻。)

---

## 6. 不做 / 已知边界

- **CSRF**:cookie 用 `SameSite=Lax`,且写接口不吃表单跨站提交(`Content-Type: application/json`),
  风险低;**不**单独加 CSRF token(如后续引入 `SameSite=None` 须重评)。
- **会话续期**:滑动续期(§3.4);闲置满 60 分钟硬过期,过期重登。不做「记住我」永久会话。
- **多设备登出**:改 `PIKS_AUTH_SECRET` 全量踢;不维护单会话撤销列表。
- **IP 白名单**:可作为边缘层的额外手段(§9),不在本 PR。
- **不保护 SPA 静态壳**:`/`、`/login`、`/assets/*`、`index.html` 放行。壳内**无数据**(数据全在 `/api/*`),
  放行不泄露任何内容;拦住反而让登录页无法加载。**门在 API 层,不在静态层**。

### 6.1 静态壳放行与 `/m/upload`(手机投递页)的交互

`/m/upload` 是独立无壳路由,其**数据操作走 `/api/v1/trades/import|confirm`(已鉴权)**。手机上首次
使用会被 401 → 跳 `/login` → 登录后回投递页。⚠️ **手机登录页必须可用**:§5.3 登录页不依赖侧栏/命令面板,
移动端宽度下卡片自适应(用布局类 `max-w` + `px`,无需专门样式)。§8 验收含手机宽度实测。

---

## 7. 部署接线(顺序是硬约束)

1. **lab `.env` 先写入** `PIKS_AUTH_PASSWORD_HASH`(用 `cmd/hashpw` 本地生成)+ `PIKS_AUTH_SECRET`
   (**改 web 为 fail-closed 前**必须已写,否则 web 起不来)。
2. `configs/docker-compose.prod.yml`:web healthcheck 改打 `/api/v1/healthz`。
3. `scripts/deploy.sh:228` 核对 curl:`/api/v1/dashboard` → `/api/v1/healthz`(免鉴权才能核对);
   如有其它 8090 探活同理。
4. `configs/.env.prod.example` 补三项占位。
5. `deploy.sh` 的 `TAG_WEB` 闭包:确认 `x/crypto` 进入 `go_deps_hash web`(依赖闭包自动覆盖,§5 复核)。
6. **生产预算**:部署后把 `app_config.ai_daily_token_budget` 从 `0` 改为 `1000000`(运维动作,**不写进代码**;
   口径见 #75)。
7. 边缘(阿里云 `host-infra`,非本仓库):**本次不动**(应用层已覆盖)。

---

## 8. 验收标准(交付时必须逐条给出实测)

| # | 项 | 判据 |
|---|---|---|
| 1 | 未登录读被拒 | `curl https://…/api/v1/dashboard` → **401**(无 cookie) |
| 2 | 未登录写被拒 | `curl -X POST …/api/v1/notes` → **401** |
| 3 | 未登录触 LLM 被拒 | `curl -X POST …/api/v1/chat`、`…/research-runs` → **401** |
| 4 | 密码错被拒 | `POST /auth/login {wrong}` → **401**,且响应不泄露「是否已配置」 |
| 5 | 登录成功 | `POST /auth/login {正确}` → **200** + `Set-Cookie`(`HttpOnly`/`Secure`/`SameSite=Lax`/`Max-Age=3600`) |
| 6 | 带 cookie 可用 | 携 cookie 重放 #1~#3 → **200** |
| 7 | 过期 401 | 伪造/构造 `exp` 已过 token → **401**;或把 TTL 设 1min 实测 |
| 7b | **滑动续期** | 剩余 <30min 时打一请求 → 响应含**新 `Set-Cookie`**(`exp` 已后延)+ `X-Auth-Token`;用新 cookie 继续可用;**闲置满 TTL 后**旧 token 不再续期、401 |
| 8 | 登出失效 | `POST /auth/logout` 后再用旧 cookie → **401** |
| 9 | Bearer 可用 | `-H "Authorization: Bearer <token>"` 等效 cookie;预共享 `PIKS_AUTH_TOKEN` 也放行 |
| 10 | healthz 免鉴权 | `curl …/api/v1/healthz` → **200**,无 cookie |
| 11 | 部署不死锁 | compose `web` healthy、`gateway` 正常起;`docker ps` 全绿 |
| 12 | 预算闸生效 | 预算设小值(如 `10`)→ `/chat`、`/research-runs` → **429**;`0` 时不拦(**语义不变**) |
| 13 | 限流生效 | 高频打 `/auth/login` → 第 6 次 **429**;LLM 端点超阈值 **429** |
| 14 | CORS 收紧 | 去掉 `*` 后同源正常;异源 `credentials` 请求不再被 `*` 误导 |
| 15 | 前端登录页 | 未登录访问任意页 → 跳 `/login`;登录 → 回 `?next`;**深浅色均美观**、移动端宽度可用 |
| 16 | 静态壳放行 | 未登录 `GET /`、`/login`、`/assets/*` → 200(仅壳,无数据) |
| 17 | 回归 | `go build/vet/test`、前端 `tsc --noEmit` + `npm run build`、既有 Playwright 冒烟不回归 |
| 18 | 部署后核对 | `curl …/api/v1/healthz` 200;栈清单登记四镜像 tag;生产预算非 0 |

---

## 9. 遗留 / 后续

- **边缘纵深防御**(可选):在阿里云 `host-infra` 给 piks vhost 再叠一层(IP 白名单 / Basic Auth),
  使 frp 隧道本身也进不来。配置不在本仓库,单列后续。
- **公网真实 IP 透传**(§4.6):现限流按隧道源 IP 聚成一桶(公网全体共享)。要按真实访客 IP 限流,
  需边缘 Nginx 透传 + frp 侧保真;未做。
- **记住我 / 永久会话**:滑动续期(§3.4)已覆盖日常「不用反复登录」;若仍要跨浏览器重启保活,再议。
- **会话撤销列表**:若将来多用户,再引入服务端会话表。
- **`x/crypto` 其他用法**:确认无重复引入(仅 bcrypt)。

---

## 10. 文档同步(随本 PR 一并)

- `docs/架构总览.md`:§9.5 增补「应用层鉴权已落地」;**§329 CORS 描述**(`*`)改为同源;新增鉴权小节。
- `README.md` §生产部署:公网入口处把「⚠️ 当前无鉴权(裸奔)」改为「已加登录鉴权」+ 访问方式。
- `configs/.env.prod.example`:补 `PIKS_AUTH_*` 三项。
- `docs/进度总表.md`:P12 行 + 子阶段明细。
- `frontend/scripts/e2e_check.mjs` / `m_upload_check.mjs`:改用预共享 `PIKS_AUTH_TOKEN` 注入
  `Authorization` 头(否则鉴权后全站页断言集体误报「路由回归」);脚本头补运行说明。
- 本设计文档 `docs/phase12/design/access-control.md`(新增)。
- `docs/phase12/design/README.md`(新增,阶段设计索引,沿用 phase9/10/11 惯例)。
