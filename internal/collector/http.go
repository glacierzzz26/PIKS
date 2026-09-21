package collector

// 事件类多源采集的共用 HTTP 设施(issue #43 T1)。
//
// 免费源无 SLA(issue #43 §红线):统一限频 + 指数退避;失败不抛给调用方时须如实返回错误,
// 由 cmd/collector 决定该源是否暂停,**绝不猜测或补造数据**。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// browserUA 免费网页接口普遍要求浏览器 UA;不伪造 Referer 的具体源自行补。
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// httpSource 带限频 + 指数退避的 GET。各驱动内嵌一个实例。
type httpSource struct {
	client   *http.Client
	minGap   time.Duration // 同源请求最小间隔(限频)
	lastCall time.Time
	attempts int // 单请求最大尝试次数(含首次)
	onWait   func(d time.Duration)
}

func newHTTPSource(timeout, minGap time.Duration, attempts int) *httpSource {
	return &httpSource{
		client:   &http.Client{Timeout: timeout},
		minGap:   minGap,
		attempts: attempts,
	}
}

// getJSON 取 JSON 并对 5xx/网络错误做指数退避重试(200ms → 400ms → 800ms,封顶 2s)。
// 4xx 视为确定性失败,立即返回(退避没用)。
// extraHeaders 为可选的逐源请求头(如财联社的 Referer)。
func (h *httpSource) getJSON(ctx context.Context, url string, extraHeaders map[string]string) ([]byte, error) {
	return h.doJSON(ctx, http.MethodGet, url, "", extraHeaders)
}

// postFormJSON 以 application/x-www-form-urlencoded 发 POST 取 JSON。
// 部分上游(如巨潮 cninfo)的查询接口**只认 POST**(实测同参数 GET 直接 500 HTML),
// 故需 POST 支持。params 须为**已 URL 编码**的查询串(如 "a=1&b=2");
// 巨潮 seDate 参数含 空格~空格,须由调用方用 url.Values.Encode() 编码。
func (h *httpSource) postFormJSON(ctx context.Context, url, params string, extraHeaders map[string]string) ([]byte, error) {
	return h.doJSON(ctx, http.MethodPost, url, params, extraHeaders)
}

// doJSON 统一的重试 + 限频 + 请求实现(GET/POST 共用,避免两份退避逻辑漂移)。
func (h *httpSource) doJSON(ctx context.Context, method, url, formBody string, extraHeaders map[string]string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < h.attempts; attempt++ {
		if attempt > 0 {
			// 指数退避 200ms → 400ms → 800ms …,封顶 2s。
			backoff := backoffDelay(attempt, 200*time.Millisecond, 2*time.Second)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		h.throttle(ctx)
		body, retryable, err := h.once(ctx, method, url, formBody, extraHeaders)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", h.attempts, lastErr)
}

// once 单次请求。retryable=false 表示确定性失败(4xx / 非网络错误),不必退避重试。
// formBody 非空时以表单 POST 发送并带 Content-Type;空则以 GET 发送。
func (h *httpSource) once(ctx context.Context, method, url, formBody string, extraHeaders map[string]string) (body []byte, retryable bool, err error) {
	var rdr io.Reader
	if method == http.MethodPost {
		rdr = strings.NewReader(formBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", browserUA)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, true, err // 网络错误可重试
	}
	defer resp.Body.Close()
	b, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, true, readErr
	}
	if resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(b, 200))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(b, 200))
	}
	return b, false, nil
}

// throttle 保证同源两次请求间隔 ≥ minGap。
func (h *httpSource) throttle(ctx context.Context) {
	if h.minGap <= 0 {
		return
	}
	if wait := h.minGap - time.Since(h.lastCall); wait > 0 {
		if h.onWait != nil {
			h.onWait(wait)
		}
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
	h.lastCall = time.Now()
}

// backoffDelay 指数退避:base << (attempt-1),封顶 max。
func backoffDelay(attempt int, base, max time.Duration) time.Duration {
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= max {
			return max
		}
	}
	if d > max {
		return max
	}
	return d
}

// toRaw 把上游原始 JSON 对象留档为 extra(issue #43:热度/分级信号不可回溯,必须原样存)。
// 序列化失败返回空 map —— extra 是附加信息,不因它阻断采集。
func toRaw(v any) json.RawMessage {
	if v == nil {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// jsonGet 从 map 里取字符串(容忍 nil / 非字符串)。
func jsonGet(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}
