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
	mux.HandleFunc("/api/v1/relationships", s.handleAPIRelationships)
	mux.HandleFunc("/api/v1/market/snapshot", s.handleAPIMarketSnapshot)
	mux.HandleFunc("/api/v1/flashes", s.handleAPIFlashes)
	mux.HandleFunc("/api/v1/notes", s.handleAPINotes)
	mux.HandleFunc("/api/v1/notes/", s.handleAPINote)
	mux.HandleFunc("/api/v1/dashboard", s.handleAPIDashboard)
	mux.HandleFunc("/api/v1/recon", s.handleAPIRecon)
	mux.HandleFunc("/api/v1/reviews", s.handleAPIReviews)
	mux.HandleFunc("/api/v1/trades", s.handleAPITrades)
	mux.HandleFunc("/api/v1/trades/", s.handleAPITradesSub)
	mux.HandleFunc("/api/v1/chat", s.handleAPIChat)
	mux.HandleFunc("/api/v1/chat/clear", s.chatClearAPI)
	mux.HandleFunc("/api/v1/settings", s.handleAPISettings)
	mux.HandleFunc("/api/v1/settings/form", s.settingsFormAPI)
	mux.HandleFunc("/api/v1/weekly", s.handleAPIWeekly)
	mux.HandleFunc("/api/v1/weekly/detail", s.weeklyDetailAPI)
	mux.HandleFunc("/api/v1/weekly/generate", s.weeklyGenerateAPI)

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
