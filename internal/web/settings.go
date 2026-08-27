package web

import (
	"net/http"
	"strconv"
	"strings"
)

// SettingsPage 大模型(AI)配置编辑页。
// 配置权威源 = 数据库 app_config 表(代码不再读 PIKS_AI_* 环境变量)。
type SettingsPage struct {
	Common
	Form  AIConfigForm
	Saved bool // 本次保存成功(重定向回显)
}

// AIConfigForm 编辑表单视图。密钥字段永远空值 + 掩码占位,绝不回填明文。
type AIConfigForm struct {
	BaseURL        string
	KeyMasked      string // 已配置(sk-k···i3Ab) / 未配置
	ModelExtract   string
	ModelReasoning string
	Budget         string // 原始串("0" = 护栏关闭)
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.saveSettings(w, r)
		return
	}
	s.showSettings(w, r, "")
}

func (s *Server) showSettings(w http.ResponseWriter, r *http.Request, errMsg string) {
	m, err := s.store.ListAppConfig(r.Context())
	if err != nil {
		s.render(w, "settings", SettingsPage{Common: Common{Title: "系统配置 · PIKS", Active: "settings", Err: "读取配置失败: " + err.Error()}})
		return
	}
	form := AIConfigForm{
		BaseURL:        m["ai_service_base_url"],
		KeyMasked:      maskSecret(m["ai_api_key"]),
		ModelExtract:   m["ai_model_extract"],
		ModelReasoning: m["ai_model_reasoning"],
		Budget:         m["ai_daily_token_budget"],
	}
	if form.Budget == "" {
		form.Budget = "0"
	}
	page := SettingsPage{
		Common: Common{Title: "系统配置 · PIKS", Active: "settings"},
		Form:   form,
		Saved:  r.URL.Query().Get("saved") == "1",
	}
	if errMsg != "" {
		page.Err = errMsg
	}
	s.render(w, "settings", page)
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.showSettings(w, r, "解析表单失败: "+err.Error())
		return
	}
	ctx := r.Context()

	base := strings.TrimSpace(r.FormValue("ai_service_base_url"))
	extract := strings.TrimSpace(r.FormValue("ai_model_extract"))
	reason := strings.TrimSpace(r.FormValue("ai_model_reasoning"))
	budget := strings.TrimSpace(r.FormValue("ai_daily_token_budget"))
	key := r.FormValue("ai_api_key") // 留空 = 不修改

	if base == "" || extract == "" || reason == "" {
		s.showSettings(w, r, "AI 服务地址与两个模型必填(不接受留空)。")
		return
	}
	if budget == "" {
		budget = "0"
	}
	if _, err := strconv.ParseInt(budget, 10, 64); err != nil {
		s.showSettings(w, r, "日 token 预算必须是整数(0 = 关闭护栏)。")
		return
	}

	for k, v := range map[string]string{
		"ai_service_base_url":   base,
		"ai_model_extract":      extract,
		"ai_model_reasoning":    reason,
		"ai_daily_token_budget": budget,
	} {
		if err := s.store.UpsertAppConfig(ctx, k, v); err != nil {
			s.showSettings(w, r, "保存失败: "+err.Error())
			return
		}
	}
	if key != "" {
		if err := s.store.UpsertAppConfig(ctx, "ai_api_key", key); err != nil {
			s.showSettings(w, r, "保存失败: "+err.Error())
			return
		}
	}
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

// maskSecret API key 只显示首尾 4 位,绝不落明文/回填。
func maskSecret(k string) string {
	if k == "" {
		return "未配置"
	}
	if len(k) <= 8 {
		return "已配置(***)"
	}
	return "已配置(" + k[:4] + "···" + k[len(k)-4:] + ")"
}
