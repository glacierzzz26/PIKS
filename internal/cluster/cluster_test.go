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

// #17 真实验证回归:实体措辞不一致("银行" vs "银行板块")→ 包含关系视为重叠 → LLM 候选。
func TestEntityContainmentCandidate(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调金融机构存款准备金率0.25个百分点", "policy", []string{"金融机构", "银行"}, now, 0.9),
		mkEvent("b", "央行降准0.25个百分点 释放约5000亿流动性", "policy", []string{"银行板块", "LPR"}, now.Add(45*time.Minute), 0.8),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 0 {
		t.Fatalf("different titles should not auto-merge: %v", c.Auto)
	}
	if len(c.LLM) != 1 {
		t.Fatalf("containment entity overlap must produce 1 LLM pair, got %d", len(c.LLM))
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

// 聚类质量回归:跨簇重复(design cluster-quality)。两条近同标题 canonical 分属两簇,
// 重审视候选池(既有 canonical ∪ 未聚类)会生成 LLM 对 → 触发确认并并簇。
// 这正是不加重审视时因 ListUnclusteredEvents 只聚未聚类而永远漏掉的对。
func TestCrossClusterCandidate(t *testing.T) {
	now := time.Now()
	events := []model.Event{
		mkEvent("a", "央行宣布下调存款准备金率0.25个百分点", "policy", []string{"银行", "房地产"}, now, 0.9),
		mkEvent("b", "央行宣布下调金融机构存款准备金率0.25个百分点", "policy", []string{"金融机构", "银行"}, now, 1.0),
	}
	c := GenCandidates(events)
	if len(c.Auto) != 0 {
		t.Fatalf("different titles should not auto-merge: %v", c.Auto)
	}
	if len(c.LLM) != 1 {
		t.Fatalf("cross-cluster duplicate titles must produce 1 LLM pair, got %d", len(c.LLM))
	}
}

// pickSurvivorIndex:survivor 恒为既有簇代表;最早创建,同则高置信;无簇成员返回 -1。
func TestPickSurvivorIndex(t *testing.T) {
	now := time.Now()
	pool := []model.Event{
		mkEvent("a", "事件甲", "policy", []string{"银行"}, now.Add(-2*time.Hour), 0.8), // 簇 X(最早)
		mkEvent("b", "事件乙", "policy", []string{"银行"}, now.Add(-1*time.Hour), 0.9), // 簇 Y(次早)
		mkEvent("c", "事件丙", "policy", []string{"银行"}, now, 0.5),                   // 簇 Z(最晚)
		mkEvent("d", "事件丁", "policy", []string{"银行"}, now.Add(1*time.Hour), 0.7),  // 未聚类(更早,不参与)
		mkEvent("e", "事件戊", "policy", []string{"银行"}, now, 1.0),                   // 簇 W(与 c 同刻,更高置信)
	}
	clusterOf := []string{"X", "Y", "Z", "", "W"}

	if got := pickSurvivorIndex(pool, clusterOf, []int{3}); got != -1 {
		t.Fatalf("no cluster member should return -1, got %d", got)
	}
	if got := pickSurvivorIndex(pool, clusterOf, []int{3, 1}); got != 1 {
		t.Fatalf("single cluster member should win, got %d", got)
	}
	if got := pickSurvivorIndex(pool, clusterOf, []int{0, 1, 2}); got != 0 {
		t.Fatalf("earliest created should win, got %d", got)
	}
	if got := pickSurvivorIndex(pool, clusterOf, []int{2, 4}); got != 4 {
		t.Fatalf("tie created_at should go to higher confidence, got %d", got)
	}
}
