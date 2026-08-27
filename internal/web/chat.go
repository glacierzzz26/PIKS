package web

// /chat AI 对话页(迭代 5-3,设计 §4):知识库问答带引用 + 截图上传识别。
// AI 配置每请求从 app_config 重读(改 /settings 即刻生效,无需重启 web)。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

const chatMaxUpload = 5 << 20 // 5MB

type ChatPage struct {
	Common
	Messages []ChatMessageView
	Models   string // 本次问答用到的模型摘要(页面脚注)
	Note     string
}

// ChatMessageView 渲染视图:refs 已解包、时间已格式化、attachments 转附件 id 列表。
type ChatMessageView struct {
	ID        string
	Role      string
	Content   string
	CreatedAt string
	Refs      store.ChatRefs
	Images    []string
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.chatPost(w, r)
		return
	}
	s.chatShow(w, r, "", "")
}

func (s *Server) chatShow(w http.ResponseWriter, r *http.Request, note, errMsg string) {
	ctx := r.Context()
	sessionID, err := s.store.GetOrCreateChatSession(ctx)
	if err != nil {
		s.render(w, "chat", ChatPage{Common: Common{Title: "AI 对话 · PIKS", Active: "chat", Err: "读会话失败: " + err.Error()}})
		return
	}
	msgs, err := s.store.ListChatMessages(ctx, sessionID)
	if err != nil {
		s.render(w, "chat", ChatPage{Common: Common{Title: "AI 对话 · PIKS", Active: "chat", Err: "读消息失败: " + err.Error()}})
		return
	}
	page := ChatPage{
		Common:   Common{Title: "AI 对话 · PIKS", Active: "chat"},
		Messages: toChatViews(msgs),
		Note:     note,
	}
	if errMsg != "" {
		page.Err = errMsg
	}
	s.render(w, "chat", page)
}

func toChatViews(msgs []model.ChatMessage) []ChatMessageView {
	out := make([]ChatMessageView, 0, len(msgs))
	for _, m := range msgs {
		v := ChatMessageView{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			CreatedAt: m.CreatedAt.In(cst).Format("01-02 15:04"),
		}
		var refs store.ChatRefs
		if len(m.Refs) > 0 {
			_ = json.Unmarshal(m.Refs, &refs)
		}
		v.Refs = refs
		var atts []string
		if len(m.Attachments) > 0 {
			_ = json.Unmarshal(m.Attachments, &atts)
		}
		v.Images = atts
		out = append(out, v)
	}
	return out
}

// chatPost 处理提问/上传/清空。
func (s *Server) chatPost(w http.ResponseWriter, r *http.Request) {
	isMultipart := strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
	if isMultipart {
		if err := r.ParseMultipartForm(chatMaxUpload + 1<<20); err != nil {
			s.chatShow(w, r, "", "表单解析失败(文件超 5MB?): "+err.Error())
			return
		}
	} else if err := r.ParseForm(); err != nil {
		s.chatShow(w, r, "", "表单解析失败: "+err.Error())
		return
	}
	ctx := r.Context()
	if r.FormValue("action") == "clear" {
		sid, err := s.store.GetOrCreateChatSession(ctx)
		if err == nil {
			_ = s.store.ClearChatSession(ctx, sid)
		}
		http.Redirect(w, r, "/chat", http.StatusSeeOther)
		return
	}

	question := strings.TrimSpace(r.FormValue("question"))
	var file io.ReadCloser
	var fh *multipart.FileHeader
	if isMultipart {
		f, h, err := r.FormFile("file")
		if err != nil && err != http.ErrMissingFile {
			s.chatShow(w, r, "", "读取上传文件失败: "+err.Error())
			return
		}
		file, fh = f, h
	}
	if question == "" && fh == nil {
		s.chatShow(w, r, "", "请输入问题或上传截图。")
		return
	}

	sid, err := s.store.GetOrCreateChatSession(ctx)
	if err != nil {
		s.chatShow(w, r, "", "会话创建失败: "+err.Error())
		return
	}

	// 每请求重读 AI 配置(改 /settings 即刻生效)。
	cfgMap, err := s.store.ListAppConfig(ctx)
	if err != nil {
		s.chatShow(w, r, "", "读 AI 配置失败: "+err.Error())
		return
	}
	base, key := cfgMap["ai_service_base_url"], cfgMap["ai_api_key"]
	if base == "" || key == "" {
		s.chatShow(w, r, "", "AI 未配置:请先到 /settings 填写服务地址与密钥。")
		return
	}

	// 先存用户消息(附件若有)。
	attIDs := []string{}
	var img *ai.ImagePart
	if fh != nil {
		if !allowedImageType(fh.Header.Get("Content-Type")) {
			s.chatShow(w, r, "", "仅支持图片(png/jpeg/webp/gif)。")
			return
		}
		data, rerr := io.ReadAll(io.LimitReader(file, chatMaxUpload+1))
		if rerr != nil {
			s.chatShow(w, r, "", "读文件失败: "+rerr.Error())
			return
		}
		if len(data) > chatMaxUpload {
			s.chatShow(w, r, "", "图片超过 5MB 上限。")
			return
		}
		attID, aerr := s.saveUpload(ctx, fh.Filename, fh.Header.Get("Content-Type"), data)
		if aerr != nil {
			s.chatShow(w, r, "", "保存截图失败: "+aerr.Error())
			return
		}
		attIDs = append(attIDs, attID)
		img = &ai.ImagePart{Data: data, MIME: fh.Header.Get("Content-Type")}
	}
	userMsg := model.ChatMessage{SessionID: sid, Role: "user", Content: question}
	if len(attIDs) > 0 {
		b, _ := json.Marshal(attIDs)
		userMsg.Attachments = b
	}
	if err := s.store.InsertChatMessage(ctx, &userMsg); err != nil {
		s.chatShow(w, r, "", "存用户消息失败: "+err.Error())
		return
	}

	// 组装回答。
	assistant, note, aerr := s.answerChat(ctx, cfgMap, question, img)
	if aerr != nil {
		// 如实降级:把失败写入对话历史(用户可见),并标错误。
		assistant = &model.ChatMessage{SessionID: sid, Role: "assistant", Content: "⚠️ 调用失败:" + aerr.Error()}
	}
	assistant.SessionID = sid
	if err := s.store.InsertChatMessage(ctx, assistant); err != nil {
		s.chatShow(w, r, "", "存回答失败: "+err.Error())
		return
	}
	if aerr != nil {
		s.chatShow(w, r, note, "AI 调用失败: "+aerr.Error())
		return
	}
	s.chatShow(w, r, note, "")
}

