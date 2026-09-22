// issue #75 的收敛与退避单测。放在 cluster 包,与 cluster_test.go 同风格。
package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
)

// UnmatchedIndices 必须精确给出「进了池但不在任何分量里」的下标 —— 调用方据此盖扫描水位,
// 给错会永久漏召回(多标了)或永不收敛(少标了)。
func TestUnmatchedIndices(t *testing.T) {
	cases := []struct {
		name  string
		n     int
		comps [][]int
		want  []int
	}{
		{"全部无对端", 3, nil, []int{0, 1, 2}},
		{"全部成簇", 3, [][]int{{0, 1, 2}}, nil},
		{"部分成簇", 5, [][]int{{1, 3}}, []int{0, 2, 4}},
		{"空池", 0, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := UnmatchedIndices(c.n, c.comps)
			if len(got) != len(c.want) {
				t.Fatalf("len(got)=%d want %d (%v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v want %v", got, c.want)
				}
			}
		})
	}
}

// 假 provider:按调用序号返回预置错误/响应,用于验证退避与「只在成功时计账」。
type fakeProvider struct {
	calls  int
	errs   []error // 第 n 次调用返回 errs[n](不足则 nil)
	usage  ai.Usage
	onCall func(n int)
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) HealthCheck(context.Context) error { return nil }

func (f *fakeProvider) StructuredOutput(_ context.Context, _ ai.StructuredRequest) (ai.StructuredResponse, error) {
	f.calls++
	if f.onCall != nil {
		f.onCall(f.calls)
	}
	if f.calls-1 < len(f.errs) && f.errs[f.calls-1] != nil {
		// 模拟真实 provider:失败时 Usage 为零值(网关 429 时确实拿不到 usage)。
		return ai.StructuredResponse{}, f.errs[f.calls-1]
	}
	return ai.StructuredResponse{
		Data:  []byte(`{"results":[{"pair_index":0,"is_same":true,"canonical_title":"合并标题"}]}`),
		Usage: f.usage,
	}, nil
}

// APIError 分类:429/5xx 可重试,其它 4xx 确定性。
func TestAPIErrorRetryable(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{429, true}, {500, true}, {502, true}, {503, true},
		{400, false}, {401, false}, {403, false}, {404, false},
	}
	for _, c := range cases {
		e := &ai.APIError{Status: c.status}
		if got := e.Retryable(); got != c.want {
			t.Errorf("status %d: Retryable()=%v want %v", c.status, got, c.want)
		}
	}
}

// 每批重试上限:429 连发时 ConfirmPairs 只试 confirmAttempts 次,不无限循环。
func TestConfirmPairsRetryCap(t *testing.T) {
	defer func(d time.Duration) { retryBackoff = d }(retryBackoff)
	retryBackoff = 0 // 测试不等待

	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "降准靴子落地 央行释放流动性", "policy", []string{"银行", "房地产"}, now.Add(time.Hour), 0.8),
	}
	c := GenCandidates(events)
	if len(c.LLM) != 1 {
		t.Fatalf("expected 1 LLM pair, got %d", len(c.LLM))
	}

	// 全部调用都 429:应重试 confirmAttempts 次后放弃并报错。
	fp := &fakeProvider{errs: []error{
		&ai.APIError{Status: 429}, &ai.APIError{Status: 429}, &ai.APIError{Status: 429},
	}}
	_, tokens, err := ConfirmPairs(context.Background(), fp, events, c.LLM, 20, 0)
	if err == nil {
		t.Fatal("expected error when all attempts are 429")
	}
	if fp.calls != confirmAttempts {
		t.Fatalf("expected %d attempts, got %d", confirmAttempts, fp.calls)
	}
	// 全部失败 ⇒ 一分钱都不该记账(旧实现会把 0 累加进去,口径失真)。
	if tokens != 0 {
		t.Fatalf("failed attempts must not be billed, got %d", tokens)
	}
}

// 确定性 4xx 不重试:只调一次就放弃。
func TestConfirmPairsNoRetryOnDeterministic4xx(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "降准靴子落地 央行释放流动性", "policy", []string{"银行", "房地产"}, now.Add(time.Hour), 0.8),
	}
	c := GenCandidates(events)
	fp := &fakeProvider{errs: []error{&ai.APIError{Status: 400}}}
	if _, _, err := ConfirmPairs(context.Background(), fp, events, c.LLM, 20, 0); err == nil {
		t.Fatal("expected error on deterministic 4xx")
	}
	if fp.calls != 1 {
		t.Fatalf("deterministic 4xx must not be retried, got %d calls", fp.calls)
	}
}

// 第 1 次 429、第 2 次成功 ⇒ 用时一次重试,并把**成功那次**的 usage 计入。
func TestConfirmPairsRecoversAfterRetry(t *testing.T) {
	defer func(d time.Duration) { retryBackoff = d }(retryBackoff)
	retryBackoff = 0

	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "降准靴子落地 央行释放流动性", "policy", []string{"银行", "房地产"}, now.Add(time.Hour), 0.8),
	}
	c := GenCandidates(events)
	fp := &fakeProvider{
		errs:  []error{&ai.APIError{Status: 429}},
		usage: ai.Usage{InputTokens: 100, OutputTokens: 20},
	}
	verds, tokens, err := ConfirmPairs(context.Background(), fp, events, c.LLM, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if fp.calls != 2 {
		t.Fatalf("expected 1 retry (2 calls), got %d", fp.calls)
	}
	if tokens != 120 {
		t.Fatalf("only the successful call should be billed, got %d", tokens)
	}
	if !verds[0].IsSame || verds[0].CanonicalTitle != "合并标题" {
		t.Fatalf("verdict not applied: %+v", verds[0])
	}
}

// ctx 取消时退避立即返回,不挂满退避时长。
func TestBackoffRespectsContext(t *testing.T) {
	defer func(d time.Duration) { retryBackoff = d }(retryBackoff)
	retryBackoff = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	backoff(ctx, 1)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("canceled ctx should not wait for backoff, waited %s", elapsed)
	}
}

// 错误包裹链里能认出 APIError(errors.As 是 ConfirmPairs 判定可重试的依据)。
func TestAPIErrorErrorsAs(t *testing.T) {
	wrapped := errors.Join(errors.New("ctx"), &ai.APIError{Status: 429})
	var e *ai.APIError
	if !errors.As(wrapped, &e) || e.Status != 429 {
		t.Fatalf("errors.As must find wrapped APIError, got %v", wrapped)
	}
}
