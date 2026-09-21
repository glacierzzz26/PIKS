package collector

// cninfoAnnounceDriver 巨潮资讯网公告驱动(证监会指定披露网站)。issue #50 / #43 T4。
//
// 端点与字段经本次实测(2026-09-21),勿凭想象改字段:
//   POST http://www.cninfo.com.cn/new/hisAnnouncement/query
//   Content-Type: application/x-www-form-urlencoded; charset=UTF-8
//   Header: Referer: http://www.cninfo.com.cn/new/commonUrl?url=disclosure/list/notice
//   body: pageNum={n}&pageSize=30&column=szse&plate=szsh&tabName=fulltext&stock=&searchkey=
//         &secid=&category=&trade=&seDate={Y-m-d}~{Y-m-d}&sortName=&sortType=&isHLtitle=true
//   → {totalAnnouncement,totalRecordNum,announcements[],classifiedAnnouncements,totalSecurities}
//     announcements[].{announcementId,secCode,secName,announcementTitle,announcementTime(ms),
//                       adjunctUrl,adjunctSize,adjunctType,pageColumn,announcementType,
//                       columnId,announcementContent,announcementTypeName}
//
// ⚠️ **必须 POST**(实测同参数 GET 返回 500 HTML 页);故用 httpSource.postFormJSON。
//
// ⚠️ **scope 参数实测(2026-09-17,单日 totalAnnouncement)**:
//   column=szse&plate=szsh → 1184   ← 本驱动采用
//   column=&plate=         → 1945   （额外 761 条为基金/债券/港股等,见 pageColumn=HKZB）
//   column=szse&plate=sz|sh|bj → 625 / 476 / 83（三者相加=1184,故 szsh 即 深+沪+京 A股）
//   **column=szse 是「仅股票」过滤器**(非「深市」)—— 去掉它会混入 761 条非个股公告。
//   本驱动定位为**个股公告**,故取 column=szse&plate=szsh。
//
// ⚠️ **pageSize 上限实测为 30**(请求 50/100 均只回 30 条)。单日 1184 条 = 40 页,
//   按 minGap=1s 约 40s/日,压力可忽略。
//
// ⚠️ 只存标题 + 外链,**不存正文**:上游 announcementContent 实测恒为空(正文只在 PDF 里),
//   且 PDF 解析成本/上游友好度都不划算。adjunctUrl 指向**原始 PDF**
//   (http://static.cninfo.com.cn/{adjunctUrl},实测 200/application/pdf/204KB),
//   比网页详情页更可直接引用 —— 这是巨潮相对东财的一个实质优势。
//
// ⚠️ announcementType 是**不可解数字码**(如 "01010503||010112||010115||011705"),
//   announcementTypeName 实测为 null,字典端点不可得。故 extra 原样留存类型码,
//   **不猜测**其含义;columnId 同为数字码,一并留存备后续解码。
//
// 归一化:ExternalID=announcementId(上游稳定标识,进去重键,见迁移 0016),
// Content=标题(占位,满足 content NOT NULL;如实不造正文),PublishedAt=announcementTime。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	cninfoQueryURL = "http://www.cninfo.com.cn/new/hisAnnouncement/query"
	cninfoPDFBase  = "http://static.cninfo.com.cn/"
)

type cninfoAnnounceDriver struct {
	http     *httpSource
	date     string // 目标交易日 YYYY-MM-DD;空 = 用运行日(北京时间)
	pageSize int    // 上游硬上限 30,实测再大也只回 30
	maxPages int    // 分页护栏
}

func newCninfoAnnounceDriver() *cninfoAnnounceDriver {
	return &cninfoAnnounceDriver{
		// 单日 40 页;间隔 1s ≈ 40s/日。巨潮是官方指定披露站、承载量远大于免费财经接口,
		// 无需像东财正文接口那样退到 2s+;仍保留重试退避。
		http:     newHTTPSource(20*time.Second, 1*time.Second, 3),
		pageSize: 30,
		maxPages: 120, // 30*120=3600,远超单日量,纯护栏
	}
}

func (d *cninfoAnnounceDriver) Name() string { return "cninfo-announce" }

// cninfoResp 真实 DTO(实测 2026-09-21)。
type cninfoResp struct {
	TotalAnnouncement int              `json:"totalAnnouncement"`
	TotalRecordNum    int              `json:"totalRecordNum"`
	Announcements     []cninfoAnnounce `json:"announcements"`
}

type cninfoAnnounce struct {
	AnnouncementID    string `json:"announcementId"`
	SecCode           string `json:"secCode"`
	SecName           string `json:"secName"`
	AnnouncementTitle string `json:"announcementTitle"`
	AnnouncementTime  int64  `json:"announcementTime"` // unix 毫秒
	AdjunctURL        string `json:"adjunctUrl"`       // 相对路径,拼 cninfoPDFBase
	AdjunctSize       int    `json:"adjunctSize"`      // KB
	AdjunctType       string `json:"adjunctType"`      // PDF
	PageColumn        string `json:"pageColumn"`       // SZCY/SZzb/HKZB 等板块标记
	AnnouncementType  string `json:"announcementType"` // 数字码,不可解
	ColumnID          string `json:"columnId"`         // 数字码,不可解
}

