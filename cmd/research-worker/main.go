// research-worker 深研队列常驻 worker(2026-09-20 容器拆分 P2)。
//
// 存在理由:拆镜像后 web 容器不含 Python 运行时(os/exec python3 必挂 —— 2026-09-12 已证),
// 故「UI 触发深研」的执行方从 web 进程内 goroutine 移出,改为 web 只建 pending 行、
// 本进程认领并驱动编排。产物落 PG,前端轮询 GET 取状态,响应形状不变。
//
// 用法:
//
//	bin/research-worker [-concurrency 2] [-poll 5s] [-reap-interval 1m] [-drain 7m]
//
// 与 cmd/research-run 的关系:同一编排路径(internal/research.Orchestrator),
// 差别仅在 run 来自队列(已认领)而非命令行 —— 见 research.ProcessClaimed。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"piks/internal/ai"
	"piks/internal/config"
	"piks/internal/research"
	"piks/internal/store"
)

func main() {
	concurrency := flag.Int("concurrency", 2, "并行处理的 run 数(SKIP LOCKED 保证各取各的)")
	poll := flag.Duration("poll", 5*time.Second, "队列空时的轮询间隔")
	reapInterval := flag.Duration("reap-interval", time.Minute, "孤儿 run 回收扫描间隔")
	drain := flag.Duration("drain", 7*time.Minute, "收到关停信号后等在跑 run 收尾的上限")
	flag.Parse()

	cfg := config.Load()
	rootCtx := context.Background()
	pool, err := store.Open(rootCtx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)
	if err := s.ApplyAppConfig(rootCtx, &cfg); err != nil {
		fatal("apply app config:", err)
	}

	log.Printf("research-worker 启动:concurrency=%d poll=%s reap=%s grace=%s",
		*concurrency, *poll, *reapInterval, research.ReapGrace())

	// stopCtx 只关停**认领循环**;在跑的 run 用独立的 runCtx,收到 SIGTERM 不被打断
	// (在跑一半被杀会把 run 留成孤儿,要靠 reaper 兜 —— 能优雅收尾就不该走那条路)。
	stopCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var wg sync.WaitGroup
	// 唤醒信号:listener 收到 NOTIFY 就往里塞一个 token(非阻塞,缓冲区满即丢 ——
	// 丢 token 没关系,认领循环本身会在醒来后把队列抽空,而轮询兜底保证最多晚 poll 一轮)。
	wake := make(chan struct{}, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := s.ListenResearchNotify(stopCtx, func() {
			select {
			case wake <- struct{}{}:
			default: // 已有待处理唤醒,无需重复
			}
		}, 5*time.Second); err != nil && stopCtx.Err() == nil {
			log.Printf("队列监听退出(轮询兜底继续生效): %v", err)
		}
	}()

	// 认领协程:每个都跑同一循环,SKIP LOCKED 让它们拿到互不相交的行。
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			claimLoop(stopCtx, wake, s, cfg, *poll, workerID)
		}(i)
	}

	// 回收协程:把「进行中但久无心跳」的孤儿 run 归为 failed(进程崩溃/被杀留下的)。
	wg.Add(1)
	go func() {
		defer wg.Done()
		reapLoop(stopCtx, s, *reapInterval)
	}()

	<-stopCtx.Done()
	log.Printf("收到关停信号:停止认领,最多等 %s 让在跑 run 收尾", *drain)
	// 认领协程随 stopCtx 立刻退出;等在跑的 run 收尾(drain 上限)。
	// 注:run 自身用 Background ctx,故这里靠 wg 等它自然结束,而非取消它。
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		log.Printf("research-worker 干净退出")
	case <-time.After(*drain):
		log.Printf("等待 %s 超时,强制退出(在跑 run 可能成为孤儿,由 reaper 收口)", *drain)
	}
}

