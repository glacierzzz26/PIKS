// collector 采集命令:源适配器 → 归一化 → content_hash 去重 → raw_documents。
//
// issue #43 T1:事件类多源采集。`-driver all` 依次跑全部独立机构源,每源落各自的
// sources 行(**机构名**,不再是任务名 `news-flash`),使前端可区分来源。
// 单源失败**不阻断**其余源(免费源无 SLA)。
//
// issue #68 C 层:两种运行模式。
//   - **一次性**(默认,`-interval=0`):跑一轮即退,供日管线收盘后补齐(pipeline.sh)。
//   - **常驻**(`-interval>0`):交易时段内每 interval 采一轮,反封禁护栏
//     (令牌桶/空响应哨兵/熔断,见 internal/collector/limiter.go)的状态
//     **留内存跨轮生效** —— 一次性进程每轮从零开始,哨兵与熔断无从累积。
//     形态仿 cmd/research-worker(常驻 + signal.NotifyContext + 优雅退出)。
//
// ⚠️ 常驻循环**只跑快讯源**(`-driver news`):公告(巨潮)单日 ~1200 条 / 40 页,
// 量小但翻页重,留在日管线 16:10 采一次即可,不进盘中高频循环(否则每 3 分钟重复翻 40 页)。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"piks/internal/collector"
	"piks/internal/config"
	"piks/internal/model"
	"piks/internal/store"
)

// sourceSpec 一个机构源的采集规格:驱动名 + sources.name(机构名)+ source_type。
// 机构名是**独立机构的标识**(非任务名),前端「快讯」tab 的来源筛选以此为准。
//
// ⚠️ source_type 还决定**是否送 LLM 抽取**(见 runOne):'announcement' 的源是官方披露、
// 无真假问题,落 status='collected' —— 既不进 worker(只取 status='raw'),
// 也不报对账异常(processed_no_event 只看 status='processed')。issue #50 T4。
var sourceSpecs = []struct {
	Driver     string
	Name       string // sources.name —— 机构名
	SourceType string
}{
	{"dongcai", "东方财富", "news"},
	{"jin10", "金十数据", "news"},
	{"cls", "财联社", "news"},
	{"sina", "新浪财经", "news"},
	{"ths", "同花顺", "news"},
	{"futu", "富途资讯", "news"},
	// 公告作为**原始事件源**接入(issue #50 / #43 T4):官方披露站(证监会指定),
	// 只存标题+外链,不抓正文(正文只在 PDF 里)。
	{"cninfo-announce", "巨潮资讯", "announcement"},
}

// cst 北京时间(盘中时段闸用)。
var cst = time.FixedZone("CST", 8*3600)

func main() {
	var (
		driverFlag = flag.String("driver", "file", "collector driver: file|all|news|dongcai|jin10|cls|sina|ths|futu|cninfo-announce")
		input      = flag.String("input", "", "file driver input path")
		sourceName = flag.String("source", "", "source name (机构名) override;默认按 driver 映射")
		interval   = flag.Duration("interval", 0, "常驻模式轮询间隔(如 3m);0 = 跑一轮即退(日管线用)")
		session    = flag.String("session", "09:15-15:05", "常驻模式盘中时段闸(北京时间 HH:MM-HH:MM);空 = 不限时段")
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

	specs, err := resolveSpecs(*driverFlag, *sourceName)
	if err != nil {
		fatal("collector:", err)
	}

	if *interval <= 0 {
		// 一次性模式(日管线):单源失败不阻断;全部失败才退出码非零。
		if failed, total := runAll(ctx, s, specs, *input); failed == total {
			fatal("collector: all sources failed")
		}
		return
	}

	runResident(s, specs, *input, *interval, *session)
}

// runAll 跑一轮全部源,返回 (失败源数, 源总数)。单源失败不阻断其余源(免费源无 SLA,#43)。
func runAll(ctx context.Context, s *store.Store, specs []spec, input string) (failed, total int) {
	for _, sp := range specs {
		if err := runOne(ctx, s, sp, input); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "collector %s/%s FAILED: %v\n", sp.Driver, sp.Name, err)
		}
	}
	return failed, len(specs)
}

// runResident 常驻循环:交易时段内每 interval 采一轮,收到 SIGTERM/SIGINT 优雅退出。
//
// 不落「今日已跑」stamp —— 那是日管线的单一日锁,盘中轮询本就不该被它阻断
// (设计 §5.1 已指出该锁正是盘中不跑的根因)。
func runResident(s *store.Store, specs []spec, input string, interval time.Duration, session string) {
	start, end, hasSession := parseSession(session)
	log.Printf("collector 常驻启动:interval=%s session=%q 源数=%d", interval, session, len(specs))

	stopCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	tick := func() {
		now := time.Now().In(cst)
		if !inSession(now, start, end, hasSession) {
			return // 时段外/非工作日静默跳过(不产生请求)
		}
		runAll(stopCtx, s, specs, input)
	}
	tick() // 启动即采一轮,不等首个 tick

	for {
		select {
		case <-stopCtx.Done():
			log.Printf("收到关停信号,collector 退出")
			return
		case <-ticker.C:
			tick()
		}
	}
}

// parseSession 解析 "HH:MM-HH:MM" 为当日分钟数;空串或格式非法时 has=false(不限时段)。
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

// inSession 判断北京时间是否落在 [start,end] 分钟内,且为工作日。
// has=false 时只判工作日(不限时段)。weekday 用传入时刻自算,便于单测注入。
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

