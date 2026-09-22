// Package ths 同花顺(10jqka)私有协议客户端 —— 只为「我的自选」自动同步服务。
//
// ── 为什么自建而不复用 ────────────────────────────────────────────────────
// internal/collector 的 httpSource 面向**无凭据的公开新闻源**(浏览器 UA、无 cookie、
// 走 per-host 令牌桶护栏)。同花顺自选是**持登录态**的私有接口:需要注入整串 cookie、
// 认移动端 UA、且 host 只有 t/ugc/d/auth/upass 五个,用 collector 的护栏反而不合适。
// 故本包自带一份精简的「限频 + 指数退避」,不引 collector 依赖(避免把新闻链路拖进 tools)。
//
// ── 协议事实(2026-09-22 真 cookie 实测)────────────────────────────────────
//
// 取「我的自选」只需两个**无签名 GET**:
//
//	① 名单  GET https://t.10jqka.com.cn/newcircle/group/getSelfStockWithMarket/
//	         → {"errorCode":0,"result":[{"code":"601091","marketid":"17"}, …]}
//	② 元数据 GET https://ugc.10jqka.com.cn/selfstock_detail?reqtype=download&app_flag=0E&userid=<uid>
//	         → XML <ret code="0"><item version="312" selfstock_detail="<base64(JSON)>"/>
//	           JSON = [{"M":"17","C":"601091","P":"37.85","T":"20260922"}, …]
//
// 🔴 **两个接口都不返回股票名** —— 只有 code + marketid + 加入价/加入日。落 PIKS 的
// entities 又必须有名字(UNIQUE(type,name)),故名字走 name.go 的 realhead 端点另取。
//
// 登录(账密)走 auth.go 的 verify2 四步 → docookie2 换 cookie;cookie 里 sess_tk 是 JWT,
// exp ≈ 7 天。**无滑块、无设备指纹**。
//
// ── 本包红线 ──────────────────────────────────────────────────────────────
// ⚫ 只读。绝不调用 modifySelfStock / 任何写接口(自选由同花顺单向镜像,PIKS 不自作主张)。
// ⚫ 不得为「多分组同步」引入 multiStorage/blockstock —— 那是把指数(N225/KS11)灌进
//
//	entities 的入口(见 docs/phase11/design/watchlist-sync.md §3)。
package ths
