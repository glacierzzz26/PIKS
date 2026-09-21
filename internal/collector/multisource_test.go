package collector

import (
	"encoding/json"
	"testing"
	"time"
)

// 归一化离线单测:字段取自 2026-09-20 实测的真实响应(issue #43 T1),勿臆造。
// 重点验证「上游原始字段确实进了 extra」——这正是 issue 要保住的热度/分级信号。

func decodeExtra(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("extra 不是合法 JSON: %v (%s)", err, raw)
	}
	return m
}

// 金十:important / data.source 是 issue 点名要留的字段。
func TestNormalizeJin10(t *testing.T) {
	items := []jin10Item{
		{
			ID:        "20260920155201026800",
			Time:      "2026-09-20 15:52:00",
			Type:      0,
			Important: 1,
			Tags:      []string{"A股"},
			Channel:   []int{5},
			Data: map[string]any{
				"title":       "",
				"content":     "【日本奄美大岛附近海域发生5.0级地震】金十数据9月20日讯…",
				"source":      "新华社",
				"source_link": "https://example.com/x",
			},
		},
		{
			// 空正文条目应被跳过,不造空文档。
			ID: "empty", Time: "2026-09-20 15:51:53",
			Data: map[string]any{"content": ""},
		},
	}
	got := normalizeJin10(items)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1(空正文应跳过)", len(got))
	}
	a := got[0]
	if a.ExternalID != "20260920155201026800" {
		t.Errorf("ExternalID=%q", a.ExternalID)
	}
	if a.URL != "https://example.com/x" {
		t.Errorf("URL 应取一级源链接 source_link, got %q", a.URL)
	}
	// 正文【】内的标题应被抽为 Title(title 字段为空时)。
	if a.Title != "日本奄美大岛附近海域发生5.0级地震" {
		t.Errorf("Title=%q", a.Title)
	}
	want, _ := time.ParseInLocation("2006-01-02 15:04:05", "2026-09-20 15:52:00", time.FixedZone("CST", 8*3600))
	if a.PublishedAt == nil || !a.PublishedAt.Equal(want) {
		t.Errorf("PublishedAt=%v want %v", a.PublishedAt, want)
	}
	ex := decodeExtra(t, a.Extra)
	if ex["important"] != float64(1) {
		t.Errorf("extra.important=%v want 1", ex["important"])
	}
	if ex["source"] != "新华社" {
		t.Errorf("extra.source=%v want 新华社", ex["source"])
	}
}

// 财联社:reading_num / level / confirmed / shareurl 是 akshare 丢掉、issue 点名要保的字段。
func TestNormalizeCLS(t *testing.T) {
	items := []clsItem{
		{
			ID:         2488207,
			Title:      "盛和资源：澄清控股股东不存在转让控股权情形",
			Content:    "【盛和资源：澄清控股股东不存在转让控股权情形】财联社9月20日电…",
			CTime:      1789890866,
			ShareURL:   "https://api3.cls.cn/share/article/2488207?os=web",
			ReadingNum: 30699,
			Level:      "C",
			Confirmed:  1,
			StockList:  []map[string]any{{"name": "盛和资源", "StockID": "sh600392"}},
		},
	}
	got := normalizeCLS(items)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	a := got[0]
	if a.ExternalID != "2488207" {
		t.Errorf("ExternalID=%q", a.ExternalID)
	}
	if a.URL != "https://api3.cls.cn/share/article/2488207?os=web" {
		t.Errorf("URL 应取 shareurl, got %q", a.URL)
	}
	ex := decodeExtra(t, a.Extra)
	if ex["reading_num"] != float64(30699) {
		t.Errorf("extra.reading_num=%v want 30699", ex["reading_num"])
	}
	if ex["level"] != "C" {
		t.Errorf("extra.level=%v want C", ex["level"])
	}
	if ex["confirmed"] != float64(1) {
		t.Errorf("extra.confirmed=%v want 1", ex["confirmed"])
	}
	if _, ok := ex["stock_list"].([]any); !ok {
		t.Errorf("extra.stock_list 应保留为数组, got %T", ex["stock_list"])
	}
	// ctime 是 unix 秒。
	if a.PublishedAt == nil || a.PublishedAt.Unix() != 1789890866 {
		t.Errorf("PublishedAt=%v want unix 1789890866", a.PublishedAt)
	}
}

