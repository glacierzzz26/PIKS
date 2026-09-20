package cluster

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// 跨源数值冲突检测(issue #49 / epic #43 T3)。
//
// 场景:同一真实事件被多家机构分别报道 → 已聚成一簇(T2)。各家的事实句是**自由文本**,
// 措辞各异;若两家对**同一个量**给出**不同的数**,那是必须让人看见的差异 —— 不能因为
// canonical 只留一条就悄悄丢掉另一个版本(三层模型 Fact ≠ Inference:模型不替人挑版本)。
//
// 判定走**确定性数值比对**,不调 LLM。理由:
//  1. 可复现 —— 结论不依赖网关可用性与模型版本;
//  2. 可离线标定 —— 能在真实语料上量出误报率(issue #49 验收 c);
//  3. dev 的 AI 网关(OpenCode Zen)实测欠费,LLM 路径本就不可用。

// conflictUnits 数值单位白名单。
//
// ⚠️ 单位**必须**锚定,不能只认数字裸值 —— 否则日期/年份/序号(2026、300313 等)全被当成
// 「值」参与比对,变成噪音源。只收 A 股快讯里真实承载「一个量」的单位。
//
// 顺序有讲究:`个百分点` 必须排在 `个`/`百分点` 之前(正则交替按左到右匹配最长合理项),
// `万亿元` 排在 `亿元` 前,`万元` 排在 `元` 前 —— 否则会截出错误的单位归属。
var conflictUnits = []string{
	"个百分点", "百分点", "万亿元", "亿元", "万元", "万股", "万手", "倍", "家", "只", "元", "%", "％",
}

// conflictNumRe 抽取「数值 + 单位」。数值支持小数(0.25、134.6)。
var conflictNumRe = regexp.MustCompile(
	`(\d+(?:\.\d+)?)\s*(` + strings.Join(conflictUnits, "|") + `)`)

// normalizeUnit 统一单位写法(全角百分号归一为半角),避免同单位被当成两个。
func normalizeUnit(u string) string {
	if u == "％" {
		return "%"
	}
	return u
}

// factNumbers 抽出一条事实句里的「单位 → 值集合」。
// 同一句里同一单位出现多个值(如「同比增长40%，环比增长30%」)会得到 2 个值 ——
// 这种**多值**在冲突判定里是「口径不同」的信号,按规则不报警(见 DetectFactConflicts)。
func factNumbers(s string) map[string]map[float64]bool {
	out := make(map[string]map[float64]bool)
	for _, m := range conflictNumRe.FindAllStringSubmatch(s, -1) {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		u := normalizeUnit(m[2])
		if out[u] == nil {
			out[u] = make(map[float64]bool)
		}
		out[u][v] = true
	}
	return out
}

// factSkeleton 去掉「数值 + 单位」后的句子骨架,用于判断两句话是否在说**同一个量**。
//
// ⚠️ 只删数字,不删汉字 —— 这是本规则最关键的一处。把汉字也删掉虽然能提高改写鲁棒性,
// 但会让「拟回购5000万股」与「拟增持3000万股」骨架全同(实测 J 从 0.40 冲到 1.00),
// 「回购」vs「增持」这类**不同动作**随即变成误报。保留汉字让动作/指标词留在骨架里。
func factSkeleton(s string) string {
	return conflictNumRe.ReplaceAllString(s, "")
}

// Conflict 一条数值冲突:同一单位上,两侧各自给出**唯一且不同**的值。
//
// SentenceA/SentenceB 是双方**原文**。前端必须两条都显示(issue #49 红线「留双源原文,
// 禁止静默择一」)—— 只给一个版本等于替人做了选择。
type Conflict struct {
	Unit      string    `json:"unit"`
	Values    []float64 `json:"values"` // 两侧值,升序
	SentenceA string    `json:"sentence_a"`
	SentenceB string    `json:"sentence_b"`
}

