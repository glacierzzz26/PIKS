package collector

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// 热榜归一化测试(issue #68 D 层)。**纯函数、零网络** —— 钉住三件事:
//   ① rank 按数组顺序赋号(上游已按热度降序,不得重排);
//   ② HotValue 原样透传(不换算、不跨源比较);
//   ③ 空标题行跳过(不造空行)。

func TestNormalizeThsTopic(t *testing.T) {
	in := []thsTopicItem{
		{Code: "A1", Title: "华字辈大涨", HotValue: 601009, JumpURL: "//t.10jqka.com.cn/x", AttachType: "att_stock",
			AttachInfo: map[string]any{"att_stock": []any{map[string]any{"name": "新华传媒", "code": "600825"}}}},
		{Code: "A2", Title: "  ", HotValue: 500000}, // 空标题 → 跳过(但**不重排**后续 rank?见断言)
		{Code: "A3", Title: "美股全线大涨", HotValue: 909614, JumpURL: "//t.10jqka.com.cn/y"},
	}
	out := normalizeThsTopic(in)
	if len(out) != 2 {
		t.Fatalf("空标题应被跳过: want 2, got %d", len(out))
	}
	if out[0].Source != HotSourceThs {
		t.Errorf("source: got %q, want %q", out[0].Source, HotSourceThs)
	}
	// rank = 数组下标+1(**上游榜位**),不是产出后的重编号 —— 空行不占产出、但**不留白**?
	// 不:本实现保留原榜位(i+1),故跳过第 2 行后剩下的 rank 是 1 与 3(出现空号)。
	// 断言钉死这一行为:**名次是事实,不得因展示跳过而重排**。
	if out[0].Rank != 1 || out[1].Rank != 3 {
		t.Errorf("rank 应保留上游榜位 1 与 3(第 2 行空标题被跳,不得重排);got %d,%d",
			out[0].Rank, out[1].Rank)
	}
	if out[0].HotValue == nil || *out[0].HotValue != 601009 {
		t.Errorf("hot_value 应原样透传 601009")
	}
	// 协议相对 URL 补 https:
	if out[0].URL != "https://t.10jqka.com.cn/x" {
		t.Errorf("urL: got %q(应补 https:)", out[0].URL)
	}
	// 关联个股原样留档
	var ex map[string]any
	if err := json.Unmarshal(out[0].Extra, &ex); err != nil {
		t.Fatalf("extra 应是合法 JSON: %v", err)
	}
	if ex["attach_type"] != "att_stock" {
		t.Errorf("extra.attach_type 应留档,got %v", ex["attach_type"])
	}
}

// 无 jump_url 时 URL 为空(不猜),hot_value 为 0 时**不当作缺失**(0 是有效值)。
func TestNormalizeThsTopicZeroAndEmptyURL(t *testing.T) {
	out := normalizeThsTopic([]thsTopicItem{{Code: "B1", Title: "某话题", HotValue: 0}})
	if len(out) != 1 {
		t.Fatalf("want 1, got %d", len(out))
	}
	if out[0].URL != "" {
		t.Errorf("无 jump_url 时 URL 应为空,got %q", out[0].URL)
	}
	if out[0].HotValue == nil || *out[0].HotValue != 0 {
		t.Errorf("hot_value=0 是有效值,不应为 nil")
	}
}

func TestNormalizeCLSHotArticle(t *testing.T) {
	in := []clsHotArticle{
		{ID: 2489519, Title: "【早报】美股芯片股全线暴涨", ReadNum: 297028, Author: "财联社", CTime: 1790031600},
		{ID: 2489460, Title: "", ReadNum: 100}, // 空标题 → 跳过
	}
	out := normalizeCLSHotArticle(in)
	if len(out) != 1 {
		t.Fatalf("空标题应被跳过: want 1, got %d", len(out))
	}
	if out[0].Source != HotSourceCLS {
		t.Errorf("source: got %q", out[0].Source)
	}
	// URL 规则(实测):https://www.cls.cn/detail/<id>
	if out[0].URL != "https://www.cls.cn/detail/2489519" {
		t.Errorf("url: got %q", out[0].URL)
	}
	if out[0].HotValue == nil || *out[0].HotValue != 297028 {
		t.Errorf("readNum 应原样透传为 hot_value")
	}
}

// 🔴 关键:SSR 结构漂移**必须报错**,不得退化成「成功 0 条」(#64 教训)。
// 喂三段真实会出现的坏输入,断言都拿到 error。
func TestParseCLSHotArticleHTMLFailsLoudly(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"无 __NEXT_DATA__(改版)", `<html><body>no next data here</body></html>`},
		{"__NEXT_DATA__ 不是 JSON", `<html><script id="__NEXT_DATA__" type="application/json">not json</script></html>`},
		{"hotArticleData 为空", `<html><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"hotArticleData":[]}}}</script></html>`},
	}
	for _, c := range cases {
		if _, err := parseCLSHotArticleHTML([]byte(c.body)); err == nil {
			t.Errorf("%s: 应报错(结构漂移不得当空成功),却 err==nil", c.name)
		}
	}
}

// 正常样片段能解析出条数与字段(回归:防改坏 JSON 路径)。
func TestParseCLSHotArticleHTMLOK(t *testing.T) {
	body := `<html><head><script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"hotArticleData":[{"id":1,"title":"甲","readNum":9}],"other":1}}}` +
		`</script></head></html>`
	list, err := parseCLSHotArticleHTML([]byte(body))
	if err != nil {
		t.Fatalf("应解析成功: %v", err)
	}
	if len(list) != 1 || list[0].Title != "甲" || list[0].ReadNum != 9 {
		t.Fatalf("解析结果不符: %+v", list)
	}
}

// 两源**不得**提供跨源合并/加权 —— 设计 §6(粒度不同、同题对=0,混排必误导)。
// 弱断言:本包不得出现跨源合并函数名。能拦住最可能的回归形态(有人加个 Combine 把热榜拍平)。
// ⚠️ 若日后确实要合并,先读 docs/phase11/design/hot-topic.md 红线 —— 那需要先推翻实测结论。
func TestNoCrossSourceMergeHelper(t *testing.T) {
	src, err := os.ReadFile("hottopic.go")
	if err != nil {
		t.Fatalf("读源文件失败: %v", err)
	}
	for _, banned := range []string{"MergeHotTopic", "CombineHotTopic", "CrossSourceRank"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("热榜不得跨源合并/重排(设计 §6 方案 A):发现 %s", banned)
		}
	}
}
