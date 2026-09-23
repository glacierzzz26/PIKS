package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"piks/internal/config"
	"piks/internal/store"
)

// Server Web 服务。Go 只提供 JSON API(/api/v1 只读投影 + 写接口 + 图表/截图等旧 API);
// 页面全部由 React SPA 提供,Go 不再渲染 HTML。
type Server struct {
	store *store.Store
	cfg   config.Config
	// 限流器(P12 / issue #78):登录防爆破 + LLM 端点 per-IP。内存令牌桶,单实例足够。
	loginLimiter *rateLimiter
	llmLimiter   *rateLimiter
}

func NewServer(s *store.Store, cfg config.Config) (*Server, error) {
	// fail-closed(P12 D3.3):口令哈希与签名密钥缺失即拒绝启动,绝不「无配置=放行」。
	if strings.TrimSpace(cfg.AuthPasswordHash) == "" || strings.TrimSpace(cfg.AuthSecret) == "" {
		return nil, fmt.Errorf("访问控制未配置: 需设 PIKS_AUTH_PASSWORD_HASH 与 PIKS_AUTH_SECRET" +
			"(见 docs/phase12/design/access-control.md §4.7;未配置一律拒启动,不接受无鉴权运行)")
	}
	return &Server{
		store:        s,
		cfg:          cfg,
		loginLimiter: newRateLimiter(5, time.Minute),  // 登录 5 次/分(防在线爆破)
		llmLimiter:   newRateLimiter(30, time.Minute), // LLM 端点 30 次/分(per-IP)
	}, nil
}

// Routes 返回完整路由表。
//
// 访问控制(P12 / issue #78):除 healthz 与 auth/login 外,**全部** /api/* 经 requireAuth。
// 白名单式 —— 新增接口默认受保护,而非默认裸奔(见 docs/phase12/design/access-control.md §4.1)。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// ---- 免鉴权端点(仅此两个)----
	mux.HandleFunc("/api/v1/healthz", s.healthzAPI)     // 存活探针(compose healthcheck)
	mux.HandleFunc("/api/v1/auth/login", s.authLoginAPI) // 登录本身

	// ---- 以下全部需鉴权 ----
	mux.HandleFunc("/api/v1/auth/logout", s.requireAuth(s.authLogoutAPI))
	mux.HandleFunc("/api/v1/auth/me", s.requireAuth(s.authMeAPI))

	// JSON API(图谱点选面板 + 截图回显)
	mux.HandleFunc("/api/graph", s.requireAuth(s.handleGraphAPI))
	mux.HandleFunc("/api/events/", s.requireAuth(s.handleEventAPI))
	mux.HandleFunc("/api/entities/", s.requireAuth(s.handleEntityAPI))
	mux.HandleFunc("/api/attachments/", s.requireAuth(s.handleAttachmentAPI))

	// /api/v1 只读投影(React 前端数据源,字段对齐 frontend/src/lib/types.ts)
	mux.HandleFunc("/api/v1/events", s.requireAuth(s.handleAPIEvents))
	mux.HandleFunc("/api/v1/entities", s.requireAuth(s.handleAPIEntities))
	// 自选聚合(design frontend-ia §2.3):首页数据源,自选实体 + 持仓标记一次返回。
	mux.HandleFunc("/api/v1/watchlist", s.requireAuth(s.handleAPIWatchlist))
	mux.HandleFunc("/api/v1/relationships", s.requireAuth(s.handleAPIRelationships))
	mux.HandleFunc("/api/v1/market/snapshot", s.requireAuth(s.handleAPIMarketSnapshot))
	mux.HandleFunc("/api/v1/flashes", s.requireAuth(s.handleAPIFlashes))
	// 热榜(issue #68 D 层):独立于事件链路的只读投影,两源分列、不合并(设计 §6 红线)。
	mux.HandleFunc("/api/v1/hot-topics", s.requireAuth(s.handleAPIHotTopics))
	// 公告流(issue #50):原始事件源原始投影,与快讯互不混入(见 store 两条查询的 source_type 过滤)。
	mux.HandleFunc("/api/v1/announcements", s.requireAuth(s.handleAPIAnnouncements))
	mux.HandleFunc("/api/v1/event-types", s.requireAuth(s.handleAPIEventTypes))
	mux.HandleFunc("/api/v1/notes", s.requireAuth(s.handleAPINotes))
	mux.HandleFunc("/api/v1/notes/", s.requireAuth(s.handleAPINote))
	mux.HandleFunc("/api/v1/dashboard", s.requireAuth(s.handleAPIDashboard))
	// 早/晚档事件榜单(issue #83 P-3):窗口内全集,时间序 + 印证度计数(不排名)。
	mux.HandleFunc("/api/v1/board", s.requireAuth(s.handleAPIBoard))
	mux.HandleFunc("/api/v1/recon", s.requireAuth(s.handleAPIRecon))
	mux.HandleFunc("/api/v1/reviews", s.requireAuth(s.handleAPIReviews))
	mux.HandleFunc("/api/v1/account", s.requireAuth(s.handleAPIAccount))
	mux.HandleFunc("/api/v1/trades", s.requireAuth(s.handleAPITrades))
	mux.HandleFunc("/api/v1/trades/", s.requireAuth(s.handleAPITradesSub))
	// 个股中心聚合(design frontend-ia §2.4):?code 一站返回持仓/深研/事件/笔记/涨停。
	mux.HandleFunc("/api/v1/stock/", s.requireAuth(s.handleAPIStock))
	mux.HandleFunc("/api/v1/chat", s.requireAuth(s.handleAPIChat))
	mux.HandleFunc("/api/v1/chat/clear", s.requireAuth(s.chatClearAPI))
	mux.HandleFunc("/api/v1/settings", s.requireAuth(s.handleAPISettings))
	mux.HandleFunc("/api/v1/settings/form", s.requireAuth(s.settingsFormAPI))
	mux.HandleFunc("/api/v1/weekly", s.requireAuth(s.handleAPIWeekly))
	mux.HandleFunc("/api/v1/weekly/detail", s.requireAuth(s.weeklyDetailAPI))
	mux.HandleFunc("/api/v1/weekly/generate", s.requireAuth(s.weeklyGenerateAPI))
	// 个股深研(research 并入,design research-merge.md §4.8):
	// 列表/触发 + 单份报告。触发即返回 run_id,实际编排在后台(D-10)。
	mux.HandleFunc("/api/v1/research-runs", s.requireAuth(s.handleAPIResearchRuns))
	mux.HandleFunc("/api/v1/research-runs/", s.requireAuth(s.handleAPIResearchRun))

	return mux
}
