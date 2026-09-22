// watch-sync 同花顺「我的自选」自动同步(issue #87):账密/cookie → 每日 3 次镜像进 PIKS。
//
// 形态仿 cmd/hot-topic 的常驻模式,**关键差异**:
//
//	hot-topic 每轮都采;watch-sync **每日定点**(09:00/12:55/18:00 北京时间),
//	靠 task_runs 去重(meta.slot = "2026-09-22#09:00",**只有 success 才算已跑**)。
//	这解决了「容器重启后同一天重复跑」的问题 —— 自选一天 3 次是用户明确的时点,
//	不能因重启变成一天 5 次(多余登录 = 风控风险)。
//
// 🔴 凭据:注入 cookie **优先于**账密(注入已实测;账密待 Phase A)。凭据直接从
//
//	app_config 表读 ths_cookie / ths_account / ths_password —— config.ApplyAppConfig
//	只认 5 个 ai_* 键,不合并新键,故此处直读 ListAppConfig。
//
// 🔴 失败一律**可见**(#64 教训):无凭据 → failed + 指路 /settings;登录失败 → 退避
//
//	(5m→15m→30m→1h,硬上限 6 次/日,触顶当轮 failed 并提示改用 cookie);上游空/
//	过滤后为空 → failed;detail 单独失败 → 降级(名单照常,meta.detail_failed=true)。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"piks/internal/config"
	"piks/internal/store"
	"piks/internal/ths"
	"piks/internal/watchsync"
)

// cst 北京时间(时点闸用)。
var cst = time.FixedZone("CST", 8*3600)

// loginBackoff 登录失败退避阶梯(5m→15m→30m→1h)。
var loginBackoff = []time.Duration{5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour}

// maxLoginsPerDay 每日登录尝试硬上限(防重试风暴 → 风控/锁号)。
const maxLoginsPerDay = 6

func main() {
	var (
		interval = flag.Duration("interval", time.Minute, "常驻模式时钟检查粒度")
		slots    = flag.String("slots", "09:00,12:55,18:00", "每日同步时点(北京时间,逗号分隔)")
		grace    = flag.Duration("grace", 90*time.Minute, "时点宽容窗(超窗不补陈旧快照)")
		tail     = flag.Duration("tail-grace", 4*time.Hour, "末个(收盘后)时点的宽容窗")
		once     = flag.Bool("once", false, "忽略时点闸,立即跑一轮")
		dryRun   = flag.Bool("dry-run", false, "只拉取并打印计划,不写库(task_runs 标 dry_run)")
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

	if *once || *dryRun {
		// 手动触发:slot 用 "manual#<北京时刻>",不去重(人就是要现在跑)。
		slot := "manual#" + time.Now().In(cst).Format("15:04")
		st, err := runOnce(ctx, s, slot, *dryRun)
		if err != nil {
			fatal("watch-sync:", err)
		}
		printStats(st)
		return
	}
	runResident(s, *interval, *slots, *grace, *tail)
}

func fatal(msg ...any) {
	fmt.Fprintln(os.Stderr, msg...)
	os.Exit(1)
}

// creds 从 app_config 读同花顺凭据。注入 cookie 优先(已实测)。
func creds(ctx context.Context, s *store.Store) (ths.Credentials, error) {
	m, err := s.ListAppConfig(ctx)
	if err != nil {
		return ths.Credentials{}, err
	}
	return ths.Credentials{
		Cookie:   strings.TrimSpace(m["ths_cookie"]),
		Account:  strings.TrimSpace(m["ths_account"]),
		Password: strings.TrimSpace(m["ths_password"]),
	}, nil
}

// ErrLogin 建立会话失败(与「上游数据异常」区分,供常驻退避/日上限用)。
var ErrLogin = errors.New("watch-sync: 建立会话失败")

// runOnce 跑一轮同步并记账。slot 写进 meta.slot(重启去重的唯一键)。
// dryRun=true 时不建实体/不改状态,meta 标 dry_run。
func runOnce(ctx context.Context, s *store.Store, slot string, dryRun bool) (watchsync.Stats, error) {
	var stats watchsync.Stats
	trigger := "resident"
	if dryRun {
		trigger = "dry-run"
	}

	cr, err := creds(ctx, s)
	if err != nil {
		return stats, fmt.Errorf("读凭据: %w", err)
	}
	if cr.Cookie == "" && (cr.Account == "" || cr.Password == "") {
		// 无凭据:明确指路,不静默 —— 记 failed(不是 skipped:配置缺失是运维事件,
		// 该在任务面板上红着,而不是像「非交易时段」那样沉默)。
		runID, _ := s.StartTaskRun(ctx, "watch-sync")
		errMsg := "未配置同花顺凭据:请在 /settings 填 ths_cookie(或 ths_account+ths_password);注入 cookie 优先于账密"
		_ = s.FinishTaskRun(ctx, runID, "failed", errMsg, map[string]any{"slot": slot, "trigger": trigger})
		return stats, errors.New(errMsg)
	}

	client := ths.New(cr)
	if err := client.EnsureSession(ctx); err != nil {
		runID, _ := s.StartTaskRun(ctx, "watch-sync")
		_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(),
			map[string]any{"slot": slot, "trigger": trigger, "login_failed": true,
				"cookie_source": client.CookieSource()})
		return stats, fmt.Errorf("%w: %v", ErrLogin, err)
	}

	runID, err := s.StartTaskRun(ctx, "watch-sync")
	if err != nil {
		return stats, err
	}
	stats, err = watchsync.Apply(ctx, s, client, dryRun)
	meta := map[string]any{
		"slot":          slot,
		"trigger":       trigger,
		"cookie_source": client.CookieSource(),
		"stats":         stats,
	}
	if dryRun {
		meta["dry_run"] = true
	}
	if err != nil {
		_ = s.FinishTaskRun(ctx, runID, "failed", err.Error(), meta)
		return stats, err
	}
	if err := s.FinishTaskRun(ctx, runID, "success", "", meta); err != nil {
		return stats, err
	}
	return stats, nil
}

