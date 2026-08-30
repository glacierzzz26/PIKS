package web

import (
	"context"

	"piks/internal/ai"
)

// fetchModelOptions 用当前已存 base_url+key 拉 provider 模型列表(失败返回 nil,页面兜底)。
func (s *Server) fetchModelOptions(ctx context.Context, m map[string]string) []string {
	if m["ai_service_base_url"] == "" || m["ai_api_key"] == "" {
		return nil
	}
	c := ai.NewOpenAICompat(m["ai_service_base_url"], m["ai_api_key"], m["ai_model_extract"])
	opts, err := c.ListModels(ctx)
	if err != nil {
		return nil
	}
	return opts
}

// mergeOpts 合并选项去重,保序;空值忽略。
func mergeOpts(primary []string, extra ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(primary)+len(extra))
	for _, s := range append(append([]string{}, primary...), extra...) {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func onlyNonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
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
