package web

import (
	"net/http"
	"strings"

	"piks/internal/config"
	"piks/internal/store"
)

// Server Web 服务。Go 只提供 JSON API(/api/v1 只读投影 + 写接口 + 图表/截图等旧 API);
// 页面全部由 React SPA 提供,Go 不再渲染 HTML。
type Server struct {
	store *store.Store
	cfg   config.Config
}

func NewServer(s *store.Store, cfg config.Config) (*Server, error) {
	return &Server{store: s, cfg: cfg}, nil
}

// Routes 返回完整路由表。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// JSON API(图谱点选面板 + 截图回显)
	mux.HandleFunc("/api/graph", s.handleGraphAPI)
	mux.HandleFunc("/api/events/", s.handleEventAPI)
	mux.HandleFunc("/api/entities/", s.handleEntityAPI)
	mux.HandleFunc("/api/attachments/", s.handleAttachmentAPI)

	// /api/v1 只读投影(React 前端数据源,字段对齐 frontend/src/lib/types.ts)
	mux.HandleFunc("/api/v1/events", s.handleAPIEvents)
	mux.HandleFunc("/api/v1/entities", s.handleAPIEntities)
	// 自选聚合(design frontend-ia §2.3):首页数据源,自选实体 + 持仓标记一次返回。
	mux.HandleFunc("/api/v1/watchlist", s.handleAPIWatchlist)
	mux.HandleFunc("/api/v1/relationships", s.handleAPIRelationships)
	mux.HandleFunc("/api/v1/market/snapshot", s.handleAPIMarketSnapshot)
	mux.HandleFunc("/api/v1/flashes", s.handleAPIFlashes)
	// 公告流(issue #50):原始事件源原始投影,与快讯互不混入(见 store 两条查询的 source_type 过滤)。
	mux.HandleFunc("/api/v1/announcements", s.handleAPIAnnouncements)
	mux.HandleFunc("/api/v1/event-types", s.handleAPIEventTypes)
	mux.HandleFunc("/api/v1/notes", s.handleAPINotes)
	mux.HandleFunc("/api/v1/notes/", s.handleAPINote)
	mux.HandleFunc("/api/v1/dashboard", s.handleAPIDashboard)
	mux.HandleFunc("/api/v1/recon", s.handleAPIRecon)
	mux.HandleFunc("/api/v1/reviews", s.handleAPIReviews)
	mux.HandleFunc("/api/v1/account", s.handleAPIAccount)
	mux.HandleFunc("/api/v1/trades", s.handleAPITrades)
	mux.HandleFunc("/api/v1/trades/", s.handleAPITradesSub)
	// 个股中心聚合(design frontend-ia §2.4):?code 一站返回持仓/深研/事件/笔记/涨停。
	mux.HandleFunc("/api/v1/stock/", s.handleAPIStock)
	mux.HandleFunc("/api/v1/chat", s.handleAPIChat)
	mux.HandleFunc("/api/v1/chat/clear", s.chatClearAPI)
	mux.HandleFunc("/api/v1/settings", s.handleAPISettings)
	mux.HandleFunc("/api/v1/settings/form", s.settingsFormAPI)
	mux.HandleFunc("/api/v1/weekly", s.handleAPIWeekly)
	mux.HandleFunc("/api/v1/weekly/detail", s.weeklyDetailAPI)
	mux.HandleFunc("/api/v1/weekly/generate", s.weeklyGenerateAPI)
	// 个股深研(research 并入,design research-merge.md §4.8):
	// 列表/触发 + 单份报告。触发即返回 run_id,实际编排在后台(D-10)。
	mux.HandleFunc("/api/v1/research-runs", s.handleAPIResearchRuns)
	mux.HandleFunc("/api/v1/research-runs/", s.handleAPIResearchRun)

	return s.cors(mux)
}

// cors 为 /api/v1 只读投影添加浏览器跨域头(React 前端 :3100 → Go :8090)。
// 无鉴权的个人系统,允许任意 Origin;GET 简单请求无需预检,OPTIONS 一并兜底。
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
