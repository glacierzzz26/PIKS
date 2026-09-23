package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"piks/internal/config"
)

// ============================================================
// 访问控制(设计 docs/phase12/design/access-control.md;issue #78)
//
// 单密码登录 + HMAC-SHA256 签名会话 token,覆盖全部 /api/*(白名单式:
// healthz 与 auth/login 是仅有的两个免鉴权端点,其余一律经 requireAuth)。
// 会话 60 分钟滑动续期(活跃即续,闲置满 TTL 才失效)。
//
// token 形态:base64url(exp|nonce)|base64url(HMAC-SHA256(secret, "exp|nonce"))
// 自签自验,无需服务端存储;改 PIKS_AUTH_SECRET 即令全部既有 token 失效(全量踢出)。
// ============================================================

const (
	authCookieName = "piks_auth"
	// renewWindow:剩余寿命低于 TTL 的这个比例时重签(滑动续期)。
	// 0.5 × TTL = 30min(默认 60min TTL),即约每 30 分钟一次 Set-Cookie。
	renewWindowRatio = 0.5
	// clockSkew 容许前后 60s 时钟偏差(边缘/lab 同源,基本用不上,防御性)。
	clockSkew = 60 * time.Second
)

// AuthToken 校验结果。authToken 中间件在续期时回写 cookie 头。
type authState struct {
	ok    bool
	exp   time.Time // 当前 token 的到期时刻
	renew bool      // 是否应滑动续期
}

// signToken 由过期时刻签发一枚 token。
func (s *Server) signToken(exp time.Time) string {
	payload := strconv.FormatInt(exp.Unix(), 10) + "|" + randomNonce()
	mac := hmac.New(sha256.New, []byte(s.cfg.AuthSecret))
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)
	return b64([]byte(payload)) + "." + b64(sig)
}

// verifyToken 验签 + 校验未过期(含时钟偏差余量)。
func (s *Server) verifyToken(tok string) authState {
	dot := strings.IndexByte(tok, '.')
	if dot <= 0 {
		return authState{}
	}
	payloadB, err := b64dec(tok[:dot])
	if err != nil {
		return authState{}
	}
	sigB, err := b64dec(tok[dot+1:])
	if err != nil {
		return authState{}
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.AuthSecret))
	mac.Write(payloadB)
	if subtle.ConstantTimeCompare(sigB, mac.Sum(nil)) != 1 {
		return authState{}
	}
	parts := strings.SplitN(string(payloadB), "|", 2)
	if len(parts) != 2 {
		return authState{}
	}
	expUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return authState{}
	}
	exp := time.Unix(expUnix, 0)
	now := time.Now()
	if now.After(exp.Add(clockSkew)) {
		return authState{} // 已过期
	}
	ttl := s.authTTL()
	renew := exp.Sub(now) < time.Duration(float64(ttl)*renewWindowRatio)
	return authState{ok: true, exp: exp, renew: renew}
}

func (s *Server) authTTL() time.Duration {
	min := s.cfg.AuthTTLMin
	if min <= 0 {
		min = 60
	}
	return time.Duration(min) * time.Minute
}

// preSharedOK 校验可选的预共享长期 token(脚本/部署核对用;空 = 该路径关闭)。
// 用定长比较防时序泄漏。
func (s *Server) preSharedOK(tok string) bool {
	want := s.cfg.AuthToken
	if want == "" || tok == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(tok), []byte(want)) == 1
}

// credential 从 cookie 或 Authorization: Bearer 取候选 token(两处同值,不分叉)。
func credential(r *http.Request) string {
	if c, err := r.Cookie(authCookieName); err == nil && c.Value != "" {
		return c.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// requireAuth 包装全部需鉴权的处理器。预共享 token 直接放行;否则验签 + 滑动续期。
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := credential(r)
		if s.preSharedOK(tok) {
			next(w, r)
			return
		}
		st := s.verifyToken(tok)
		if !st.ok {
			w.Header().Set("WWW-Authenticate", `Cookie realm="piks"`)
			apiErrJSON(w, http.StatusUnauthorized, "未登录")
			return
		}
		// 滑动续期:重签并覆盖 cookie(浏览器)/ 下发 X-Auth-Token(Bearer 客户端可读回)。
		if st.renew {
			s.issueSession(w, r, true)
		}
		next(w, r)
	}
}