// Fetch 按 totalAnnouncement 翻页取完当日公告。翻页中单页失败即返回已取数据 + 错误
// (由 cmd/collector 决定是否暂停该源;绝不猜测或补造数据)。
func (d *cninfoAnnounceDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	date := d.date
	if date == "" {
		date = time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02")
	}
	var out []RawNews
	for page := 1; page <= d.maxPages; page++ {
		r, err := d.fetchPage(ctx, date, page)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("cninfo-announce: %w", err)
			}
			return out, fmt.Errorf("cninfo-announce: page %d: %w", page, err)
		}
		out = append(out, normalizeCninfoAnnounce(r.Announcements)...)
		// 取完即止:已取条数 ≥ totalAnnouncement,或本页不足一页(尾页)。
		if len(out) >= r.TotalAnnouncement || len(r.Announcements) < d.pageSize {
			break
		}
	}
	observeFetch(cninfoQueryURL, len(out))
	return out, nil
}

func (d *cninfoAnnounceDriver) fetchPage(ctx context.Context, date string, page int) (*cninfoResp, error) {
	// 用 url.Values 编码:seDate 含空格与 `~`,不编码会被上游拒。
	vals := url.Values{}
	vals.Set("pageNum", fmt.Sprintf("%d", page))
	vals.Set("pageSize", fmt.Sprintf("%d", d.pageSize))
	vals.Set("column", "szse") // 仅股票(非「深市」,见文件头实测)
	vals.Set("plate", "szsh")  // 深+沪+京 A股
	vals.Set("tabName", "fulltext")
	vals.Set("seDate", date+"~"+date)
	vals.Set("sortName", "")
	vals.Set("sortType", "")
	vals.Set("isHLtitle", "true")

	body, err := d.http.postFormJSON(ctx, cninfoQueryURL, vals.Encode(), map[string]string{
		"Referer": "http://www.cninfo.com.cn/new/commonUrl?url=disclosure/list/notice",
	})
	if err != nil {
		return nil, err
	}
	var r cninfoResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("bad json: %w", err)
	}
	return &r, nil
}

// normalizeCninfoAnnounce 真实 DTO → 归一化 RawNews(纯函数,可离线单测)。
// 无 announcementId 或无标题的行跳过 —— 二者缺一都无法构成可读、可去重的公告。
func normalizeCninfoAnnounce(items []cninfoAnnounce) []RawNews {
	out := make([]RawNews, 0, len(items))
	for _, it := range items {
		title := strings.TrimSpace(it.AnnouncementTitle)
		if it.AnnouncementID == "" || title == "" {
			continue
		}
		out = append(out, RawNews{
			ExternalID:  it.AnnouncementID,
			URL:         cninfoAdjunctURL(it.AdjunctURL),
			Title:       cninfoTitle(it, title),
			Content:     title, // 占位:正文只在 PDF 里(announcementContent 恒空)。content NOT NULL 由此满足。
			PublishedAt: cninfoTime(it.AnnouncementTime),
			Extra: toRaw(map[string]any{
				"announcement_id": it.AnnouncementID,
				"sec_code":        it.SecCode,
				"sec_name":        it.SecName,
				"page_column":     it.PageColumn,
				// 数字码原样留存;**不猜测**含义(announcementTypeName 实测 null)。
				"announcement_type": it.AnnouncementType,
				"column_id":         it.ColumnID,
				"adjunct_type":      it.AdjunctType,
				"adjunct_url":       it.AdjunctURL,
				"adjunct_size":      it.AdjunctSize,
			}),
		})
	}
	return out
}

// cninfoTitle 拼一行可读标题:"<代码> <简称>:<标题>"(含代码便于检索/对齐个股中心)。
// 上游 announcementTitle 实测**不含简称前缀**(形如「关于公司对外投资认购基金份额的公告」),
// 故补上「代码 简称:」;代码/简称缺失时如实退化,不补造。
func cninfoTitle(it cninfoAnnounce, title string) string {
	if it.SecCode == "" {
		return title
	}
	if it.SecName == "" {
		return it.SecCode + " " + title
	}
	return it.SecCode + " " + it.SecName + ":" + title
}

// cninfoAdjunctURL 拼原文 PDF 绝对地址(实测 200/application/pdf)。
// 无 adjunctUrl 时如实为空,由前端 SourceLink 渲染为纯文本(不死链)。
func cninfoAdjunctURL(adjunct string) string {
	adjunct = strings.TrimSpace(adjunct)
	if adjunct == "" {
		return ""
	}
	return cninfoPDFBase + strings.TrimPrefix(adjunct, "/")
}

// cninfoTime unix 毫秒 → *time.Time;非正数如实为 nil(不造 1970 假时间)。
func cninfoTime(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms)
	return &t
}
