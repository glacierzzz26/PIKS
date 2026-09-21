package collector

// per-host 反封禁护栏(issue #68 C 层 S3)。提频(盘中每 3 分钟)的**硬前置**:
// 现状只有 per-source `minGap`(http.go),对「同一 host 的突发」无约束。三条护栏:
//
//	1. 令牌桶  —— 限同一 host 的**平均速率 + 突发**(压住分页/退避重试的短时密集请求);
//	2. 空响应哨兵 —— 请求成功但**连续 N 次 0 条**视为被上游**静默限流**,自动放慢该 host;
//	3. 熔断   —— 同一 host **连续 K 次失败**后开路,冷却期内**直接失败、不发网络 I/O**。
//
// ⚠️ **实测佐证**(issue #43 / #68 §5.1):东财 push2 对频繁请求回 **SSL RST(IP 级限流)**;
// 东财正文接口 300 连发 209 失败。免费源无 SLA,护栏是**不重演限流**的前提,不是优化。
//
// 设计取舍:
//   - **per-host、进程内共享**:同一 host 的多个驱动/分页共用一个限流器(设计要求 per-host 全局);
//   - **不落盘**:状态在内存,故护栏要**跨周期生效必须常驻进程** —— 这正是 collector 以
//     `-interval` 常驻循环运行(而非每次 cron 拉起一次性进程)的原因(见 cmd/collector);
//   - **确定性可测**:`reserve` 是**纯决策**(给定 now → 需等待多久),睡眠留给调用方,
//     故单测可注入假时钟、零真睡(见 limiter_test.go)。

import (
	"context"
	"errors"
	"math"
	"net/url"
	"sync"
	"time"
)

// 护栏默认参数(实测校准见文件头;改动须重估上游友好度)。
const (
	hostRefillPerSec = 2.0 // 令牌补充速率(个/秒);单源 minGap=2s 时通常不触发,主要压突发
	hostBurst        = 3.0 // 突发容量

	emptyStreakToPenalize = 3 // 连续 N 次空响应 → 判定静默限流
	penaltyCap            = 8 // 有效速率最大降速倍数(1/N)

	failStreakToOpen = 4                // 连续 N 次失败 → 开路
	breakerCooldown  = 60 * time.Second // 开路冷却时长
)

// errCircuitOpen 熔断器开路:该 host 冷却中,调用方应视为「暂时不可用」而非请求失败。
var errCircuitOpen = errors.New("collector: host circuit open (cooling down, no request sent)")

type hostLimiter struct {
	mu sync.Mutex

	// 令牌桶
	tokens float64
	last   time.Time

	// 空响应哨兵
	emptyStreak int
	penalty     float64 // 有效速率 = hostRefillPerSec / penalty;≥1,封顶 penaltyCap

	// 熔断
	failStreak int
	tripped    bool      // 曾开路 → 半开期只容忍 1 次失败即重新开路
	openUntil  time.Time // 开路截止;now < openUntil 时 refuse

	now func() time.Time // 测试注入假时钟
}

func newHostLimiter(now func() time.Time) *hostLimiter {
	return &hostLimiter{tokens: hostBurst, penalty: 1, now: now}
}

// ---- 进程内注册表:按 host 惰性建、共享 ----

var (
	hostMu       sync.Mutex
	hostLimiters = map[string]*hostLimiter{}
)

// hostOf 取 URL 的 host(解析失败则退化为原串,仍能起分组作用)。
func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return rawURL
}

// limiterFor 取(或建)某 URL 所属 host 的限流器。进程内共享,故可跨驱动/跨分页累计。
func limiterFor(rawURL string) *hostLimiter {
	h := hostOf(rawURL)
	hostMu.Lock()
	defer hostMu.Unlock()
	l, ok := hostLimiters[h]
	if !ok {
		l = newHostLimiter(time.Now)
		hostLimiters[h] = l
	}
	return l
}

// observeFetch 驱动侧便捷入口:按该驱动的固定端点 URL,记账本次 Fetch 拿到的条数
// (喂给空响应哨兵)。Fetch 成功但 0 条即计一次空响应。
func observeFetch(rawURL string, n int) { limiterFor(rawURL).observe(n) }

// observe 空响应哨兵入口:驱动在 Fetch 成功返回时喂入**本次拿到的条数**。
// n==0 记为一次空响应;连续 emptyStreakToPenalize 次则降速一档(×2,封顶 penaltyCap);
// 出现非空即刻复位(误判自愈)。
func (l *hostLimiter) observe(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > 0 {
		l.emptyStreak = 0
		l.penalty = 1
		return
	}
	l.emptyStreak++
	if l.emptyStreak >= emptyStreakToPenalize {
		l.penalty = math.Min(penaltyCap, l.penalty*2)
		l.emptyStreak = 0
	}
}

// record 记账一次请求结果(熔断依据)。hostAlive=true 表示 host 有正常响应
// (含 4xx —— 那是请求问题,不代表 host 不可用),立即复位并闭合熔断。
func (l *hostLimiter) record(hostAlive bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if hostAlive {
		l.failStreak = 0
		l.tripped = false
		l.openUntil = time.Time{}
		return
	}
	l.failStreak++
	threshold := failStreakToOpen
	if l.tripped {
		threshold = 1 // 半开探测期:再失败一次即重新开路
	}
	if l.failStreak >= threshold {
		l.openUntil = l.now().Add(breakerCooldown)
		l.tripped = true
		l.failStreak = 0
	}
}

// effectiveRate 当前有效补充速率(令牌/秒),含哨兵降速。
func (l *hostLimiter) effectiveRate() float64 {
	return hostRefillPerSec / l.penalty
}

// reserve 计算并**占用**一个令牌,返回调用方需等待的时长(纯决策,不睡眠 —— 便于假时钟单测)。
// 熔断开路时返回 errCircuitOpen(调用方应放弃本次请求,不发网络 I/O)。
func (l *hostLimiter) reserve(now time.Time) (time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.openUntil.IsZero() && now.Before(l.openUntil) {
		return 0, errCircuitOpen
	}
	if l.last.IsZero() {
		l.last = now
	}
	rate := l.effectiveRate()
	if elapsed := now.Sub(l.last).Seconds(); elapsed > 0 {
		l.tokens = math.Min(hostBurst, l.tokens+elapsed*rate)
		l.last = now
	}
	if l.tokens >= 1 {
		l.tokens--
		return 0, nil
	}
	wait := time.Duration((1 - l.tokens) / rate * float64(time.Second))
	l.tokens = 0
	l.last = now.Add(wait) // 占用未来时点:下次调用据此续算,避免重复等待
	return wait, nil
}

// wait 按 reserve 的决策睡眠(经 ctx 可中断);onWait 钩子供测试观察等待。
func (l *hostLimiter) wait(ctx context.Context, onWait func(time.Duration)) error {
	d, err := l.reserve(l.now())
	if err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	if onWait != nil {
		onWait(d)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