// conflictGate 骨架相似度门控:低于此不算「同一句话」,不进入数值比对。
//
// 取值依据 = 对**生产真实语料**(669 events / 1410 条事实句,2026-09-20 实测,可复现:
// 见 docs/phase11/design/event-cross-source-conflict.md §4 的取证命令)扫门控:
//
//	随机句对 20 万次抽样的误报率    对抗集误报    真阳性
//	0.0 → 1.6555%                  3/4           5/5
//	0.5 → 0.1305%                  2/4           4/5
//	0.6 → 0.0750%                  0/4           3/5   ← 取此值:误报归零的第一个点
//	0.7 → 0.0555%                  0/4           3/5
//	0.8 → 0.0320%                  0/4           3/5
//
// ⚠️ **已知召回边界(不掩饰)**:0.6 处真阳性为 3/5 —— 两条**改写幅度大**的同量异值句
// (如「此举预计释放长期资金3000亿元」vs「预计可释放长期资金约5000亿元」)骨架 J 仅
// 0.47~0.50,与「不同年份营收」(J=0.50)、「回购 vs 增持」(J=0.40)等**误报**同处一个区间,
// **纯骨架比对无法区分**。要覆盖这类远改写需 LLM 语义比对(网关可用后再议,见 issue #45)。
// 本任务取「宁漏不误」:跨源转载的常态是**近同改写**(骨架 J≈1.0,本规则稳报),
// 远改写属少数,漏掉不会误导,误报会。
const conflictGate = 0.6

// sentencePair 找 a 在 b 中最匹配的一句(骨架 Jaccard 最高),并返回该相似度。
func bestMatch(a string, b []string) (string, float64) {
	var best string
	var bestV float64
	ab := Bigrams(NormalizeTitle(factSkeleton(a)))
	for _, s := range b {
		v := Jaccard(ab, Bigrams(NormalizeTitle(factSkeleton(s))))
		if v > bestV {
			best, bestV = s, v
		}
	}
	return best, bestV
}

// DetectFactConflicts 对同一真实事件的两个成员事件,逐句配对,检出数值冲突。
//
// 规则(每条都对应一次实测,勿凭直觉改):
//  1. A 的每句在 B 中找骨架最像的一句;
//  2. 门控 conflictGate 之上才算「说同一个量」;
//  3. 该单位两侧**各自恰好 1 个值**且**不同** → 报冲突;任一侧多值 → 不报。
//
// 第 3 条的「多值不报」是刻意的**宁漏不误**:一句里同单位出现两个数,通常是并列口径
// (同比 vs 环比)而非分歧,强报会制造噪音。
func DetectFactConflicts(factsA, factsB []string) []Conflict {
	return detectFactConflicts(factsA, factsB, conflictGate)
}

// detectFactConflicts 带门控参数的实现,便于离线标定在真实语料上扫描门控
// (门控取值依据见 conflictGate 注释;标定方法见 docs/phase11/design/event-cross-source-conflict.md §4)。
func detectFactConflicts(factsA, factsB []string, gate float64) []Conflict {

	var out []Conflict
	for _, a := range factsA {
		b, score := bestMatch(a, factsB)
		if b == "" || score < gate {
			continue
		}
		na, nb := factNumbers(a), factNumbers(b)
		// seen:conflictUnits 含同义写法(如 `%` 与 `％`),归一后是同一个单位 ——
		// 不按归一后的键去重会把同一处冲突重复报两遍。
		seen := make(map[string]bool, len(conflictUnits))
		for _, u := range conflictUnits {
			unit := normalizeUnit(u)
			if seen[unit] {
				continue
			}
			seen[unit] = true
			va, vb := na[unit], nb[unit]
			// 两侧都必须恰好一个值,且不同。
			if len(va) != 1 || len(vb) != 1 {
				continue
			}
			var fa, fb float64
			for v := range va {
				fa = v
			}
			for v := range vb {
				fb = v
			}
			if fa == fb {
				continue
			}
			vs := []float64{fa, fb}
			sort.Float64s(vs)
			out = append(out, Conflict{
				Unit: unit, Values: vs,
				SentenceA: a, SentenceB: b,
			})
		}
	}
	return out
}
