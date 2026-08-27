package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"piks/internal/config"
	"piks/internal/store"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// tmplFuncs 模板函数(情绪样式/条宽百分比/涨跌色/计数/枚举中文化)。
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
		// zh 后台英文枚举 → 前台 "中文(英文)";未收录原样返回(诚实)。
		"zh": func(v string) string {
			zhMap := map[string]string{
				"company": "公司", "earnings": "业绩", "industry": "行业", "macro": "宏观",
				"policy": "政策", "tech": "科技", "concept": "概念", "topic": "主题",
				"active": "活跃", "extracted": "已抽取", "merged": "已合并",
				"Strong": "强势", "Neutral": "中性", "Weak": "弱势",
				"limit_up": "涨停数", "limit_down": "跌停数", "breadth_ratio": "涨跌比",
				"broken_rate": "炸板率", "max_board": "最高连板", "strong_yesterday": "昨日强势",
				"industry_count": "涨停行业数",
			}
			if c, ok := zhMap[v]; ok {
				return c + "(" + v + ")"
			}
			return v
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
		// selIn 判断某 id 是否在已选集合里(笔记关联多选)。
		"selIn": func(id string, list []string) bool {
			for _, v := range list {
				if v == id {
					return true
				}
			}
			return false
		},
	}
}

// Server Web 服务。只读展示(5-1)+ 大模型配置编辑(5-2 前置);AI 对话在 5-3 加。
type Server struct {
	store *store.Store
	cfg   config.Config
	pages map[string]*template.Template // 页面名 → 已解析 base+page 模板集
}

// NewServer 解析全部页面模板。
func NewServer(s *store.Store, cfg config.Config) (*Server, error) {
	sv := &Server{store: s, cfg: cfg, pages: map[string]*template.Template{}}
	pages := []struct{ name, file string }{
		{"dashboard", "templates/dashboard.html"},
		{"graph", "templates/graph.html"},
		{"events", "templates/events.html"},
		{"event", "templates/event.html"},
		{"entity", "templates/entity.html"},
		{"reviews", "templates/reviews.html"},
		{"review", "templates/review.html"},
		{"recon", "templates/recon.html"},
		{"notes", "templates/notes.html"},
		{"note_form", "templates/note_form.html"},
		{"note", "templates/note.html"},
		{"weekly", "templates/weekly.html"},
		{"settings", "templates/settings.html"},
		{"chat", "templates/chat.html"},
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
	mux.HandleFunc("/notes", s.handleNotes)
	mux.HandleFunc("/notes/new", s.handleNoteNew)
	mux.HandleFunc("/notes/", s.handleNote)
	mux.HandleFunc("/weekly", s.handleWeekly)
	mux.HandleFunc("/settings", s.handleSettings)
	mux.HandleFunc("/chat", s.handleChat)

	// JSON API(图谱点选面板 + 迭代 5-3 对话 grounding 复用 + 截图回显)
	mux.HandleFunc("/api/graph", s.handleGraphAPI)
	mux.HandleFunc("/api/events/", s.handleEventAPI)
	mux.HandleFunc("/api/entities/", s.handleEntityAPI)
	mux.HandleFunc("/api/attachments/", s.handleAttachmentAPI)

	return mux
}
