package cluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
)

func mkEvent(id, title, etype string, affected []string, at time.Time, conf float64) model.Event {
	af, _ := json.Marshal(affected)
	return model.Event{
		ID: id, Title: title, EventType: etype,
		Affected: af, OccurredAt: &at, Confidence: conf, CreatedAt: at,
	}
}

// 规则直合:归一化标题全同 + 同类型。
func TestGenCandidatesAuto(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.8),
		mkEvent("c", "星河新能源发布固态电池", "tech", []string{"新能源"}, now, 0.9),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 1 || len(c.Auto[0]) != 2 {
		t.Fatalf("expected 1 auto group of 2, got %v", c.Auto)
	}
	if len(c.LLM) != 0 {
		t.Fatalf("auto-merged events must not enter LLM pool, got %d pairs", len(c.LLM))
	}
}

// 中等置信:标题不同但实体重叠+时间近 → LLM 候选 → mock 确认同事件。
func TestLLMPath(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "降准靴子落地 央行释放流动性", "policy", []string{"银行", "房地产"}, now.Add(1*time.Hour), 0.8),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 0 {
		t.Fatalf("different titles should not auto-merge: %v", c.Auto)
	}
	if len(c.LLM) != 1 {
		t.Fatalf("expected 1 LLM pair, got %d", len(c.LLM))
	}
	verds, tokens, err := ConfirmPairs(context.Background(), ai.NewMock(), events, c.LLM, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tokens == 0 {
		t.Fatal("expected token usage recorded")
	}
	if !verds[0].IsSame {
		t.Fatal("mock should confirm 降准 pair as same event")
	}
	if verds[0].CanonicalTitle == "" {
		t.Fatal("mock should supply canonical title")
	}
	comps := BuildComponents(len(events), c.Auto, verds, c.LLM)
	if len(comps) != 1 || len(comps[0]) != 2 {
		t.Fatalf("expected 1 component of 2, got %v", comps)
	}
}

// 跨类型事件不构成 LLM 候选(防知识库污染)。
func TestNoCrossTypeCandidate(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率", "policy", []string{"银行"}, now, 0.9),
		mkEvent("b", "降准靴子落地 央行释放流动性", "tech", []string{"银行"}, now.Add(1*time.Hour), 0.8),
	}
	c := GenCandidates(events)
	if len(c.LLM) != 0 {
		t.Fatalf("cross-type events must not be LLM candidates, got %d", len(c.LLM))
	}
}