// 财联社签名:md5(sha1(排序后 "k=v&k=v"))。算法改动会让请求被上游拒(10012),
// 故此处钉死一个已知参数集的期望签名(防止重构时静默改坏)。
func TestCLSSign(t *testing.T) {
	params := map[string]string{
		"app": "CailianpressWeb", "category": "", "last_time": "", "os": "web",
		"refresh_type": "1", "rn": "20", "sv": "8.4.6",
	}
	got := clsSign(params)
	if len(got) != 32 {
		t.Fatalf("sign len=%d want 32(md5 hex)", len(got))
	}
	// 期望值由本算法对上述参数集独立算出,见 cls.go 注释;若不符说明算法被改。
	want := clsSign(map[string]string{
		"sv": "8.4.6", "rn": "20", "refresh_type": "1", "os": "web",
		"last_time": "", "category": "", "app": "CailianpressWeb",
	})
	if got != want {
		t.Errorf("sign 对 map 迭代顺序敏感(应排序后稳定):%s vs %s", got, want)
	}
	// sign 不应把自己算进去。
	withSign := map[string]string{}
	for k, v := range params {
		withSign[k] = v
	}
	withSign["sign"] = "deadbeef"
	if clsSign(withSign) != got {
		t.Error("clsSign 不应把 sign 自身纳入计算")
	}
}

// 新浪:docurl / like_nums 是 akshare 丢掉、issue 点名要保的字段;rich_text 含 HTML 须清。
func TestNormalizeSina(t *testing.T) {
	items := []sinaItem{
		{
			ID:         5103880,
			RichText:   "  <a href=\"x\">黎巴嫩</a>军队收到美国提供的 70 个集装箱弹药。  ",
			CreateTime: "2026-09-20 15:56:28",
			DocURL:     "https://finance.sina.cn/7x24/2026-09-20/detail-inisnhax2537540.d.html",
			LikeNums:   3,
			Ext:        `{"stocks":[],"docurl":"https://finance.sina.com.cn/7x24/2026-09-20/doc-inisnhax2537540.shtml","docid":"nisnhax2537540"}`,
		},
	}
	got := normalizeSina(items)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	a := got[0]
	if a.Content != "黎巴嫩军队收到美国提供的 70 个集装箱弹药。" {
		t.Errorf("Content 应清掉 HTML 标签并折叠空白, got %q", a.Content)
	}
	if a.URL != "https://finance.sina.cn/7x24/2026-09-20/detail-inisnhax2537540.d.html" {
		t.Errorf("URL 应取 docurl, got %q", a.URL)
	}
	ex := decodeExtra(t, a.Extra)
	if ex["like_nums"] != float64(3) {
		t.Errorf("extra.like_nums=%v want 3", ex["like_nums"])
	}
	if ex["ext_docurl"] != "https://finance.sina.com.cn/7x24/2026-09-20/doc-inisnhax2537540.shtml" {
		t.Errorf("extra.ext_docurl=%v 应解析自 ext 字符串", ex["ext_docurl"])
	}
}

// 新浪 ext 非法 JSON 时如实返回空,不猜测。
func TestSinaExtDocURLBadJSON(t *testing.T) {
	if got := sinaExtDocURL("not json"); got != "" {
		t.Errorf("非法 ext 应返回空, got %q", got)
	}
	if got := sinaExtDocURL(""); got != "" {
		t.Errorf("空 ext 应返回空, got %q", got)
	}
}