// spec 一个待采集源的解析结果。
type spec struct {
	Driver     string
	Name       string
	SourceType string
}

// resolveSpecs 把 -driver/-source 解析为待采集源清单。
// 显式 -source 覆盖机构名(单一 driver 时才有意义;file 驱动靠它归属)。
func resolveSpecs(driver, source string) ([]spec, error) {
	if driver == "all" {
		out := make([]spec, 0, len(sourceSpecs))
		for _, s := range sourceSpecs {
			out = append(out, spec{Driver: s.Driver, Name: s.Name, SourceType: s.SourceType})
		}
		return out, nil
	}
	// news = 仅快讯源(6 个),常驻盘中轮询用:公告翻页重,留在日管线采一次(见文件头)。
	if driver == "news" {
		var out []spec
		for _, s := range sourceSpecs {
			if s.SourceType == "news" {
				out = append(out, spec{Driver: s.Driver, Name: s.Name, SourceType: s.SourceType})
			}
		}
		return out, nil
	}
	if driver == "file" {
		name := source
		if name == "" {
			name = "news-flash" // 迭代 0 保底驱动的既有归属,保持向后兼容
		}
		return []spec{{Driver: "file", Name: name, SourceType: "news"}}, nil
	}
	for _, s := range sourceSpecs {
		if s.Driver == driver {
			name := s.Name
			if source != "" {
				name = source
			}
			return []spec{{Driver: s.Driver, Name: name, SourceType: s.SourceType}}, nil
		}
	}
	return nil, fmt.Errorf("unknown driver: %s", driver)
}

// runOne 采集单个源并入库。任一步失败即返回错误(由调用方决定是否阻断)。
func runOne(ctx context.Context, s *store.Store, sp spec, input string) error {
	runID, err := s.StartTaskRun(ctx, "collector")
	if err != nil {
		return fmt.Errorf("start task run: %w", err)
	}

	drv, err := collector.NewDriver(sp.Driver, input)
	if err != nil {
		return finishFail(ctx, s, runID, sp, err)
	}

	src, err := ensureSource(ctx, s, sp)
	if err != nil {
		return finishFail(ctx, s, runID, sp, err)
	}
	// 人工暂停的源**真跳过**(issue #68 C 层):此前 PauseSource 写了 status 却无人读取
	// (采集照旧每轮去拉),暂停名不副实。现尊重它 —— 与 ReconSilentSources 已排除 paused 同口径。
	// ⚠️ 不再按「单轮 fetch 连续失败」自动 PauseSource:3 分钟轮询下那样会 3 分钟就停一个源;
	// 瞬时故障改由 per-host **熔断**承担(时间盒 + 自愈,见 internal/collector/limiter.go)。
	if src.Status == "paused" {
		_ = s.FinishTaskRun(ctx, runID, "skipped", "",
			map[string]any{"source": sp.Name, "driver": sp.Driver, "reason": "source paused"})
		fmt.Printf("collector %s/%s: skipped (paused)\n", sp.Driver, sp.Name)
		return nil
	}

	// 单次 Fetch(其内部 httpSource 已含最多 3 次退避重试 + per-host 护栏)。
	news, err := drv.Fetch(ctx)
	if err != nil {
		return finishFail(ctx, s, runID, sp, err)
	}

	newCount, dupCount, failCount := 0, 0, 0
	// 公告等原始事件源落 'collected':已采集、无需 LLM 抽取(issue #50)。
	// 快讯源保持空 → InsertRawDocument 默认 'raw'(待抽取)。
	status := ""
	if sp.SourceType == "announcement" {
		status = "collected"
	}
	for _, n := range news {
		ok, err := s.InsertRawDocument(ctx, &model.RawDocument{
			SourceID:    src.ID,
			ExternalID:  collector.StrPtr(n.ExternalID),
			URL:         collector.StrPtr(n.URL),
			Title:       collector.StrPtr(n.Title),
			Content:     n.Content,
			ContentHash: collector.ContentHash(n.Content),
			PublishedAt: n.PublishedAt,
			Status:      status,
			Extra:       n.Extra,
		})
		switch {
		case err != nil:
			failCount++
		case ok:
			newCount++
		default:
			dupCount++
		}
	}

	meta := map[string]any{
		"source": sp.Name,
		"driver": sp.Driver,
		"new":    newCount,
		"dup":    dupCount,
		"failed": failCount,
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		return fmt.Errorf("finish task run: %w", err)
	}
	fmt.Printf("collector %s/%s: new=%d dup=%d failed=%d\n",
		sp.Driver, sp.Name, newCount, dupCount, failCount)
	return nil
}

// ensureSource 取或建该机构源。源被暂停时**不**自动复活(须人工确认,防抖动源空转)。
func ensureSource(ctx context.Context, s *store.Store, sp spec) (model.Source, error) {
	src, err := s.GetSourceByName(ctx, sp.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		src = model.Source{Name: sp.Name, SourceType: sp.SourceType}
		if err := s.CreateSource(ctx, &src); err != nil {
			return model.Source{}, err
		}
		return src, nil
	}
	return src, err
}

// finishFail 记账失败并返回错误(不 os.Exit —— 多源模式下单源失败须继续跑其余源)。
func finishFail(ctx context.Context, s *store.Store, runID int64, sp spec, err error) error {
	_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(),
		map[string]any{"source": sp.Name, "driver": sp.Driver})
	return err
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}
