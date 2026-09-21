// Package announce 公告分级:A 层巨潮公告的**标题级**重要性分级(issue #68 / A 层)。
//
// 为什么只能靠标题(实测,勿凭想象改):
//
//	巨潮 announcementType 是**不可解数字码**(如 "01010503||010112||010115||012325"),
//	announcementTypeName 实测**逐条为 null**,字典端点不可得(internal/collector/cninfo_announce.go)。
//	故分级只能用 announcementTitle 做关键词规则。⚠️ 不得据码臆断类别。
//
// 分级用途(本期)= **展示层折叠**,不是进 LLM 的门槛:
//
//	公告现状是 status='collected' —— 不进 worker(只取 status='raw')、不报对账异常(T4 决定)。
//	故公告当前 **LLM 成本为 0**,分级**不产生成本收益**;它的价值是让 ~1200 条/日不再平铺。
//	⚠️ 将来若要「必读+重要进抽取」,是一道独立改判(须同步改 worker 取值 + reconcile 口径)。
//
// 分级是**机器判定(Inference)不是事实**:UI 须如实标注(项目既有纪律,同 P7 量价形态)。
package announce

import "strings"

// 级别取值。落 raw_documents.grade(迁移 0017),与 DB CHECK 约束一致。
const (
	LevelMust      = "must"      // 必读:监管动作 / 退市风险 / 重大重组 / 控制权变更
	LevelImportant = "important" // 重要:股权激励 / 回购 / 增减持 / 重大合同
	LevelRoutine   = "routine"   // 常规:定期报告 / 三会决议 / 权益分派
	LevelNoise     = "noise"     // 噪音:工商变更 / 独董述职 / 中介机构衍生文件
)

// 关键词表。**顺序即优先级**,见 Grade 的判定次序 —— 顺序错了会误分级:
//
//   - neg 必须最先:否定式样板("未被处罚""最近五年没有被采取监管措施")含 must 词但语义相反,
//     实测 2026-09-18 单日有 3 条这类文件,若放在 must 之后会被误判为「必读」。
//   - intermediary 次之:券商/律所的核查意见、法律意见、持续督导报告是**衍生文件**,
//     它们标题里含「重大资产重组」「向特定对象发行」等 must/important 词,但本身无独立信息。
//     实测 2026-09-18「居然智家重大资产重组」一家就有 4 份不同券商的核查意见,占满必读位。
//
// ⚠️ 改这些表**必须重跑校准**(见 grade_test.go 的分布用例),否则占比会漂。
var (
	neg = []string{"未被", "未受到", "未发生", "不存在", "无违规", "未违反", "未受", "没有", "最近五年", "最近三年"}

	intermediary = []string{
		"法律意见", "持续督导", "保荐", "鉴证报告", "审计报告", "跟踪报告", "督导", "核查意见",
		"独立财务顾问", "会议资料", "会计师事务所", "评估报告", "验证报告", "专项意见",
		"受托管理报告", "财务顾问", "核查报告",
	}

	must = []string{
		"立案告知", "立案调查", "被立案", "退市", "风险警示", "暂停上市", "终止上市",
		"无法表示意见", "保留意见", "否定意见", "监管函", "关注函", "警示函", "处罚",
		"纪律处分", "重大资产重组", "控制权变更", "要约收购", "详式权益变动", "收购报告书",
		"预亏", "预减", "首亏", "业绩大幅", "被实施", "破产", "重整", "清算",
		"资金占用", "违规担保", "被采取", "市场禁入",
	}

	important = []string{
		"股权激励", "限制性股票", "回购", "增持", "减持", "重大合同", "中标", "框架协议",
		"战略合作", "员工持股", "向特定对象发行", "权益变动", "重大投资", "签订",
	}

	routine = []string{
		"年度报告", "半年度报告", "季度报告", "董事会决议", "股东大会", "股东会决议", "监事会",
		"权益分派", "分红", "利润分配", "简式权益变动", "提示性公告", "进展公告",
		"业绩说明会", "业绩快报",
	}

	noise = []string{
		"工商变更", "独立董事", "述职", "名称变更", "经营范围", "变更注册资本", "迁址",
		"办公地址", "H股公告",
	}
)

// Grade 按标题判定级别(纯函数,可离线单测)。
//
// 判定次序(不可调换,理由见各表注释):
//
//	否定式 → 中介机构衍生文件 → 必读 → 重要 → 噪音 → 常规 → **默认常规**
//
// ⚠️ **默认落「常规」而非「噪音」** —— 红线(issue #68 §3.3):分级失败/未命中宁可多显示,
// 不可误隐藏。噪音级只折叠、不隐藏,但默认选常规能把误判代价压到最低。
func Grade(title string) string {
	s := title
	switch {
	case matches(s, neg):
		return LevelRoutine
	case matches(s, intermediary):
		return LevelNoise
	case matches(s, must):
		return LevelMust
	case matches(s, important):
		return LevelImportant
	case matches(s, noise):
		return LevelNoise
	case matches(s, routine):
		return LevelRoutine
	default:
		return LevelRoutine
	}
}

// matches 标题是否含任一关键词(等价于 Python `k in s`,校准脚本按同一语义跑)。
func matches(s string, keys []string) bool {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}