// answerChat 检索 + 组装上下文 + 调 LLM,返回待入库的 assistant 消息与提示。
func (s *Server) answerChat(ctx context.Context, cfg map[string]string, question string, img *ai.ImagePart) (*model.ChatMessage, string, error) {
	extract := cfg["ai_model_extract"]
	if extract == "" {
		extract = "deepseek-chat"
	}
	vision := cfg["ai_model_vision"]

	// 截图 + 视觉模型未配置 → 如实降级(设计 §4.3)。
	if img != nil && vision == "" {
		return &model.ChatMessage{
			Role: "assistant",
			Content: "⚠️ 已收到截图,但截图/视觉模型未配置(`ai_model_vision` 为空)。" +
				"请在 /settings 选择一个视觉模型(如 deepseek-v4-flash-vision-exp)后重试,或用文字描述图片内容。",
		}, "截图识别已降级:未配置视觉模型。", nil
	}

	// 检索 grounding。
	var events []model.Event
	var entities []model.Entity
	q := question
	if q == "" {
		q = "截图"
	}
	events, entities, err := s.store.SearchKnowledge(ctx, q, 8, 8)
	if err != nil {
		return nil, "", fmt.Errorf("检索知识库: %w", err)
	}

	system, contextBlock := buildChatContext(events, entities, img != nil)
	note := ""
	if len(events) == 0 && len(entities) == 0 {
		system += "\n注意:检索结果为空,如实说明'知识库未检索到相关内容',不要编造。"
		note = "未检索到与问题相关的知识库条目(回答可能基于模型通用知识)。"
	}

	modelName := extract
	if img != nil {
		modelName = vision
	}
	c := ai.NewOpenAICompat(cfg["ai_service_base_url"], cfg["ai_api_key"], modelName)
	user := question
	if user == "" {
		user = "请识别这张截图的内容,并结合知识库说明它涉及什么。"
	}
	if contextBlock != "" {
		user = "知识库检索结果:\n" + contextBlock + "\n\n用户问题/请求:\n" + user
	}
	resp, err := c.Chat(ctx, ai.ChatOptions{System: system, User: user, Image: img})
	if err != nil {
		return nil, "", err
	}

	content, refs := extractRefs(resp.Content, events, entities)
	refsJSON, _ := json.Marshal(refs)
	return &model.ChatMessage{Role: "assistant", Content: content, Refs: refsJSON}, note, nil
}

