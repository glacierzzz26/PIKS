// 聚类重审视 Pass(design docs/phase2/design/cluster-quality.md):修复「跨簇重复盲区」。
// 根因:`ListUnclusteredEvents` 只返回 cluster_id IS NULL 的事件,已聚类 canonical 永不进入候选池,
// 导致 ① 两个 canonical 各属一簇后彼此不可见(跨簇重复),② 重复对象已聚类的新报道永久滞留未聚类池。
// 本 Pass 把候选池扩为「全部活跃簇 canonical ∪ 剩余未聚类事件」,复用迭代 1 的
// GenCandidates + ConfirmPairs + BuildComponents(阈值不变),确认的同簇重复并入既有簇。
package cluster

import (
	"context"
	"fmt"
	"time"

	"piks/internal/ai"
	"piks/internal/model"
	"piks/internal/store"
)

// memberCounts 由 clusterID 给出簇内「非 merged 活跃成员数」,供 survivor 选举护栏用。
type memberCounts map[string]int

// pickSurvivorIndex 从分量 comp 里挑 survivor 下标(存活 canonical):
//   - 必须属于某个既有簇(clusterOf[i] != "");
//   - 多成员簇优先于单成员簇(见下方护栏);
//   - 同优先级内按「最早创建,同则更高置信」(与 ApplyClusters canonical 选取同比较器);
//   - 无簇成员返回 -1(分量全是未聚类事件,正常 pass 应已处理,护栏跳过)。
//
// clusterOf 与 pool 平行:pool[i] 所属簇 id,未聚类为 ""。
//
// 🔴 单成员簇不得吃掉多成员簇(issue #75 复核挖出的潜伏 bug):
// 原实现纯按「最早创建」选 survivor。分量里若同时有一个**更早的单成员簇**和一个多源簇,
// 单成员簇会当选,随后 MergeClusters 把多源簇整个吸入 ⇒ **多源簇的 LLM canonicalTitle
// 被丢弃**(MergeClusters 只留 survivor 的簇标题),违反设计文档 D-Q3
// (「survivor 恒为既有簇…标题不动」,docs/phase2/design/cluster-quality.md §D-Q3)。
// 多源簇承载印证度与跨源来源列表,教训价值更高,故按成员数分层:
// **成员多的簇优先当 survivor**;成员数相同再比创建时间/置信。
func pickSurvivorIndex(pool []model.Event, clusterOf []string, counts memberCounts, comp []int) int {
	survivor := -1
	for _, i := range comp {
		if clusterOf[i] == "" {
			continue
		}
		if survivor == -1 || betterSurvivor(pool, clusterOf, counts, i, survivor) {
			survivor = i
		}
	}
	return survivor
}

// betterSurvivor 报告候选 a 是否比当前最优 b 更适合当 survivor。
func betterSurvivor(pool []model.Event, clusterOf []string, counts memberCounts, a, b int) bool {
	ca, cb := counts[clusterOf[a]], counts[clusterOf[b]]
	if ca != cb {
		return ca > cb // 多成员簇优先
	}
	if !pool[a].CreatedAt.Equal(pool[b].CreatedAt) {
		return pool[a].CreatedAt.Before(pool[b].CreatedAt)
	}
	return pool[a].Confidence > pool[b].Confidence
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
// 返回:新并入簇的事件数(新 merged)、消耗 token、送确认的对数。
// maxTokens>0 时作为本次可用剩余预算,超出停止确认(剩余对视为不同事件,同 ConfirmPairs)。
//
// since 为零值 = 不限窗口;非零 = 只取该时刻之后仍活跃的簇 + 该时刻之后扫描的事件(issue #75)。
//
// 池 = ① 窗口内活跃簇的代表 ∪ ② 窗口内未聚类事件(cluster_scanned_at IS NULL)
// ∪ ③ 窗口内**已扫描但无对端**的事件(cluster_scanned_at IS NOT NULL)。
// ③ 不可省 —— 见 store.ScannedEventsSince 的注释:少了它,被标记的事件会从正常 pass 与
// 重审视两处同时消失,永久漏召回(标记列方案与「建单例簇」的召回等价性全靠它)。
func ReexamineClusters(ctx context.Context, s *store.Store, p ai.Provider, batch int, maxTokens int64, since time.Time) (int, int64, int, error) {
	reps, err := s.ListActiveClusterRepresentativesSince(ctx, since)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reexamine: list representatives: %w", err)
	}
	unclustered, err := s.ListUnclusteredEvents(ctx, 0) // 不限:池大小由 since 收敛,不由 limit 截断
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reexamine: list unclustered: %w", err)
	}
	scanned, err := s.ScannedEventsSince(ctx, since)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("reexamine: list scanned: %w", err)
	}

	pool := make([]model.Event, 0, len(reps)+len(unclustered)+len(scanned))
	clusterOf := make([]string, 0, len(pool))
	for _, r := range reps {
		pool = append(pool, r.Event)
		clusterOf = append(clusterOf, r.ClusterID)
	}
	for _, e := range unclustered {
		pool = append(pool, e)
		clusterOf = append(clusterOf, "")
	}
	for _, e := range scanned {
		pool = append(pool, e)
		clusterOf = append(clusterOf, "")
	}
	if len(pool) < 2 {
		return 0, 0, 0, nil
	}

	// survivor 选举护栏要按簇成员数分层 ⇒ 先把各簇活跃成员数一次算好(见 pickSurvivorIndex)。
	counts := make(memberCounts)
	for _, cid := range clusterOf {
		if cid == "" {
			continue
		}
		if _, ok := counts[cid]; ok {
			continue
		}
		n, err := countActiveMembers(ctx, s, cid)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("reexamine: count members of %s: %w", cid, err)
		}
		counts[cid] = n
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
		survivor := pickSurvivorIndex(pool, clusterOf, counts, comp)
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
				// 未聚类/已扫描事件:确认同事件 → 并入 survivor 簇为 merged 成员。
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
				counts[survID] += n // 合并后 survivor 簇变大,后续比较仍成立
			}
		}
	}
	return merged, tokens, len(cand.LLM), nil
}
