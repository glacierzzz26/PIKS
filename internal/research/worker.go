package research

import (
	"context"
	"time"

	"piks/internal/store"
)

// ReapGrace 孤儿回收宽限:编排总上限 + 2 分钟。
//
// 必须 > TimeoutTotal,且 > 编排内任意两次心跳(research_runs.updated_at)写入的最大间隔,
// 否则 reaper 会误杀正在跑的 run。+2min 是给「最后一次心跳 → 定论落库」留的收尾余量。
func ReapGrace() time.Duration { return TimeoutTotal + 2*time.Minute }

// ProcessClaimed 执行一条**已由队列认领**的 run(store.ClaimPendingResearchRun 的对手方)。
//
// 与 CLI 路径(cmd/research-run)的差别全部落在 Options 的三个字段上,零分支复制:
//   - RunID = run.RunID  → 续跑这条已存在的 run,不另生成 run_id
//   - Claimed = true     → 认领方已写首个状态转移(gathering),编排不重复写
//   - Quick/Days 来自行  → 触发方(web)落库的运行参数,认领后由此还原
//
// 其余(采集/合成/机检/落库/失败如实留痕)与 CLI 完全同路。
// ctx 应带 TimeoutTotal 超时(超时从**认领**算起,排队等待不消耗预算);
// 编排自身的失败已落 research_runs.error,故此处 err 仅用于日志,不改变控制流。
func ProcessClaimed(ctx context.Context, o *Orchestrator, run *store.ResearchRun) (*Result, error) {
	return o.Run(ctx, Options{
		Code:    run.Code,
		Profile: run.Profile,
		Days:    run.Days,
		RunID:   run.RunID,
		// RequireSynthesis=!Quick(P7):快速模式无 AI 也出确定性结论,不被网关阻塞。
		RequireSynthesis: !run.Quick,
		// PriorRuns:与 web 触发路径同值(新一期把最近两份 done 研报作合成输入)。
		PriorRuns: PriorRunLimit,
		Claimed:   true,
	})
}
