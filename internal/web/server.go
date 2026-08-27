package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"piks/internal/store"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// tmplFuncs 模板函数(情绪样式/条宽百分比/涨跌色/计数)。
func tmplFuncs() template.FuncMap {
	return template.FuncMap{
		"emClass": func(s string) string {
			l := strings.ToLower(s)
			switch {
			case strings.Contains(l, "strong"):
				return "em-strong"
			case strings.Contains(l, "neutral"):
				return "em-neutral"
			default:
				return "em-weak"
			}
		},
		"pct": func(v, max int) int {
			if max <= 0 {
				return 0
			}
			return v * 100 / max
		},
		"pctCls": func(s string) string {
			switch {
			case strings.HasPrefix(s, "+"):
				return "up"
			case strings.HasPrefix(s, "-"):
				return "down"
			default:
				return "flat"
			}
		},
		"inc": func(i int) int { return i + 1 },
	}
}

// Server Web 服务。只读展示(迭代 5 阶段 5-1);编辑/AI 对话在 5-2/5-3 加。
type Server struct {
	store *store.Store
	pages map[string]*template.Template // 页面名 → 已解析 base+page 模板集
}

// NewServer 解析全部页面模板。
func NewServer(s *store.Store) (*Server, error) {
	sv := &Server{store: s, pages: map[string]*template.Template{}}
	pages := []struct{ name, file string }{
		{"dashboard", "templates/dashboard.html"},
		{"graph", "templates/graph.html"},
		{"events", "templates/events.html"},
		{"event", "templates/event.html"},
		{"entity", "templates/entity.html"},
		{"reviews", "templates/reviews.html"},
		{"review", "templates/review.html"},
		{"recon", "templates/recon.html"},
	}
	for _, p := range pages {
		t, err := template.New(p.name).Funcs(tmplFuncs()).ParseFS(templatesFS, "templates/base.html", p.file)
		if err != nil {
			return nil, fmt.Errorf("parse page %s: %w", p.file, err)
		}
		sv.pages[p.name] = t
	}
	return sv, nil
}

// Routes 返回完整路由表。
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))

	// 页面
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/graph", s.handleGraph)
	mux.HandleFunc("/events", s.handleEvents)
	mux.HandleFunc("/events/", s.handleEvent)
	mux.HandleFunc("/entities/", s.handleEntity)
	mux.HandleFunc("/reviews", s.handleReviews)
	mux.HandleFunc("/reviews/", s.handleReview)
	mux.HandleFunc("/recon", s.handleRecon)

	// JSON API(图谱点选面板 + 迭代 5-3 对话 grounding 复用)
	mux.HandleFunc("/api/graph", s.handleGraphAPI)
	mux.HandleFunc("/api/events/", s.handleEventAPI)
	mux.HandleFunc("/api/entities/", s.handleEntityAPI)

	return mux
}