// issueSession 签发会话:写 Set-Cookie(浏览器)+ X-Auth-Token(Bearer 客户端)。
// clear=true 时清空 cookie(登出用)。
func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, renew bool) {
	if !renew {
		// 登出:清 cookie。
		http.SetCookie(w, &http.Cookie{
			Name: authCookieName, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isSecure(r),
		})
		return
	}
	exp := time.Now().Add(s.authTTL())
	tok := s.signToken(exp)
	http.SetCookie(w, &http.Cookie{
		Name: authCookieName, Value: tok, Path: "/",
		MaxAge: int(s.authTTL().Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isSecure(r),
	})
	// Bearer 客户端(无 cookie 自动覆盖)可读回续期后的 token。
	w.Header().Set("X-Auth-Token", tok)
}

// isSecure:生产经 HTTPS(边缘终结 TLS,回源 HTTP)—— X-Forwarded-Proto=https 视为安全,
// 否则按请求是否为 TLS。dev 直连 http 时 Secure 关闭,便于本地 cookie 调试。
func isSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// ==================== 端点 ====================

// POST /api/v1/auth/login {password} —— 单密码登录。失败一律 401「密码错误」,
// 不区分「未配置」与「密码错」(避免探测);成功下发 cookie + X-Auth-Token。
func (s *Server) authLoginAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiErrJSON(w, http.StatusMethodNotAllowed, "仅支持 POST")
		return
	}
	if !s.loginLimiter.allow(ipKey(r)) {
		apiErrJSON(w, http.StatusTooManyRequests, "尝试过于频繁,请稍后再试。")
		return
	}
	var p struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		apiErrJSON(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(s.cfg.AuthPasswordHash), []byte(p.Password)) != nil {
		apiErrJSON(w, http.StatusUnauthorized, "密码错误")
		return
	}
	s.issueSession(w, r, true)
	s.writeJSON(w, map[string]any{"ok": true})
}

// POST /api/v1/auth/logout —— 幂等:无 cookie 也 200。
func (s *Server) authLogoutAPI(w http.ResponseWriter, r *http.Request) {
	s.issueSession(w, r, false)
	s.writeJSON(w, map[string]any{"ok": true})
}

// GET /api/v1/auth/me —— 前端鉴权门探活。走到这里必已鉴权(中间件拦未登录)。
func (s *Server) authMeAPI(w http.ResponseWriter, r *http.Request) {
	tok := credential(r)
	if s.preSharedOK(tok) {
		s.writeJSON(w, map[string]any{"authed": true})
		return
	}
	st := s.verifyToken(tok)
	s.writeJSON(w, map[string]any{"authed": st.ok, "exp": st.exp.Unix()})
}

// GET /api/v1/healthz —— 免鉴权存活探针(compose healthcheck 用)。
// ⚠️ 不查库、不触 LLM:上鉴权后原先打 /api/v1/dashboard 的 healthcheck 会 401 →
// web 永不 healthy → gateway depends_on 卡死 → 部署挂死(设计 §4.4)。
func (s *Server) healthzAPI(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, map[string]any{"ok": true})
}

// ==================== 小工具 ====================

// budgetExhausted 判断今日 AI 预算是否已用尽(P12 补漏 + issue #75 口径)。
//
// ⚠️ 口径与既有 weekly/trades 检查**逐字一致**:`ai_daily_token_budget > 0` 才检查,
// **0 = 护栏关闭**(不是「不限预算」,#75 D)。口径若变,这些调用点要一起改。
// 此前 /chat 与 /research-runs **完全没有**这道闸(设计 §1.3 核实的钱洞)。
func (s *Server) budgetExhausted(ctx context.Context) bool {
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		return false // 读配置失败不误拦(与既有调用点同:err != nil 即放行)
	}
	budget, _ := strconv.ParseInt(cfgMap["ai_daily_token_budget"], 10, 64)
	if budget <= 0 {
		return false
	}
	today, err := s.store.TokensSince(ctx, config.BeijingMidnight(time.Now()))
	return err == nil && today >= budget
}

// llmGuard 是 LLM / 重活端点的统一入口闸:先限流(per-IP),再查预算。
// 通过返回 true;否则已写好 429 响应,调用方直接 return。
func (s *Server) llmGuard(w http.ResponseWriter, r *http.Request) bool {
	if !s.llmLimiter.allow(ipKey(r)) {
		apiErrJSON(w, http.StatusTooManyRequests, "请求过于频繁,请稍后再试。")
		return false
	}
	if s.budgetExhausted(r.Context()) {
		apiErrJSON(w, http.StatusTooManyRequests, "今日 AI 预算已用尽,请稍后再试(预算恢复后自动可用)。")
		return false
	}
	return true
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func b64dec(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }

// randomNonce 每次签发一个随机串,令同秒内重复签发也产出不同 token。
func randomNonce() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 极端情况下退回纳秒时间戳(仍唯一,不致命)。
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// ipKey 取限流键:优先 X-Real-IP(nginx 已设),回退 RemoteAddr 的 host。
func ipKey(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rateLimiter 内存令牌桶(单进程;web 单实例足够,不做分布式)。
// per 窗口秒数内允许 n 次;超限拒。键为 IP。
type rateLimiter struct {
	mu     sync.Mutex
	per    time.Duration
	n      int
	hits   map[string][]time.Time
	lastGC time.Time
}

func newRateLimiter(n int, per time.Duration) *rateLimiter {
	return &rateLimiter{per: per, n: n, hits: map[string][]time.Time{}, lastGC: time.Now()}
}

// allow 记录一次访问并判定是否放行(滑动窗口)。
func (l *rateLimiter) allow(key string) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	// 惰性 GC:每次约一分钟清一次过期条目,防 map 无界增长。
	if now.Sub(l.lastGC) > l.per {
		for k, ts := range l.hits {
			kept := ts[:0]
			for _, t := range ts {
				if now.Sub(t) < l.per {
					kept = append(kept, t)
				}
			}
			if len(kept) == 0 {
				delete(l.hits, k)
			} else {
				l.hits[k] = kept
			}
		}
		l.lastGC = now
	}
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.per {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.n {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
