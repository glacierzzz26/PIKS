# phase12 定稿设计索引

> 阶段:**P12 安全加固**。已定稿设计在此登记,此后按它执行,改动需走变更。

## P12 访问控制(应用层鉴权 + 预算护栏)📝 设计定稿,待实现(2026-09-23)

| 文档 | 状态 | 日期 | 备注 |
|---|---|---|---|
| [access-control.md](./access-control.md) | 📝 **设计定稿,待实现** | 2026-09-23 | issue **#78**(公网暴露无鉴权)。给**全部 `/api/*`** 加 **应用层**单密码登录 + 签名会话 cookie(**60min** 超时);同时**同 PR** 堵住公网状态下可被任意人触发的 **LLM 计费洞**(给 `/chat`、`/research-runs` 补 `ai_daily_token_budget` 检查 + per-IP 限流 + 生产预算从 `0` 改非 0)。**落点选应用层而非仅边缘**:lab `piks-gateway` 发布 `0.0.0.0:8090`,**局域网可绕过边缘直连**,且应用层进仓库可测。**两处必验的坑**:① `docker-compose.prod.yml` 的 web healthcheck 打 `/api/v1/dashboard`,上鉴权后会 **401 → web 永不 healthy → gateway 卡死 → 部署挂**,故新增**免鉴权 `/api/v1/healthz`** 并改 healthcheck;② CORS 的 `Access-Control-Allow-Origin: *`(`server.go:74`)与带凭据跨域**天然冲突**,生产同源单入口下已多余 → **去掉改同源**。密码 **bcrypt**(`x/crypto`),HMAC-SHA256 自签 token(`exp|nonce`,cookie + `Bearer` 两用,改 `PIKS_AUTH_SECRET` 即全量踢出),**未配置即 fatal 启动**(fail-closed)。零 DB schema。 |