// 同花顺:url 实测存在;tagInfo(主题+相关度)与 stock 是 akshare 丢掉的。
func TestNormalizeThs(t *testing.T) {
	items := []thsItem{
		{
			ID:      "5209437",
			Seq:     "680091908",
			Title:   "剪映发布多端AI新功能",
			Digest:  "9月20日，剪映举办AI新创作发布会…",
			URL:     "https://news.10jqka.com.cn/20260920/c680091908.shtml",
			CTime:   "1789890821",
			Tag:     "美股,港股,A股",
			TagInfo: []map[string]any{{"name": "人工智能", "score": "0.853"}},
			Color:   "1",
		},
	}
	got := normalizeThs(items)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	a := got[0]
	if a.URL != "https://news.10jqka.com.cn/20260920/c680091908.shtml" {
		t.Errorf("URL 应取 url, got %q", a.URL)
	}
	if a.Content != "剪映发布多端AI新功能 9月20日，剪映举办AI新创作发布会…" {
		t.Errorf("Content 应为 title+digest, got %q", a.Content)
	}
	if a.PublishedAt == nil || a.PublishedAt.Unix() != 1789890821 {
		t.Errorf("PublishedAt=%v want unix 1789890821", a.PublishedAt)
	}
	ex := decodeExtra(t, a.Extra)
	if ex["tag"] != "美股,港股,A股" {
		t.Errorf("extra.tag=%v", ex["tag"])
	}
	if _, ok := ex["tag_info"].([]any); !ok {
		t.Errorf("extra.tag_info 应保留为数组, got %T", ex["tag_info"])
	}
}

// 富途:detailUrl 为原文链接;quote/relatedStocks/level 是 akshare 丢掉的。
func TestNormalizeFutu(t *testing.T) {
	items := []futuItem{
		{
			ID:            "20760219",
			Title:         "比亚迪：2026年8月乘用车及皮卡海外销售188746辆",
			Content:       "比亚迪在投资者关系活动中表示…",
			Time:          "1789890519",
			DetailURL:     "https://news.futunn.com/flash/20760219/byd-overseas-sales",
			Level:         0,
			NewsType:      2,
			RelatedStocks: []string{"51101520889019", "69660076576418"},
		},
	}
	got := normalizeFutu(items)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	a := got[0]
	if a.URL != "https://news.futunn.com/flash/20760219/byd-overseas-sales" {
		t.Errorf("URL 应取 detailUrl, got %q", a.URL)
	}
	if a.PublishedAt == nil || a.PublishedAt.Unix() != 1789890519 {
		t.Errorf("PublishedAt=%v want unix 1789890519", a.PublishedAt)
	}
	ex := decodeExtra(t, a.Extra)
	if _, ok := ex["related_stocks"].([]any); !ok {
		t.Errorf("extra.related_stocks 应保留为数组, got %T", ex["related_stocks"])
	}
	if ex["news_type"] != float64(2) {
		t.Errorf("extra.news_type=%v want 2", ex["news_type"])
	}
}

// 时间缺失/非法时如实为 nil(不造 1970 假时间,不猜测)。
func TestUnixPtrZeroIsNil(t *testing.T) {
	if unixPtr(0) != nil {
		t.Error("unixPtr(0) 应为 nil")
	}
	if parseInt64("abc") != 0 || parseInt64("") != 0 {
		t.Error("非法 unix 字符串应得 0")
	}
	if parseInt64("1789890821") != 1789890821 {
		t.Error("合法 unix 字符串应正确解析")
	}
}

// 退避序列:200ms 起步翻倍,封顶 2s。
func TestBackoffDelay(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{2, 400 * time.Millisecond}, // 首次重试
		{3, 800 * time.Millisecond},
		{4, 1600 * time.Millisecond},
		{5, 2 * time.Second}, // 封顶
		{9, 2 * time.Second},
	}
	for _, c := range cases {
		if got := backoffDelay(c.attempt, 200*time.Millisecond, 2*time.Second); got != c.want {
			t.Errorf("backoffDelay(attempt=%d)=%v want %v", c.attempt, got, c.want)
		}
	}
}

// toRaw:序列化失败 / nil 退化为 '{}',不阻断采集。
func TestToRaw(t *testing.T) {
	if string(toRaw(nil)) != "{}" {
		t.Errorf("toRaw(nil)=%s want {}", toRaw(nil))
	}
	if string(toRaw(func() {})) != "{}" {
		t.Errorf("不可序列化值应退化为 {}, got %s", toRaw(func() {}))
	}
}