func printStats(st watchsync.Stats) {
	fmt.Printf("watch-sync: 上游=%d 保留=%d 丢弃=%d | add=%d keep=%d remove=%d | 缺价=%d deferred=%d detail失败=%v\n",
		st.Upstream, st.Kept, len(st.Dropped), st.Add, st.Keep, st.Remove,
		st.PriceMissing, len(st.Deferred), st.DetailFailed)
	for _, d := range st.Dropped {
		fmt.Printf("  丢弃 %s marketid=%s (%s)\n", d.Code, d.MarketID, d.Reason)
	}
	if len(st.Deferred) > 0 {
		fmt.Printf("  deferred(名称未解析,下轮重试):%s\n", strings.Join(st.Deferred, ","))
	}
}

// runResident 常驻循环:每小时钟检查一次,到点且当日未成功跑过 → 跑一轮。
func runResident(s *store.Store, interval time.Duration, slotSpec string, grace, tail time.Duration) {
	slots, err := watchsync.ParseSlots(slotSpec, grace, tail)
	if err != nil {
		fatal("时点配置:", err)
	}
	log.Printf("watch-sync 常驻启动:slots=%s grace=%s tail=%s interval=%s", slotSpec, grace, tail, interval)

	stopCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var backoffUntil time.Time // 登录失败退避截止时刻

	tick := func() {
		now := time.Now().In(cst)
		if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
			return // 周末不跑(同 hot-topic)
		}
		day := watchsync.BeijingDay(now)
		dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cst)

		// 去重真源 = DB:当日该 slot 是否已**成功**跑过(重启不丢)。
		done := func(slotKey string) bool {
			ok, err := s.TaskRunSlotDone(stopCtx, "watch-sync", slotKey, dayStart)
			return err == nil && ok
		}
		due, missed := watchsync.DueSlot(now, day, slots, done)
		for _, m := range missed {
			log.Printf("watch-sync: 时点 %s 已超宽容窗,跳过(不补陈旧快照)", m)
		}
		if due == "" {
			return
		}
		if now.Before(backoffUntil) {
			return // 登录失败退避中
		}
		// 每日登录硬上限(DB 计数,重启不重置)—— 防重试风暴锁号。
		logins, err := s.CountTaskRunsSince(stopCtx, "watch-sync", "failed", dayStart)
		if err == nil && logins >= maxLoginsPerDay {
			log.Printf("watch-sync: 今日失败已达上限 %d 次,跳过 round slot=%s(建议改用 cookie 注入)", maxLoginsPerDay, due)
			return
		}

		stats, err := runOnce(stopCtx, s, due, false)
		if err != nil {
			if errors.Is(err, ErrLogin) {
				n, _ := s.CountTaskRunsSince(stopCtx, "watch-sync", "failed", dayStart)
				wait := loginBackoff[clamp(n-1, 0, len(loginBackoff)-1)]
				backoffUntil = time.Now().Add(wait)
				log.Printf("watch-sync: slot=%s 登录失败(%v),退避 %s(今日第 %d 次)", due, err, wait, n)
				return
			}
			log.Printf("watch-sync: slot=%s 失败(%v)", due, err)
			return
		}
		backoffUntil = time.Time{}
		log.Printf("watch-sync: slot=%s 完成 add=%d keep=%d remove=%d deferred=%d detail_failed=%v",
			due, stats.Add, stats.Keep, stats.Remove, len(stats.Deferred), stats.DetailFailed)
	}
	tick() // 启动即检查一次(容器重启后若当日未跑会立刻补上)

	for {
		select {
		case <-stopCtx.Done():
			log.Printf("收到关停信号,watch-sync 退出")
			return
		case <-ticker.C:
			tick()
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
