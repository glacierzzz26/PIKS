package collector

// 热榜驱动(issue #68 D 层):A 股**事件/题材**热榜 —— 独立于 RawNews 采集链路。
//
// 🔴 与快讯/公告驱动**刻意不同**:热榜**不进 raw_documents、不进聚类**(设计 §6 红线)。
//   故本文件不复用 Driver/RawNews 接口,另立 HotTopicSource → []HotTopicItem,
//   由 cmd/hot-topic 落**独立表** hot_topic_items(迁移 0018)。这是**故意**的隔离:
//   凡把热榜塞进 raw_documents 的改动,都等于把「可被操纵的热度」混进事件印证度。
//
// 源选型(issue #68 评论 2026-09-21 实测,双端验证;勿凭想象增源):
//
//	| 源            | 端点                                              | 条数 | 热度值        |
//	| 同花顺话题榜  | dq.10jqka.com.cn/.../hot_list/v1/topic            | 15(硬上限) | hot_value 数值 |
//	| 财联社热文    | 首页 SSR __NEXT_DATA__.pageProps.hotArticleData   | 13(硬上限) | readNum 数值   |
//	其余候选均不可用:金十无热榜端点、东财/雪球/百度 403/WAF/空、
//	财联社话题是**栏目名**不是事件(「环球市场情报」)。
//
// 🔴 关键实测:两源**同题对 = 0**(生产聚类同尺 NormalizeTitle+Jaccard@0.7 比对 615 对,
//   J≥0.5 都是 0)。根因:同花顺是**题材/事件**、财联社热文是**文章/复盘**、粒度不同。
//   → 故**各出各的、不合并、不加权、不排名**(方案 A);混排必然误导。
//   本文件**不提供**任何跨源合并/加权函数 —— 那是被实测否决的方案 B。
//
// ⚠️ 护栏复用:C 层 per-host 三护栏(令牌桶/空响应哨兵/熔断)经 httpSource 自动生效。
// ⚠️ 结构漂移须**报错而非空成功**(#64 教训):财联社靠解析首页 SSR,结构一变就拿不到
//   数据,此时**必须返回 error**让 task_runs 记 failed,绝不静默退化成「采到 0 条 = 成功」。

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// 源标识(落 hot_topic_items.source 的枚举;前端按此分列/显示名)。
const (
	HotSourceThs = "ths-topic"       // 同花顺话题榜
	HotSourceCLS = "cls-hot-article" // 财联社首页热文
)

// HotTopicItem 热榜一条(归一化,与具体源无关)。
//
// ⚠️ Rank 是**该源榜内**名次;HotValue 是**该源口径**的热度值 —— 两者**仅源内可比**,
// 跨源不可比(同花顺 hot_value 与财联社 readNum 量纲不同,且各源自身尺度也不同)。
// 展示层必须分列、不得跨源排名(设计 §6 红线)。
type HotTopicItem struct {
	Source   string          // HotSource* 之一
	Rank     int             // 榜内名次(1-based)
	Title    string          // 话题标题(原文,不改写)
	HotValue *int64          // 该源口径热度;nil = 上游未给(不填 0)
	URL      string          // 原文/话题链接;空 = 上游没给可点链接
	Extra    json.RawMessage // 上游原始字段原样留档(关联个股 / brief·author·ctime)
}

// HotTopicSource 热榜源适配器。Name 返回 HotSource* 枚举(落库的 source 列)。
type HotTopicSource interface {
	Name() string
	Fetch(ctx context.Context) ([]HotTopicItem, error)
}

// NewHotTopicSources 返回全部热榜源(顺序即展示顺序:同花顺在前)。
// 新增源须同步:hot_topic_items.source 枚举 + 前端 hotTopicSources 显示名(单一真源在此)。
func NewHotTopicSources() []HotTopicSource {
	return []HotTopicSource{newThsTopicSource(), newCLSHotArticleSource()}
}

// ---- 源 1:同花顺话题榜 ----

