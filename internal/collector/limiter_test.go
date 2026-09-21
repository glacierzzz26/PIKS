package collector

// 反封禁护栏单测(issue #68 C 层)。repo 首次用 httptest.Server —— 断言「熔断开路后不再发请求」。
// 决策用假时钟(reserve 是纯决策,不睡眠),故单测零真睡、可确定性复现。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock 可推进的假时钟。
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// 令牌桶:初始 burst 个令牌可瞬时发出,用尽后按有效速率补(此处 2/s → 0.5s/个)。
func TestHostLimiterBurstThenThrottle(t *testing.T) {
	c := &fakeClock{t: time.Unix(1000, 0)}
	l := newHostLimiter(c.now)

	for i := 0; i < int(hostBurst); i++ {
		if d, err := l.reserve(c.now()); err != nil || d != 0 {
			t.Fatalf("burst %d: want 0 wait, got d=%v err=%v", i, d, err)
		}
	}
	d, err := l.reserve(c.now())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := time.Duration(float64(time.Second) / hostRefillPerSec) // 0.5s
	if d != want {
		t.Fatalf("exhausted bucket: want %v, got %v", want, d)
	}
}

// 空响应哨兵:连续 3 次空 → 降速一档(×2);出现非空即刻复位。
func TestHostLimiterEmptySentinel(t *testing.T) {
	c := &fakeClock{t: time.Unix(1000, 0)}
	l := newHostLimiter(c.now)

	l.observe(0)
	l.observe(0)
	if l.effectiveRate() != hostRefillPerSec {
		t.Fatalf("2 empties should not penalize yet, rate=%v", l.effectiveRate())
	}
	l.observe(0) // 第 3 次 → 降速
	if got := l.effectiveRate(); got != hostRefillPerSec/2 {
		t.Fatalf("after 3 empties want rate %v, got %v", hostRefillPerSec/2, got)
	}
	// 用尽 burst 后,间隔应为 1/rate = 1s(未降速时为 0.5s)。
	for i := 0; i < int(hostBurst); i++ {
		_, _ = l.reserve(c.now())
	}
	d, _ := l.reserve(c.now())
	if want := time.Second; d != want {
		t.Fatalf("penalized spacing: want %v, got %v", want, d)
	}
	// 非空 → 复位。
	l.observe(7)
	if got := l.effectiveRate(); got != hostRefillPerSec {
		t.Fatalf("non-empty should reset penalty, rate=%v", got)
	}
}

// 熔断:4 连败开路 → 冷却期内拒发 → 冷却后半开探测 → 再失败一次即重新开路。
func TestHostLimiterBreaker(t *testing.T) {
	c := &fakeClock{t: time.Unix(1000, 0)}
	l := newHostLimiter(c.now)

	for i := 0; i < failStreakToOpen; i++ {
		if _, err := l.reserve(c.now()); err != nil {
			t.Fatalf("call %d should pass before opening: %v", i, err)
		}
		l.record(false)
	}
	if _, err := l.reserve(c.now()); !errors.Is(err, errCircuitOpen) {
		t.Fatalf("want errCircuitOpen after %d failures, got %v", failStreakToOpen, err)
	}

	// 冷却后半开:放行一次探测。
	c.advance(breakerCooldown + time.Second)
	if _, err := l.reserve(c.now()); err != nil {
		t.Fatalf("after cooldown should allow a probe: %v", err)
	}
	// 半开期再失败一次 → 立刻重新开路(阈值降为 1)。
	l.record(false)
	if _, err := l.reserve(c.now()); !errors.Is(err, errCircuitOpen) {
		t.Fatalf("half-open failure should re-open, got %v", err)
	}

	// 成功则闭合。
	c.advance(breakerCooldown + time.Second)
	l.record(true)
	if _, err := l.reserve(c.now()); err != nil {
		t.Fatalf("success should close the breaker: %v", err)
	}
}

// 注册表按 host 共享、跨路径同一实例(同 host 的驱动/分页共用一个限流器)。
func TestLimiterForSharesByHost(t *testing.T) {
	a := limiterFor("https://same.example.com/path1")
	b := limiterFor("https://same.example.com/path2?x=1")
	if a != b {
		t.Fatal("same host must share one limiter")
	}
	if c := limiterFor("https://other.example.com/p"); c == a {
		t.Fatal("different hosts must not share")
	}
}

// 端到端:doJSON 走真实 httptest.Server —— 连续失败后熔断开路,后续调用**不发请求**。
func TestDoJSONCircuitBreakerStopsRequests(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	h := newHTTPSource(2*time.Second, 0, 1) // minGap=0(免等),attempts=1
	ctx := context.Background()

	for i := 0; i < failStreakToOpen; i++ {
		if _, err := h.getJSON(ctx, srv.URL, nil); err == nil {
			t.Fatalf("call %d: expected failure from 500", i)
		}
	}
	before := atomic.LoadInt32(&hits)
	if _, err := h.getJSON(ctx, srv.URL, nil); !errors.Is(err, errCircuitOpen) {
		t.Fatalf("want errCircuitOpen, got %v", err)
	}
	if after := atomic.LoadInt32(&hits); after != before {
		t.Fatalf("open breaker must not send request: hits %d → %d", before, after)
	}
}