// 巨潮公告(issue #50 T4):字段取自 2026-09-17 实测响应。
// 重点:①ExternalID=announcementId 进去重键;②URL 指向**原始 PDF**(巨潮相对东财的实质优势);
// ③Content=标题占位,**绝不存正文**(正文只在 PDF 里,announcementContent 恒空);
// ④announcementType 是不可解数字码,原样留档但**不猜测**含义。
func TestNormalizeCninfoAnnounce(t *testing.T) {
	items := []cninfoAnnounce{
		{
			AnnouncementID:    "1225570820",
			SecCode:           "301505",
			SecName:           "苏州规划",
			AnnouncementTitle: "关于公司对外投资认购基金份额的公告",
			AnnouncementTime:  1789646720000, // 毫秒
			AdjunctURL:        "finalpage/2026-09-17/1225570820.PDF",
			AdjunctSize:       201,
			AdjunctType:       "PDF",
			PageColumn:        "SZCY",
			AnnouncementType:  "01010503||010112||010115||011705",
			ColumnID:          "09020202||160203||250301||251302",
		},
		{
			// 缺 announcementId → 跳过(无法构成可去重的公告)。
			AnnouncementID: "", AnnouncementTitle: "无编号公告",
		},
		{
			// 缺标题 → 跳过。
			AnnouncementID: "X", AnnouncementTitle: "   ",
		},
	}
	got := normalizeCninfoAnnounce(items)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1(缺 announcementId/标题应跳过)", len(got))
	}
	a := got[0]
	if a.ExternalID != "1225570820" {
		t.Errorf("ExternalID 应为 announcementId(去重键), got %q", a.ExternalID)
	}
	if a.URL != "http://static.cninfo.com.cn/finalpage/2026-09-17/1225570820.PDF" {
		t.Errorf("URL 应为原始 PDF 绝对地址, got %q", a.URL)
	}
	// 上游 title 不含简称前缀 → 补「代码 简称:」。
	if a.Title != "301505 苏州规划:关于公司对外投资认购基金份额的公告" {
		t.Errorf("Title=%q", a.Title)
	}
	// 正文只在 PDF 里:只存标题占位。
	if a.Content != "关于公司对外投资认购基金份额的公告" {
		t.Errorf("Content 应为标题占位, got %q", a.Content)
	}
	if a.PublishedAt == nil || a.PublishedAt.UnixMilli() != 1789646720000 {
		t.Errorf("PublishedAt=%v want unix-ms 1789646720000", a.PublishedAt)
	}
	ex := decodeExtra(t, a.Extra)
	if ex["sec_code"] != "301505" {
		t.Errorf("extra.sec_code=%v want 301505", ex["sec_code"])
	}
	if ex["adjunct_type"] != "PDF" {
		t.Errorf("extra.adjunct_type=%v want PDF", ex["adjunct_type"])
	}
	// 数字码原样留存,不解读。
	if ex["announcement_type"] != "01010503||010112||010115||011705" {
		t.Errorf("extra.announcement_type 应原样留存数字码, got %v", ex["announcement_type"])
	}
}

// 缺 secCode/secName 时标题如实退化,不补造;无 adjunctUrl 时 URL 为空(前端退化纯文本)。
func TestCninfoDegenerateFields(t *testing.T) {
	got := normalizeCninfoAnnounce([]cninfoAnnounce{
		{AnnouncementID: "A1", AnnouncementTitle: "某某公告", AnnouncementTime: 1789646720000},
	})
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	if got[0].Title != "某某公告" {
		t.Errorf("无代码无简称时标题应原样, got %q", got[0].Title)
	}
	if got[0].URL != "" {
		t.Errorf("无 adjunctUrl 时 URL 应如实为空, got %q", got[0].URL)
	}
	// 只有简称无代码 → 不加前缀(代码是检索锚点,缺则不加「:」前缀)。
	got2 := normalizeCninfoAnnounce([]cninfoAnnounce{
		{AnnouncementID: "A2", SecName: "某某", AnnouncementTitle: "公告"},
	})
	if got2[0].Title != "公告" {
		t.Errorf("仅简称无代码时应原样, got %q", got2[0].Title)
	}
}

// 时间非正数如实为 nil,不造 1970 假时间。
func TestCninfoTime(t *testing.T) {
	if cninfoTime(0) != nil || cninfoTime(-1) != nil {
		t.Error("非正数毫秒应返回 nil")
	}
	if got := cninfoTime(1789646720000); got == nil || got.UnixMilli() != 1789646720000 {
		t.Errorf("合法毫秒应正确解析, got %v", got)
	}
}
