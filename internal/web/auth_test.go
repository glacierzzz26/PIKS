package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"piks/internal/config"
)

// newAuthTestServer 构造一个只带鉴权配置的最小 Server(store 为 nil —— 这些用例
// 不触库;requireAuth/login/me/logout/healthz 均不读 store)。
func newAuthTestServer(t *testing.T, pw string) *Server {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	s, err := NewServer(nil, config.Config{
		AuthPasswordHash: string(hash),
		AuthSecret:       "test-secret-0123456789",
		AuthTTLMin:       60,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return s
}

// TestNewServerFailClosed 锁死 P12 的硬安全默认:口令哈希或密钥缺失即拒启动,
// 绝不「无配置=放行」(设计 §3.3)。
func TestNewServerFailClosed(t *testing.T) {
	cases := map[string]config.Config{
		"两者皆空":    {},
		"只有口令哈希":  {AuthPasswordHash: "$2a$10$abcdefghijklmnopqrstuv"},
		"只有密钥":    {AuthSecret: "s"},
		"密钥为空白字符": {AuthPasswordHash: "$2a$10$x", AuthSecret: "   "},
	}
	for name, cfg := range cases {
		if _, err := NewServer(nil, cfg); err == nil {
			t.Errorf("%s: NewServer 未拒绝,应为 fail-closed", name)
		}
	}
}

// TestTokenSignVerify 签名 token 自签自验;篡改 payload / 签名 / 过期一律拒。
func TestTokenSignVerify(t *testing.T) {
	s := newAuthTestServer(t, "pw")
	exp := time.Now().Add(60 * time.Minute)
	tok := s.signToken(exp)

	if st := s.verifyToken(tok); !st.ok {
		t.Fatal("有效 token 应通过")
	}

	// 篡改签名末位 → 验签失败。
	if st := s.verifyToken(tok[:len(tok)-1] + "X"); st.ok {
		t.Error("篡改签名仍通过")
	}
	// 换一把密钥 → 验签失败。
	other := newAuthTestServer(t, "pw")
	other.cfg.AuthSecret = "another-secret"
	if st := other.verifyToken(tok); st.ok {
		t.Error("异密钥 token 仍通过")
	}
	// 已过期 → 拒。
	if st := s.verifyToken(s.signToken(time.Now().Add(-2 * time.Hour))); st.ok {
		t.Error("已过期 token 仍通过")
	}
}

// TestSlidingRenewal 剩余寿命低于窗口(< 0.5×TTL)才判续期;充裕时不续。
func TestSlidingRenewal(t *testing.T) {
	s := newAuthTestServer(t, "pw")
	// TTL=60min。<30min 才续。
	fresh := s.verifyToken(s.signToken(time.Now().Add(59 * time.Minute)))
	if !fresh.ok || fresh.renew {
		t.Errorf("新鲜 token 不应续期: ok=%v renew=%v", fresh.ok, fresh.renew)
	}
	aging := s.verifyToken(s.signToken(time.Now().Add(10 * time.Minute)))
	if !aging.ok || !aging.renew {
		t.Errorf("临近过期 token 应续期: ok=%v renew=%v", aging.ok, aging.renew)
	}
}

// TestPreSharedToken 预共享 token 定长比较;未配置则关闭该路径。
func TestPreSharedToken(t *testing.T) {
	s := newAuthTestServer(t, "pw")
	s.cfg.AuthToken = "long-lived-script-token"
	if !s.preSharedOK("long-lived-script-token") {
		t.Error("正确预共享 token 被拒")
	}
	if s.preSharedOK("wrong") || s.preSharedOK("") {
		t.Error("错误/空预共享 token 被放行")
	}
	s.cfg.AuthToken = ""
	if s.preSharedOK("long-lived-script-token") {
		t.Error("未配置时预共享路径应关闭")
	}
}

// TestRequireAuthEnforcement 未登录 401;有效 cookie 或预共享 Bearer 放行;
// healthz / login 免鉴权。直接测中间件(用探针处理器),不经真实业务处理器。
func TestRequireAuthEnforcement(t *testing.T) {
	s := newAuthTestServer(t, "pw")
	reached := false
	probe := func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}
	guarded := s.requireAuth(probe)

	// 未登录 → 401,且不触达处理器。
	rec := httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("未登录 = %d, want 401", rec.Code)
	}
	if reached {
		t.Error("未登录竟触达处理器")
	}

	// 有效 cookie → 放行。
	reached = false
	tok := s.signToken(time.Now().Add(60 * time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: tok})
	guarded(httptest.NewRecorder(), req)
	if !reached {
		t.Error("有效 cookie 未放行")
	}

	// 预共享 Bearer → 放行。
	reached = false
	s.cfg.AuthToken = "script-token"
	req = httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
	req.Header.Set("Authorization", "Bearer script-token")
	guarded(httptest.NewRecorder(), req)
	if !reached {
		t.Error("预共享 Bearer 未放行")
	}

	// healthz 免鉴权 → 200(经完整路由表)。
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthz = %d, want 200", rec.Code)
	}
}

// TestLoginFlow 正确口令发 cookie(含 HttpOnly/SameSite)+ X-Auth-Token;错误口令 401。
func TestLoginFlow(t *testing.T) {
	s := newAuthTestServer(t, "secret-pw")

	// 正确口令。
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"password":"secret-pw"}`))
	rec := httptest.NewRecorder()
	s.authLoginAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("登录正确口令 = %d, want 200", rec.Code)
	}
	var setCookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			setCookie = c.Value
			if !c.HttpOnly {
				t.Error("会话 cookie 应 HttpOnly")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Error("会话 cookie 应 SameSite=Lax")
			}
		}
	}
	if setCookie == "" {
		t.Fatal("登录未下发会话 cookie")
	}
	if rec.Header().Get("X-Auth-Token") == "" {
		t.Error("登录未下发 X-Auth-Token(Bearer 客户端需要)")
	}

	// 错误口令 → 401,且文案不泄露是否已配置。
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		strings.NewReader(`{"password":"nope"}`))
	rec = httptest.NewRecorder()
	s.authLoginAPI(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("错误口令 = %d, want 401", rec.Code)
	}
}

// TestLoginRateLimit 登录限流:超过阈值后 429(防在线爆破)。
func TestLoginRateLimit(t *testing.T) {
	s := newAuthTestServer(t, "pw")
	s.loginLimiter = newRateLimiter(3, time.Minute)
	body := `{"password":"nope"}`
	var lastCode int
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		req.RemoteAddr = "10.0.0.9:1234"
		rec := httptest.NewRecorder()
		s.authLoginAPI(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("第 5 次登录 = %d, want 429", lastCode)
	}
}

// TestLogoutClearsCookie 登出清 cookie。
func TestLogoutClearsCookie(t *testing.T) {
	s := newAuthTestServer(t, "pw")
	rec := httptest.NewRecorder()
	s.authLogoutAPI(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("登出 = %d, want 200(幂等)", rec.Code)
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName && c.MaxAge < 0 {
			found = true
		}
	}
	if !found {
		t.Error("登出未清会话 cookie")
	}
}
