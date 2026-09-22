package ths

// 同花顺自选客户端:持有 cookie / userid / 会话过期时间,提供限频 + 指数退避的 GET。
// 只读 —— 见 ths.go 的包红线。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// mobileUA 同花顺移动端 UA(实测;网页版 UA 取不到自选)。
const mobileUA = "Hexin_Gphone/11.28.03 (Royal Flush) hxtheme/0 innerversion/G037.09.028.1.32 " +
	"followPhoneSystemTheme/0 userid/000000000 getHXAPPAccessibilityMode/0 hxNewFont/1 isVip/0 " +
	"getHXAPPFontSetting/normal getHXAPPAdaptOldSetting/0 okhttp/3.14.9"

// 端点常量(集中一处,便于改版时定位)。
const (
	selfStockListURL   = "https://t.10jqka.com.cn/newcircle/group/getSelfStockWithMarket/"
	selfStockDetailURL = "https://ugc.10jqka.com.cn/selfstock_detail"
	realheadURL        = "https://d.10jqka.com.cn/v6/realhead/hs_%s/last.js"
)

// Credentials 凭据。Cookie 优先(已实测);否则用 Account/Password 登录。
type Credentials struct {
	Cookie   string // 整串 "a=1; b=2"(用户从浏览器/抓包复制)
	Account  string
	Password string
}

// ErrAuth 上游鉴权失败(cookie 过期 / 未登录)。调用方据此区分「该重登」还是「该报错」。
var ErrAuth = errors.New("ths: authentication failed")

// Client 同花顺只读客户端。**并发安全**(常驻服务可能与 web 共用,虽当前只 cmd 用)。
type Client struct {
	http   *http.Client
	minGap time.Duration
	creds  Credentials // 供 EnsureSession 按需登录

	mu       sync.Mutex
	cookies  map[string]string
	userid   string
	sessExp  time.Time // sess_tk 的 exp(零值 = 未知)
	lastCall time.Time
	loginOK  bool
}

// New 构造客户端。凭据为空则不预置会话,调用方须先 EnsureSession。
func New(creds Credentials) *Client {
	c := &Client{
		// 超时 15s:同花顺偶发抖动;比 collector 的 15s 同口径。
		http:    &http.Client{Timeout: 15 * time.Second},
		minGap:  time.Second, // 同 host 请求间隔 ≥1s(礼貌限频,防触发风控)
		cookies: map[string]string{},
	}
	if strings.TrimSpace(creds.Cookie) != "" {
		c.cookies = parseCookieString(creds.Cookie)
		c.userid = c.cookies["userid"]
		if exp, ok := jwtExpiry(c.cookies["sess_tk"]); ok {
			c.sessExp = exp
		}
		c.loginOK = len(c.cookies) > 0
	}
	c.creds = creds
	return c
}

// UserID 当前会话的 userid(自选元数据接口要带它)。未登录时为空串。
func (c *Client) UserID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.userid
}

// CookieSource 会话来源:"inject"(注入 cookie)/ "login"(账密登录)/ ""(无)。
func (c *Client) CookieSource() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.cookies) == 0 {
		return ""
	}
	if strings.TrimSpace(c.creds.Cookie) != "" {
		return "inject"
	}
	return "login"
}

// SessionExpiry 返回 sess_tk 的过期时刻(零值 = 未知/未登录)。
func (c *Client) SessionExpiry() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessExp
}

// EnsureSession 保证有可用会话:
//   - 已有注入 cookie 或已登录 → 直接返回(仅在 sess_tk 临期 <6h 且非注入来源时主动重登);
//   - 否则用账密登录(无账密则返回错误,不静默)。
func (c *Client) EnsureSession(ctx context.Context) error {
	c.mu.Lock()
	has := len(c.cookies) > 0
	inject := strings.TrimSpace(c.creds.Cookie) != ""
	exp := c.sessExp
	c.mu.Unlock()

	if has {
		// 注入来源:不自动重登(用户自己换 cookie);账密来源且临期 → 续期。
		if !inject && !exp.IsZero() && time.Until(exp) < 6*time.Hour {
			return c.Login(ctx, c.creds.Account, c.creds.Password)
		}
		return nil
	}
	if c.creds.Account == "" || c.creds.Password == "" {
		return errors.New("ths: 无凭据(既无 cookie 也无账密)")
	}
	return c.Login(ctx, c.creds.Account, c.creds.Password)
}

// getJSON GET 一个 JSON 端点(限频 + 指数退避 3 次)。extraHeaders 覆盖默认头。
// 200 非 JSON → 报错(结构漂移,不空成功)。
func (c *Client) getJSON(ctx context.Context, rawURL string, extraHeaders map[string]string) ([]byte, error) {
	body, err := c.do(ctx, rawURL, extraHeaders)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// getText GET 一个 XML/JS 文本端点(同 getJSON 的重试,只是不解析 JSON)。
func (c *Client) getText(ctx context.Context, rawURL string, extraHeaders map[string]string) ([]byte, error) {
	return c.do(ctx, rawURL, extraHeaders)
}

// do 统一的重试 + 限频实现(GET)。仅 GET —— 本包只读,不需要 POST。
func (c *Client) do(ctx context.Context, rawURL string, extraHeaders map[string]string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second): // 1s → 2s
			}
		}
		c.throttle(ctx)
		body, retryable, err := c.once(ctx, rawURL, extraHeaders)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("ths: %s: after 3 attempts: %w", rawURL, lastErr)
}

// once 单次 GET。retryable=false → 确定性失败(4xx / 鉴权失败),不重试。
func (c *Client) once(ctx context.Context, rawURL string, extraHeaders map[string]string) ([]byte, bool, error) {
	req, err := c.newGetReq(ctx, rawURL, extraHeaders)
	if err != nil {
		return nil, false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err // 网络错误可重试
	}
	defer resp.Body.Close()
	b, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, true, readErr
	}
	switch {
	case resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(b, 200))
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, false, fmt.Errorf("%w: HTTP %d", ErrAuth, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(b, 200))
	}
	return b, false, nil
}

// newGetReq 构造带移动端 UA + 当前 cookie 的 GET 请求(once 与 docookie 共用)。
func (c *Client) newGetReq(ctx context.Context, rawURL string, extraHeaders map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mobileUA)
	c.mu.Lock()
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	c.mu.Unlock()
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return req, nil
}

// throttle 保证同 host 两次请求间隔 ≥ minGap。
func (c *Client) throttle(ctx context.Context) {
	c.mu.Lock()
	wait := c.minGap - time.Since(c.lastCall)
	if wait < 0 {
		wait = 0
	}
	c.lastCall = time.Now().Add(wait) // 预占下一个时隙
	c.mu.Unlock()
	if wait > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
}

// setSession 登录成功后装载会话(内部)。
func (c *Client) setSession(cookies map[string]string, userid string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cookies = cookies
	c.userid = userid
	if exp, ok := jwtExpiry(cookies["sess_tk"]); ok {
		c.sessExp = exp
	}
	c.loginOK = true
}

// truncate 错误信息里截断响应体,避免把整页 HTML 塞进 error。
func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// withQuery 拼查询串(局部 helper,避免各处手拼 escape)。
func withQuery(base string, kv ...string) string {
	if len(kv)%2 != 0 {
		panic("withQuery: odd kv")
	}
	u, _ := url.Parse(base)
	q := u.Query()
	for i := 0; i < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	u.RawQuery = q.Encode()
	return u.String()
}
