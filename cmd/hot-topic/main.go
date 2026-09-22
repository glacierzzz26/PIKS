// hot-topic 热榜采集命令(issue #68 D 层):A 股**事件/题材**热榜 → hot_topic_items 表。
//
// 🔴 与 collector **刻意隔离**:热榜不进 raw_documents、不进聚类、不触碰 events
//
//	(设计 §6 红线 —— 热度可被操纵,不得混入印证度)。故本命令**只**写独立表。
//
// 形态仿 cmd/collector 的常驻模式(issue #68 C 层):交易时段内每 interval 采一轮,
// 复用 per-host 三护栏(状态留内存跨轮生效)。**必须常驻**的理由同 C 层:熔断/哨兵
// 靠跨轮累积,一次性进程每轮归零、护栏形同虚设。
//
// 频率:盘中每 30 分钟(用户 2026-09-22 定)。热榜分钟级变化小,30 分钟足够看出热度走势,
// 又不像 3 分钟那样灌一堆近似重复行(热榜每次仅 ~28 条 → 盘中约 11 次/日 ≈ 300 行)。
//
// 源(issue #68 评论实测):同花顺话题榜(~15 条,新 host dq.10jqka.com.cn)+
// 财联社首页热文(~13 条,解析 SSR)。⚠️ 两源**粒度不同、同题对 = 0**,故各出各的、
// 不合并(方案 A,详见 internal/collector/hottopic.go 文件头)。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"piks/internal/collector"
	"piks/internal/config"
	"piks/internal/model"
	"piks/internal/store"
)

// cst 北京时间(时段闸用)。与 cmd/collector 同口径。
var cst = time.FixedZone("CST", 8*3600)

func main() {
	var (
		interval = flag.Duration("interval", 0, "常驻模式轮询间隔(如 30m);0 = 跑一轮即退")
		session  = flag.String("session", "09:15-15:05", "盘中时段闸(北京时间 HH:MM-HH:MM);空 = 不限时段")
	)
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("open db:", err)
	}
	defer pool.Close()
	s := store.New(pool)

	if *interval <= 0 {
		if failed, total := runOnce(ctx, s); failed == total {
			fatal("hot-topic: all sources failed")
		}
		return
	}
	runResident(s, *interval, *session)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}

// runOnce 采一轮全部热榜源。返回 (失败源数, 源总数)。
// 单源失败不阻断其余源(免费源无 SLA,同 collector);快照时刻**逐源各自 now()** ——
// 串行采集下两源相差几秒,各自记真实采集时刻,查询侧按 source 各取最新(不强行对齐)。
func runOnce(ctx context.Context, s *store.Store) (failed, total int) {
	sources := collector.NewHotTopicSources()
	for _, src := range sources {
		if err := runSource(ctx, s, src); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "hot-topic %s FAILED: %v\n", srcName(src), err)
		}
	}
	return failed, len(sources)
}

// runSource 采集单个源:Fetch → 逐条转 model → 批量落库 → 记账 task_runs。
func runSource(ctx context.Context, s *store.Store, src collector.HotTopicSource) error {
	runID, err := s.StartTaskRun(ctx, "hot-topic")
	if err != nil {
		return fmt.Errorf("start task run: %w", err)
	}
	items, err := src.Fetch(ctx)
	if err != nil {
		_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(),
			map[string]any{"source": srcName(src)})
		return err
	}
	// 快照时刻 = 本轮采集时刻;同源同批共用一个值(便于按批取最新)。
	snap := time.Now()
	rows := make([]model.HotTopicItem, 0, len(items))
	for _, it := range items {
		rows = append(rows, model.HotTopicItem{
			Source:     it.Source,
			Rank:       it.Rank,
			Title:      it.Title,
			HotValue:   it.HotValue,
			URL:        collector.StrPtr(it.URL),
			Extra:      it.Extra,
			SnapshotAt: snap,
		})
	}
	n, err := s.InsertHotTopicItems(ctx, rows)
	if err != nil {
		_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(),
			map[string]any{"source": srcName(src)})
		return fmt.Errorf("insert: %w", err)
	}
	meta := map[string]any{"source": srcName(src), "items": n}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		return fmt.Errorf("finish task run: %w", err)
	}
	fmt.Printf("hot-topic %s: items=%d\n", srcName(src), n)
	return nil
}

// srcName 源名(HotSource* 枚举)—— 直接取接口的 Name()。
func srcName(src collector.HotTopicSource) string { return src.Name() }

// runResident 常驻循环:交易时段内每 interval 采一轮,收到 SIGTERM/SIGINT 优雅退出。
func runResident(s *store.Store, interval time.Duration, session string) {
	start, end, hasSession := parseSession(session)
	log.Printf("hot-topic 常驻启动:interval=%s session=%q", interval, session)

	stopCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	tick := func() {
		now := time.Now().In(cst)
		if !inSession(now, start, end, hasSession) {
			return // 时段外/非工作日静默跳过(不产生请求)
		}
		runOnce(stopCtx, s)
	}
	tick() // 启动即采一轮,不等首个 tick

	for {
		select {
		case <-stopCtx.Done():
			log.Printf("收到关停信号,hot-topic 退出")
			return
		case <-ticker.C:
			tick()
		}
	}
}

// parseSession 解析 "HH:MM-HH:MM" 为当日分钟数;空串或格式非法时 has=false(不限时段)。
// 与 cmd/collector 逐字同构(两个命令各自独立,不跨包共享,避免 cmd 间耦合)。
func parseSession(s string) (start, end int, has bool) {
	if s == "" {
		return 0, 0, false
	}
	var sh, sm, eh, em int
	if _, err := fmt.Sscanf(s, "%d:%d-%d:%d", &sh, &sm, &eh, &em); err != nil {
		log.Printf("session 格式非法(%q),按不限时段处理", s)
		return 0, 0, false
	}
	return sh*60 + sm, eh*60 + em, true
}

// inSession 判断北京时间是否落在 [start,end] 分钟内,且为工作日。has=false 时只判工作日。
func inSession(now time.Time, start, end int, has bool) bool {
	if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	if !has {
		return true
	}
	m := now.Hour()*60 + now.Minute()
	return m >= start && m <= end
}
