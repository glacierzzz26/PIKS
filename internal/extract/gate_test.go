package extract

import (
	"testing"
	"time"
)

// 抽取粗筛(issue #83 P-4 / P7)判据单测:必送/分级/阅读数/时间窗/保守默认/空 extra 不 panic。

func gateDoc(id, extra string, ago time.Duration) RawGateInput {
	return RawGateInput{ID: id, Extra: []byte(extra), RetrievedAt: time.Now().Add(-ago)}
}

// 空 extra(无信号)不得 panic,且默认全保留(保守)。
func TestGateEmptyExtraNoPanicKeepsAll(t *testing.T) {
	docs := []RawGateInput{gateDoc("a", "", time.Hour), gateDoc("b", "{}", 2*time.Hour), gateDoc("c", "not-json", 3*time.Hour)}
	res := Gate(docs, GateConfig{})
	if len(res.Keep) != 3 || len(res.Deferred) != 0 {
		t.Fatalf("默认应全保留(保守), got keep=%d deferred=%d", len(res.Keep), len(res.Deferred))
	}
}

// 默认 DeferLowSignal=false ⇒ 即便超 Max 也不落 deferred(只排序不筛)。
func TestGateDefaultDoesNotDefer(t *testing.T) {
	docs := make([]RawGateInput, 10)
	for i := range docs {
		docs[i] = gateDoc(string(rune('a'+i)), "{}", time.Duration(i)*time.Minute)
	}
	res := Gate(docs, GateConfig{Max: 3}) // 未开 DeferLowSignal
	if len(res.Deferred) != 0 || len(res.Keep) != 10 {
		t.Fatalf("默认不筛:应保留全部 10, got keep=%d deferred=%d", len(res.Keep), len(res.Deferred))
	}
}

// 必送(important/confirmed)排最前,且永不落 deferred。
func TestGateMustSendFirst(t *testing.T) {
	docs := []RawGateInput{
		gateDoc("ordinary", `{"reading_num":999999}`, time.Minute),
		gateDoc("important", `{"important":1}`, 5*time.Hour), // 老但必送
		gateDoc("confirmed", `{"confirmed":1}`, 5*time.Hour),
	}
	res := Gate(docs, GateConfig{MustSend: true, DeferLowSignal: true, Window: time.Hour, Now: time.Now()})
	if len(res.Keep) < 2 || res.Keep[0].ID != "important" && res.Keep[0].ID != "confirmed" {
		t.Fatalf("必送应排最前, got %+v", res.Keep)
	}
	for _, id := range res.Deferred {
		if id == "important" || id == "confirmed" {
			t.Errorf("必送消息不得落 deferred, got %v", res.Deferred)
		}
	}
}

// 分级 + 阅读数排序:财联社 A(level=A) > 高阅读数。
// 默认(不筛)下 Keep 顺序应体现优先级。
func TestGateOrderingByLevelThenReading(t *testing.T) {
	docs := []RawGateInput{
		gateDoc("low", `{"reading_num":10}`, time.Minute),
		gateDoc("A-level", `{"level":"A"}`, 3*time.Hour),
		gateDoc("high-read", `{"reading_num":100000}`, 2*time.Hour),
	}
	res := Gate(docs, GateConfig{})
	if len(res.Keep) != 3 {
		t.Fatalf("应保留 3, got %d", len(res.Keep))
	}
	if res.Keep[0].ID != "A-level" {
		t.Errorf("分级 A 应排最前(优先于阅读数), got %s", res.Keep[0].ID)
	}
	if res.Keep[1].ID != "high-read" {
		t.Errorf("其次应按阅读数, got %s", res.Keep[1].ID)
	}
}

// 开启 DeferLowSignal:超 Max 的**无信号**文档落 deferred;有信号的保留。
func TestGateDeferLowSignalOnly(t *testing.T) {
	docs := []RawGateInput{
		gateDoc("sig", `{"reading_num":5}`, time.Minute),
		gateDoc("nosig1", `{}`, 2*time.Minute),
		gateDoc("nosig2", `{}`, 3*time.Minute),
	}
	// Max=1:只有排最前的保留;其余按「有无信号」分流 —— sig 有信号保留,nosig 无信号落 deferred。
	res := Gate(docs, GateConfig{Max: 1, DeferLowSignal: true})
	deferred := map[string]bool{}
	for _, id := range res.Deferred {
		deferred[id] = true
	}
	if deferred["sig"] {
		t.Errorf("有信号文档不得落 deferred, got %v", res.Deferred)
	}
	if !deferred["nosig1"] || !deferred["nosig2"] {
		t.Errorf("无信号且超配额的应落 deferred, got %v", res.Deferred)
	}
	// 原因必须记账。
	for id := range deferred {
		if res.Reasons[id] == "" {
			t.Errorf("deferred 行 %s 缺原因(记账不可缺)", id)
		}
	}
}

// 时间窗:过期且非必送 → deferred(即便有信号,只要超过 Window 且开了 DeferLowSignal)。
func TestGateWindowDefersStale(t *testing.T) {
	docs := []RawGateInput{
		gateDoc("stale", `{"reading_num":999}`, 40*24*time.Hour),
		gateDoc("fresh", `{"reading_num":1}`, time.Hour),
	}
	res := Gate(docs, GateConfig{Window: 24 * time.Hour, DeferLowSignal: true, Now: time.Now()})
	if len(res.Deferred) != 1 || res.Deferred[0] != "stale" {
		t.Fatalf("超窗文档应落 deferred, got %+v", res.Deferred)
	}
	if res.Reasons["stale"] == "" {
		t.Error("deferred 原因缺失")
	}
}

// 字符串型信号(部分源 important 落成 "1")也要认。
func TestGateStringSignals(t *testing.T) {
	res := Gate([]RawGateInput{gateDoc("s", `{"important":"1"}`, time.Minute)}, GateConfig{MustSend: true})
	if len(res.Keep) != 1 {
		t.Fatalf("字符串 important=1 应被认作必送, got %+v", res.Deferred)
	}
}
