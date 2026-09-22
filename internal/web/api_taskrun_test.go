package web

import (
	"encoding/json"
	"testing"
	"time"

	"piks/internal/model"
)

// TestToTaskRunPartial 回归 issue #64:task_runs.status='partial'(部分插入失败)
// 曾被 toTaskRun 的 switch **漏掉** → 落默认分支 'running',前端显示「进行中」,
// 是另一种误导。且 meta.failed(未入库条数)此前无任何读取方,用户看不见丢数。
func TestToTaskRunPartial(t *testing.T) {
	started := time.Now()
	cases := []struct {
		name       string
		dbStatus   string
		meta       string
		wantStatus string
		wantFailed int
	}{
		{"success 无失败", "success", `{"failed":0}`, "ok", 0},
		{"成功但仍计了失败条数", "success", `{"failed":7}`, "ok", 7},
		{"部分失败", "partial", `{"new":3,"dup":0,"failed":5}`, "partial", 5},
		{"全失败", "failed", `{"failed":123}`, "failed", 123},
		{"暂停跳过", "skipped", `{}`, "skipped", 0},
		{"未知状态落 running(显式,非静默)", "weird", `{}`, "running", 0},
		{"meta 非 JSON 不炸", "success", `not-json`, "ok", 0},
		{"meta 缺失不炸", "success", ``, "ok", 0},
	}
	for _, c := range cases {
		r := model.TaskRun{
			Command: "collector", Status: c.dbStatus, StartedAt: started,
			Meta: json.RawMessage(c.meta),
		}
		got := toTaskRun(r)
		if got.Status != c.wantStatus {
			t.Errorf("%s: status = %q, want %q", c.name, got.Status, c.wantStatus)
		}
		if got.Failed != c.wantFailed {
			t.Errorf("%s: failed = %d, want %d", c.name, got.Failed, c.wantFailed)
		}
	}
}

// TestToTaskRunFailedFieldAlwaysPresent 钉住 JSON 形状:failed 是**非可选**数字
// (与 types.ts 的 `failed: number` 对齐),且为 0 时也须出现,否则前端读 undefined。
func TestToTaskRunFailedFieldAlwaysPresent(t *testing.T) {
	out, err := json.Marshal(toTaskRun(model.TaskRun{Command: "x", Status: "success", Meta: json.RawMessage(`{}`)}))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	v, ok := m["failed"]
	if !ok {
		t.Fatal("failed 字段缺失(前端类型声明为非可选数字)")
	}
	if string(v) != "0" {
		t.Errorf("failed = %s, want 0", v)
	}
}