// buildChatContext 把检索结果组装成 LLM 可见的引用块(方括号 id 供答案标注)。
func buildChatContext(events []model.Event, entities []model.Entity, hasImage bool) (system, contextBlock string) {
	system = `你是 PIKS 个人 A 股投资知识系统的问答助手。基于下方"知识库检索结果"回答用户问题。
规则:
- 每个支撑结论的事件/实体都要在句子末尾标注引用:事件 [E:事件id]、实体 [N:实体id](id 必须来自检索结果方括号内,不要自造)。
- 若问题涉及"某事件影响了什么",务必同时引用该事件本身及其影响的实体。
- 只要检索结果非空,至少引用一个事件或实体;检索结果没有覆盖的,如实说明,不要编造事件、数字或结论。
- 用中文回答,简洁专业,可适当分点。`
	if hasImage {
		system += "\n- 用户还上传了截图:先转录图中关键内容,再结合检索结果说明其关联。"
	}

	var b strings.Builder
	if len(events) > 0 {
		b.WriteString("== 相关事件 ==\n")
		for _, e := range events {
			fmt.Fprintf(&b, "[E:%s] %s (类型=%s, 状态=%s)\n", e.ID, e.Title, e.EventType, e.Status)
			if e.OccurredAt != nil {
				fmt.Fprintf(&b, "  时间: %s\n", e.OccurredAt.In(cst).Format("2006-01-02"))
			}
			if e.Summary != nil {
				fmt.Fprintf(&b, "  摘要: %s\n", *e.Summary)
			}
			if len(e.Facts) > 0 && string(e.Facts) != "null" && string(e.Facts) != "[]" {
				fmt.Fprintf(&b, "  事实: %s\n", string(e.Facts))
			}
		}
	}
	if len(entities) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("== 相关实体 ==\n")
		for _, en := range entities {
			fmt.Fprintf(&b, "[N:%s] %s (类型=%s)\n", en.ID, en.Name, en.Type)
			if len(en.Aliases) > 0 && string(en.Aliases) != "null" && string(en.Aliases) != "[]" {
				fmt.Fprintf(&b, "  别名: %s\n", string(en.Aliases))
			}
			if en.Description != nil {
				fmt.Fprintf(&b, "  描述: %s\n", *en.Description)
			}
		}
	}
	return system, strings.TrimSuffix(b.String(), "\n")
}

// extractRefs 从答案提取 [E:uuid]/[N:uuid] 标注 → refs;并清掉标注符(内容更干净)。
// 只保留检索结果中真实存在的 id(防 LLM 自造)。
func extractRefs(content string, events []model.Event, entities []model.Entity) (string, store.ChatRefs) {
	evTitle := map[string]string{}
	for _, e := range events {
		evTitle[e.ID] = e.Title
	}
	enTitle := map[string]string{}
	for _, e := range entities {
		enTitle[e.ID] = e.Name
	}
	var refs store.ChatRefs
	seenE, seenN := map[string]bool{}, map[string]bool{}
	re := regexp.MustCompile(`\[([EN]):([0-9a-fA-F-]{8,})]`)
	content = re.ReplaceAllStringFunc(content, func(s string) string {
		parts := re.FindStringSubmatch(s)
		kind, id := parts[1], parts[2]
		switch kind {
		case "E":
			if t, ok := evTitle[id]; ok && !seenE[id] {
				seenE[id] = true
				refs.Events = append(refs.Events, store.ChatRef{ID: id, Title: t})
			}
		case "N":
			if t, ok := enTitle[id]; ok && !seenN[id] {
				seenN[id] = true
				refs.Entities = append(refs.Entities, store.ChatRef{ID: id, Title: t})
			}
		}
		return "" // 标注符清掉,引用改由页面下方 refs chip 呈现
	})
	return strings.TrimSpace(content), refs
}

// saveUpload 落盘截图到 {UploadDir}/{date}/{uuid}.{ext},并记录附件元数据。
func (s *Server) saveUpload(ctx context.Context, filename, mime string, data []byte) (string, error) {
	ext := map[string]string{
		"image/png": "png", "image/jpeg": "jpg",
		"image/webp": "webp", "image/gif": "gif",
	}[mime]
	if ext == "" {
		return "", fmt.Errorf("不支持的图片类型 %s", mime)
	}
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(s.cfg.UploadDir, time.Now().In(cst).Format("2006-01-02"))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("创建上传目录: %w", err)
	}
	path := filepath.Join(dir, id+"."+ext)
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", err
	}
	attID, err := s.store.CreateAttachment(ctx, &model.Attachment{
		Filename: filename, MIME: mime, Size: int64(len(data)), Path: path,
	})
	if err != nil {
		return "", err
	}
	return attID, nil
}

// handleAttachmentAPI 回显上传的截图(/api/attachments/{id})。
func (s *Server) handleAttachmentAPI(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/attachments/")
	att, err := s.store.GetAttachment(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", att.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, att.Path)
}

func allowedImageType(mime string) bool {
	switch mime {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return true
	}
	return false
}

// newUUID 16 字节随机 hex(截图文件名用;DB 主键走 gen_random_uuid)。
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
