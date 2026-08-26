package entityextract

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"piks/internal/ai"
)

// fakeProvider 确定性 provider,用于分类器单元测试(不依赖真实 API)。
type fakeProvider struct {
	resp ai.StructuredResponse
	err  error
}

func (f *fakeProvider) Name() string                                        { return "fake" }
func (f *fakeProvider) HealthCheck(ctx context.Context) error                { return nil }
func (f *fakeProvider) StructuredOutput(ctx context.Context, req ai.StructuredRequest) (ai.StructuredResponse, error) {
	return f.resp, f.err
}

func TestClassifyValid(t *testing.T) {
	f := &fakeProvider{resp: ai.StructuredResponse{
		Data: json.RawMessage(`{"entities":[
			{"name":"LPR","type":"concept","aliases":["贷款市场报价利率"]},
			{"name":"宁德时代","type":"company","aliases":[]}
		]}`),
	}}
	c := NewClassifier(f)
	got, _, err := c.Classify(context.Background(), []string{"LPR", "宁德时代"}, nil)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entities, want 2", len(got))
	}
	if got[0].Type != "concept" || got[0].Name != "LPR" {
		t.Errorf("entity0 = %s/%s, want concept/LPR", got[0].Type, got[0].Name)
	}
	if len(got[0].Aliases) != 1 || got[0].Aliases[0] != "贷款市场报价利率" {
		t.Errorf("aliases = %v, want [贷款市场报价利率]", got[0].Aliases)
	}
}

// TestClassifyInvalidTypeDropped:非法 type 被校验丢弃;全部非法 → error(调用方兜底建 unknown,§5.7 诚实不猜类型)。
func TestClassifyInvalidTypeDropped(t *testing.T) {
	f := &fakeProvider{resp: ai.StructuredResponse{
		Data: json.RawMessage(`{"entities":[{"name":"X","type":"bogus"}]}`),
	}}
	c := NewClassifier(f)
	if _, _, err := c.Classify(context.Background(), []string{"X"}, nil); err == nil {
		t.Fatal("expected error for all-invalid output")
	}
}

// TestClassifyProviderFailure:provider 失败(3 次重试后)→ error → 调用方 unknown 落点。
func TestClassifyProviderFailure(t *testing.T) {
	f := &fakeProvider{err: errors.New("boom")}
	c := NewClassifier(f)
	if _, _, err := c.Classify(context.Background(), []string{"X"}, nil); err == nil {
		t.Fatal("expected error on provider failure")
	}
}

// TestStripSuffixes:后缀剥离候选(原词 + 依次剥离板块/概念/指数/股/类)。
func TestStripSuffixes(t *testing.T) {
	got := StripSuffixes("银行板块")
	want := []string{"银行板块", "银行"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