// claimLoop 认领循环:取一条 → 跑一条 → 无则等唤醒(NOTIFY)或轮询周期到。
//
// 等待时**必须同时**等 wake 与 poll ticker:NOTIFY 只是加速(丢了不报错),轮询才是
// 正确性兜底。只等 NOTIFY 会在漏通知时让 run 永远 pending。
func claimLoop(stopCtx context.Context, wake <-chan struct{}, s *store.Store, cfg config.Config, poll time.Duration, workerID int) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if stopCtx.Err() != nil {
			return
		}
		run, err := s.ClaimPendingResearchRun(stopCtx)
		if err != nil {
			if stopCtx.Err() != nil {
				return
			}
			log.Printf("worker#%d 认领失败(%.0fs 后重试): %v", workerID, poll.Seconds(), err)
			if !waitWork(stopCtx, wake, ticker) {
				return
			}
			continue
		}
		if run == nil {
			// 队空:等 NOTIFY 唤醒,或轮询周期到(兜底)。
			if !waitWork(stopCtx, wake, ticker) {
				return
			}
			continue
		}

		// 超时从**认领**算起(排队等待不消耗预算);run 用独立 ctx,不随 stopCtx 取消。
		ctx, cancel := context.WithTimeout(context.Background(), research.TimeoutTotal)
		// per-run 重读 AI 配置:改 /settings 后无需重启 worker(镜像 web 的每请求重读语义)。
		// 用副本,避免多个并发 run 之间共享同一 cfg 变量。
		runCfg := cfg
		if err := s.ApplyAppConfig(ctx, &runCfg); err != nil {
			log.Printf("worker#%d 读 app_config 失败(用启动时配置): %v", workerID, err)
		}
		o := research.New(s, newProvider(runCfg), runCfg.AIDailyTokenBudget)

		log.Printf("worker#%d 认领 run %s(code=%s profile=%s quick=%v)",
			workerID, run.RunID, run.Code, run.Profile, run.Quick)
		res, err := research.ProcessClaimed(ctx, o, run)
		cancel()
		if err != nil {
			log.Printf("worker#%d run %s 处理出错: %v", workerID, run.RunID, err)
			continue
		}
		log.Printf("worker#%d run %s 收口: status=%s", workerID, res.RunID, res.Status)
	}
}

// waitWork 等「有新活」的两种信号之一:NOTIFY 唤醒或轮询周期到。
// 返回 false 表示 stopCtx 已取消(调用方应退出)。
func waitWork(stopCtx context.Context, wake <-chan struct{}, ticker *time.Ticker) bool {
	select {
	case <-stopCtx.Done():
		return false
	case <-wake:
		return true
	case <-ticker.C:
		return true
	}
}

// reapLoop 定期回收孤儿 run(进行中但久无心跳)。stopCtx 取消即返回。
func reapLoop(stopCtx context.Context, s *store.Store, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stopCtx.Done():
			return
		case <-t.C:
			ids, err := s.ReapStuckResearchRuns(stopCtx, research.ReapGrace())
			if err != nil {
				if stopCtx.Err() == nil {
					log.Printf("回收孤儿 run 失败: %v", err)
				}
				continue
			}
			for _, id := range ids {
				log.Printf("回收孤儿 run %s(进行中但超过 %s 无心跳)", id, research.ReapGrace())
			}
		}
	}
}

// sleepCtx 睡 d 或直到 ctx 取消;返回 false 表示 ctx 已取消(调用方应退出)。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// newProvider 构造 AI provider(与 cmd/research-run / cmd/entity-build 同模式;
// PIKS_AI_PROVIDER=mock 走 mock)。AI 未配置返回 nil,编排器在合成步如实失败。
func newProvider(cfg config.Config) ai.Provider {
	if os.Getenv("PIKS_AI_PROVIDER") == "mock" {
		return ai.NewMock()
	}
	if cfg.AIServiceBaseURL == "" || cfg.AIAPIKey == "" {
		return nil
	}
	model := cfg.AIModelExtract
	if model == "" {
		model = cfg.AIModelReasoning
	}
	if model == "" {
		return nil
	}
	return ai.NewOpenAICompat(cfg.AIServiceBaseURL, cfg.AIAPIKey, model)
}

func fatal(v ...any) {
	_, _ = fmt.Fprintln(os.Stderr, append([]any{"research-worker:", time.Now().Format(time.RFC3339)}, v...)...)
	os.Exit(1)
}