const thsTopicURL = "https://dq.10jqka.com.cn/fuyao/hot_list_data/out/hot_list/v1/topic?stock_type=a&type=day"

type thsTopicSource struct {
	http *httpSource
}

func newThsTopicSource() *thsTopicSource {
	// dq.10jqka.com.cn 是**新 host**(快讯走 news.10jqka.com.cn,不同源);
	// 护栏按 host 分组,故本源独占一份 per-host 状态,不会与快讯互相拖累。
	return &thsTopicSource{http: newHTTPSource(15*time.Second, 2*time.Second, 3)}
}

func (s *thsTopicSource) Name() string { return HotSourceThs }

// thsTopicResp 真实 DTO(实测 2026-09-22)。attach_info 用 map 承接以原样留档。
type thsTopicResp struct {
	StatusCode int          `json:"status_code"` // 成功为 0
	Data       *thsTopicBag `json:"data"`
}

type thsTopicBag struct {
	TopicList []thsTopicItem `json:"topic_list"`
}

type thsTopicItem struct {
	Code       string         `json:"code"`
	Title      string         `json:"title"`
	Desc       string         `json:"description"`
	Type       string         `json:"type"`        // 恒为 "topic"
	AttachType string         `json:"attach_type"` // att_stock / att_vote_id / att_sub_title
	JumpURL    string         `json:"jump_url"`    // //t.10jqka.com.cn/... (协议相对)
	HotValue   int64          `json:"hot_value"`
	AttachInfo map[string]any `json:"attach_info"` // att_stock 时含关联个股
}

func (s *thsTopicSource) Fetch(ctx context.Context) ([]HotTopicItem, error) {
	body, err := s.http.getJSON(ctx, thsTopicURL, map[string]string{
		"Referer": "https://t.10jqka.com.cn/",
	})
	if err != nil {
		return nil, fmt.Errorf("ths-topic: %w", err)
	}
	var r thsTopicResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("ths-topic: bad json: %w", err)
	}
	if r.Data == nil {
		return nil, fmt.Errorf("ths-topic: 响应缺 data(结构可能已变,勿当空成功)")
	}
	items := normalizeThsTopic(r.Data.TopicList)
	observeFetch(thsTopicURL, len(items))
	return items, nil
}

// normalizeThsTopic 真实 DTO → 归一化(纯函数,可离线单测)。
//
// ⚠️ rank = **数组下标 + 1**(上游榜位),不是产出后的重新编号:上游数组已按热度降序,
//
//	下标即榜位。若某行因空标题被跳过,后续行的 rank **保留原榜位**(出现空号),
//	**不得重排** —— 重排会把「上游第 3 名」谎报成「第 2 名」,名次本身是事实。
func normalizeThsTopic(list []thsTopicItem) []HotTopicItem {
	out := make([]HotTopicItem, 0, len(list))
	for i, it := range list {
		title := strings.TrimSpace(it.Title)
		if title == "" {
			continue // 无标题无法展示;跳过而非造一条空行
		}
		hv := it.HotValue
		out = append(out, HotTopicItem{
			Source:   HotSourceThs,
			Rank:     i + 1,
			Title:    title,
			HotValue: &hv,
			URL:      thsAbsURL(it.JumpURL),
			Extra: toRaw(map[string]any{
				"code":        it.Code,
				"description": it.Desc,
				"attach_type": it.AttachType,
				"attach_info": it.AttachInfo, // 含关联个股(att_stock);原样留档
			}),
		})
	}
	return out
}

// thsAbsURL 上游 jump_url 是协议相对("//t.10jqka.com.cn/..."),补 https: 使其可直接点;
// 缺失则返回空(前端如实不渲染链接,不猜)。
func thsAbsURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "//") {
		return "https:" + u
	}
	return u
}

// ---- 源 2:财联社首页热文(SSR 解析) ----

const clsHomeURL = "https://www.cls.cn/"

