// 聚类重审视 Pass(design docs/phase2/design/cluster-quality.md):修复「跨簇重复盲区」。
// 根因:`ListUnclusteredEvents` 只返回 cluster_id IS NULL 的事件,已聚类 canonical 永不进入候选池,
// 导致 ① 两个 canonical 各属一簇后彼此不可见(跨簇重复),② 重复对象已聚类的新报道永久滞留未聚类池。
// 本 Pass 把候选池扩为「全部活跃簇 canonical ∪ 剩余未聚类事件」,复用迭代 1 的
// GenCandidates + ConfirmPairs + BuildComponents(阈值不变),确认的同簇重复并入既有簇。
package cluster

import (
	"context"
	"fmt"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

// pickSurvivorIndex 从分量 comp 里挑 survivor 下标(存活 canonical):
//   - 必须属于某个既有簇(clusterOf[i] != "");
//   - 多个时按「最早创建,同则更高置信」(与 ApplyClusters canonical 选取同比较器);
//   - 无簇成员返回 -1(分量全是未聚类事件,正常 pass 应已处理,护栏跳过)。
//
// clusterOf 与 pool 平行:pool[i] 所属簇 id,未聚类为 ""。
func pickSurvivorIndex(pool []model.Event, clusterOf []string, comp []int) int {
	survivor := -1
	for _, i := range comp {
		if clusterOf[i] == "" {
			continue
		}
		if survivor == -1 ||
			pool[i].CreatedAt.Before(pool[survivor].CreatedAt) ||
			(pool[i].CreatedAt.Equal(pool[survivor].CreatedAt) && pool[i].Confidence > pool[survivor].Confidence) {
			survivor = i
		}
	}
	return survivor
}

// countActiveMembers 返回簇内非 merged 成员数(重审视中这些成员将被并入 survivor 并标 merged)。
func countActiveMembers(ctx context.Context, s *store.Store, clusterID string) (int, error) {
	evs, err := s.ListEventsByCluster(ctx, clusterID)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range evs {
		if e.Status != "merged" {
			n++
		}
	}
	return n, nil
}

// ReexamineClusters 重审视既有 canonical 与未聚类事件(design cluster-quality §3)。
// 返回:新并入簇的事件数(新 merged)、LLM 确认对数、消耗 token。
// maxTokens>0 时作为本次可用剩余预算,超出停止确认(剩余对视为不同事件,同 ConfirmPairs)。
func ReexamineClusters(ctx context.Context, s *store.Store, p ai.Provider, batch int, maxTokens int64) (int, int64, int, error) {
	reps, err := s.ListActiveClusterRepresentatives(ctx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reexamine: list representatives: %w", err)
	}
	unclustered, err := s.ListUnclusteredEvents(ctx, 10000)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reexamine: list unclustered: %w", err)
	}

	pool := make([]model.Event, 0, len(reps)+len(unclustered))
	clusterOf := make([]string, 0, len(pool))
	for _, r := range reps {
		pool = append(pool, r.Event)
		clusterOf = append(clusterOf, r.ClusterID)
	}
	for _, e := range unclustered {
		pool = append(pool, e)
		clusterOf = append(clusterOf, "")
	}
	if len(pool) < 2 {
		return 0, 0, 0, nil
	}

	cand := GenCandidates(pool)
	var verdicts []PairVerdict
	tokens := int64(0)
	if len(cand.LLM) > 0 {
		verdicts, tokens, err = ConfirmPairs(ctx, p, pool, cand.LLM, batch, maxTokens)
		if err != nil {
			return 0, tokens, len(cand.LLM), fmt.Errorf("reexamine: confirm: %w", err)
		}
	}
	comps := BuildComponents(len(pool), cand.Auto, verdicts, cand.LLM)

	merged := 0
	for _, comp := range comps {
		survivor := pickSurvivorIndex(pool, clusterOf, comp)
		if survivor == -1 {
			continue // 全未聚类分量:正常 pass 已处理,护栏跳过
		}
		survID := clusterOf[survivor]
		for _, i := range comp {
			if i == survivor {
				continue
			}
			switch {
			case clusterOf[i] == "":
				// 未聚类事件:确认同事件 → 并入 survivor 簇为 merged 成员。
				if err := s.SetEventCluster(ctx, pool[i].ID, survID, "merged"); err != nil {
					return merged, tokens, len(cand.LLM), fmt.Errorf("reexamine: join unclustered %s: %w", pool[i].ID, err)
				}
				merged++
			case clusterOf[i] == survID:
				// 同簇内其他代表(同簇 auto 对,正常 pass 已处理):跳过。
			default:
				// 另一簇整体并入 survivor。
				n, err := countActiveMembers(ctx, s, clusterOf[i])
				if err != nil {
					return merged, tokens, len(cand.LLM), fmt.Errorf("reexamine: count members of %s: %w", clusterOf[i], err)
				}
				if err := s.MergeClusters(ctx, clusterOf[i], survID); err != nil {
					return merged, tokens, len(cand.LLM), fmt.Errorf("reexamine: merge cluster %s into %s: %w", clusterOf[i], survID, err)
				}
				merged += n
			}
		}
	}
	return merged, tokens, len(cand.LLM), nil
}
