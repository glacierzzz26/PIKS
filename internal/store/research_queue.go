package store

// 深研队列的唤醒通道(2026-09-20 容器拆分 P2)。
//
// 为什么需要唤醒:web 建 pending 行后,worker 若只靠轮询,延迟 = 轮询间隔(实测可接受但
// 白白落后几秒)。web 一发 NOTIFY、worker 一收即认领,延迟降到毫秒级。
//
// ⚠️ NOTIFY 只作**加速**,不作**正确性依赖**:通知可能丢(worker 断连重连期间发的、
// 或 LISTEN 尚未就绪时发的),而丢了就等于 run 永远 pending。故 worker 侧必须同时保留
// 轮询兜底 —— 见 cmd/research-worker 的 claimLoop(select wake / poll ticker)。
// 单靠 NOTIFY 而不轮询是错的;单靠轮询是能跑的,只是慢。两者都有才是对的。

import (
	"context"
	"time"
)

// ResearchPendingChannel 队列的 NOTIFY 频道名。
// web(发)与 worker(收)共用此常量 —— 频道名写错是静默失效(收不到就是收不到,不报错),
// 故必须单一来源,不允许各自硬编码字面量。
const ResearchPendingChannel = "piks_research_pending"

// NotifyResearchPending 通知队列有新 pending run。失败不致命(worker 的轮询会兜底),
// 故调用方可只记日志、不中断触发流程。
func (s *Store) NotifyResearchPending(ctx context.Context) error {
	// 频道名是常量标识符,非用户输入(LISTEN/NOTIFY 不支持参数占位,只能拼接)。
	_, err := s.Pool.Exec(ctx, `NOTIFY `+ResearchPendingChannel)
	return err
}

// ListenResearchNotify 持一条**独立连接** LISTEN 队列频道;每次收到通知调用 onNotify。
//
// 为什么必须独立连接:池里的连接用完即还、会被复用,不能长期占用做 LISTEN(pgxpool 会让
// 该连接一直待在池里,listener 状态随复用被清掉)。故用 Acquire 拿一条专用连接。
//
// 断线重连:连接出错(网络抖动/库重启)时按 backoff 重连;重连成功后**先补调一次 onNotify**
// —— 断连窗口内发的 NOTIFY 全丢了,靠这次补扫兜底(与轮询互为兜底)。
//
// 阻塞至 ctx 取消,返回 ctx.Err()。
func (s *Store) ListenResearchNotify(ctx context.Context, onNotify func(), backoff time.Duration) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.listenOnce(ctx, onNotify); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		// 连接断开/短读:退避后重连。重连成功即补扫(见上)。
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

// listenOnce 一次连接生命周期:连接 → LISTEN → 循环等通知,直到出错或 ctx 取消。
func (s *Store) listenOnce(ctx context.Context, onNotify func()) error {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `LISTEN `+ResearchPendingChannel); err != nil {
		return err
	}
	// 重连/首连成功后补扫一次:覆盖「上次断连窗口内漏掉的通知」与「worker 启动前
	// 已存在的存量 pending 行」。这一步让 worker 启动即可自愈,无需等下一个轮询周期。
	onNotify()

	for {
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			return err
		}
		onNotify()
	}
}