// clsNextDataRe 抠出 Next.js 的 __NEXT_DATA__ JSON。非贪婪到第一个 </script>。
var clsNextDataRe = regexp.MustCompile(`(?s)<script id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

type clsHotArticleSource struct {
	http *httpSource
}

func newCLSHotArticleSource() *clsHotArticleSource {
	return &clsHotArticleSource{http: newHTTPSource(20*time.Second, 2*time.Second, 3)}
}

func (s *clsHotArticleSource) Name() string { return HotSourceCLS }

// clsNextData 只取到 hotArticleData 这一段(其余字段无用于热榜,少结构化一层)。
// ⚠️ 无独立 API(issue #68 实测):财联社热文只能解析首页 SSR —— 本文件最脆弱的一点。
type clsNextData struct {
	Props struct {
		PageProps struct {
			HotArticleData []clsHotArticle `json:"hotArticleData"`
		} `json:"pageProps"`
	} `json:"props"`
}

type clsHotArticle struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Brief   string `json:"brief"`
	CTime   int64  `json:"ctime"`   // unix 秒
	ReadNum int64  `json:"readNum"` // 阅读数(=真实关注度)
	Author  string `json:"author"`
	Stocks  string `json:"stocks"` // ⚠️ 实测**恒为空字符串**(勿据此以为没数据)
}

func (s *clsHotArticleSource) Fetch(ctx context.Context) ([]HotTopicItem, error) {
	body, err := s.http.getJSON(ctx, clsHomeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("cls-hot-article: %w", err)
	}
	list, err := parseCLSHotArticleHTML(body)
	if err != nil {
		return nil, err
	}
	items := normalizeCLSHotArticle(list)
	observeFetch(clsHomeURL, len(items))
	return items, nil
}

// parseCLSHotArticleHTML 从首页 HTML 抠出热文列表。**结构漂移一律返回 error** ——
// 绝不静默退化成「0 条 = 成功」(#64 教训)。独立成函数便于离线单测(喂真实样片段)。
func parseCLSHotArticleHTML(body []byte) ([]clsHotArticle, error) {
	m := clsNextDataRe.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("cls-hot-article: 首页未找到 __NEXT_DATA__(SSR 结构已变,脆弱点失效)")
	}
	var nd clsNextData
	if err := json.Unmarshal(m[1], &nd); err != nil {
		return nil, fmt.Errorf("cls-hot-article: __NEXT_DATA__ 解析失败: %w", err)
	}
	list := nd.Props.PageProps.HotArticleData
	if len(list) == 0 {
		// 解析成功但空 —— 同样视为失败:财联社首页热文**实测恒为 13 条**,
		// 空要么是上游改版、要么是本轮被限流;如实报错比记「成功 0 条」有用。
		return nil, fmt.Errorf("cls-hot-article: hotArticleData 为空(实测应 ~13 条,疑似改版或限流)")
	}
	return list, nil
}

// normalizeCLSHotArticle 真实 DTO → 归一化(纯函数,可离线单测)。
// rank 语义同 normalizeThsTopic:数组下标 + 1(上游榜位),跳过空行时保留原榜位、不重排。
func normalizeCLSHotArticle(list []clsHotArticle) []HotTopicItem {
	out := make([]HotTopicItem, 0, len(list))
	for i, it := range list {
		title := strings.TrimSpace(it.Title)
		if title == "" {
			continue
		}
		hv := it.ReadNum
		out = append(out, HotTopicItem{
			Source:   HotSourceCLS,
			Rank:     i + 1,
			Title:    title,
			HotValue: &hv,
			// web URL 规则(issue #68 实测):https://www.cls.cn/detail/<id>
			URL: fmt.Sprintf("https://www.cls.cn/detail/%d", it.ID),
			Extra: toRaw(map[string]any{
				"brief":      it.Brief,
				"author":     it.Author,
				"ctime":      it.CTime,
				"article_id": it.ID,
			}),
		})
	}
	return out
}
