// Package web 迭代 5 Web 平台(替换 Obsidian 界面层,设计 web-app.md)。
// Go 侧只提供 JSON API(读投影 + 写接口 + 图谱/截图等旧 API),页面由 React SPA 渲染。
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"piks/internal/entityextract"
	"piks/internal/model"
)

// cst 北京时区(复盘页/看板日期展示)。
var cst = time.FixedZone("CST", 8*3600)

// termIndex 受影响词 → 实体(事件详情 JSON /api/events/{id} 的实体链接)。
// 与 publish.TermResolver 同语义,但这里返回实体 ID,而非 wikilink 路径。
type termIndex struct {
	byKey map[string]*model.Entity // key = type + "\x00" + (name|alias)
}

func newTermIndex(ents []model.Entity) *termIndex {
	idx := &termIndex{byKey: map[string]*model.Entity{}}
	for i := range ents {
		e := &ents[i]
		key := e.Type + "\x00" + e.Name
		if _, ok := idx.byKey[key]; !ok {
			idx.byKey[key] = e
		}
		var as []string
		if json.Unmarshal(e.Aliases, &as) == nil {
			for _, a := range as {
				k := e.Type + "\x00" + a
				if _, ok := idx.byKey[k]; !ok {
					idx.byKey[k] = e
				}
			}
		}
	}
	return idx
}

// resolve 精确 + 后缀剥离匹配(同 publish.Resolve,但返回实体而非路径)。
func (t *termIndex) resolve(term string) (*model.Entity, bool) {
	for _, c := range entityextract.StripSuffixes(term) {
		for _, typ := range []string{"company", "industry", "concept", "topic", "unknown"} {
			if e := t.byKey[typ+"\x00"+c]; e != nil {
				return e, true
			}
		}
	}
	return nil, false
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func fmtDate(t interface{ Format(string) string }) string { return t.Format("2006-01-02") }
func fmtTime(t interface{ Format(string) string }) string { return t.Format("01-02 15:04") }

// maxN 前 N 个。
func maxN[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func orStr(v *string, def string) string {
	if v == nil || strings.TrimSpace(*v) == "" {
		return def
	}
	return *v
}

func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
