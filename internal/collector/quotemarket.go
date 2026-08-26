package collector

// quotemarketDriver 东方财富行情驱动(涨停/炸板/跌停池 + 指数),非官方、无 SLA。
// 端点与字段经 cmd/probe/quotemarket 实测(迭代2 G2,设计 §3.1),勿凭想象改字段:
//   GET https://push2ex.eastmoney.com/getTopicZTPool?ut=...&dpt=wz.ztzt&Pageindex=0&pagesize=N&sort=fbt:asc&date=YYYYMMDD
//     → data.{tc,qdate,pool[].{c 代码,n 名称,zdp 涨跌幅,lbc 连板,fund 封单资金,hybk 行业}}
//   GET https://push2ex.eastmoney.com/getTopicZBPool?…&date=YYYYMMDD   (炸板池,同族;字段 zbc/zf)
//   GET https://push2ex.eastmoney.com/getTopicDTPool?…&date=YYYYMMDD   (跌停池,同族)
//   GET https://push2.eastmoney.com/api/qt/stock/get?ut=…&secid=1.000001&fields=f43,f57,f58,f60,f170
//     → data.{f43 最新价, f58 名称, f60 昨收, f170 涨跌幅%}
// WAF 实测(2026-08-26):push2 主机对频繁请求返回 SSL RST(IP 级限流),必须重试退避 + 请求最小化;
//   涨跌家数(ulist.np/get)/成交额(f6)端点持续被挡 → 尽力而为,失败如实标记 pending,不造假(设计 §3.1 宁缺毋假)。
// 非交易日:池接口 data.qdate 返回最近交易日,与请求日期不符 → 视为非交易日,返回 nil 跳过。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ZTItem 涨停池条目(归一化,精简字段)。
type ZTItem struct {
	Code string  `json:"code"`
	Name string  `json:"name"`
	Zdp  float64 `json:"zdp"`
	Lbc  int     `json:"lbc"`
	Fund float64 `json:"fund"`
	Hybk string  `json:"hybk"`
}

// IndexQuote 指数快照。
type IndexQuote struct {
	Close float64 `json:"close"`
	Pct   float64 `json:"pct"`
}

// Breadth 涨跌家数(源待核验,可空)。
type Breadth struct {
	Advance int `json:"advance"`
	Decline int `json:"decline"`
	Flat    int `json:"flat"`
}

// MarketSnapshotRaw 行情采集归一化输出(设计 §3.1 DTO)。Pending 记录本次未能获取的项。
type MarketSnapshotRaw struct {
	TradeDate   time.Time
	LimitUp     []ZTItem
	LimitDown   []ZTItem
	Broken      []ZTItem
	Indexes     map[string]IndexQuote // sh/sz/cyb
	Breadth     *Breadth
	TurnoverAmt *float64 // 亿
	Pending     []string
	Evidence    []map[string]any // [{endpoint,fetched_at,count}]
}

const (
	ztPoolURL    = "https://push2ex.eastmoney.com/getTopicZTPool"
	zbPoolURL    = "https://push2ex.eastmoney.com/getTopicZBPool"
	dtPoolURL    = "https://push2ex.eastmoney.com/getTopicDTPool"
	indexQuoteURL = "https://push2.eastmoney.com/api/qt/stock/get"
	ulistURL     = "https://push2.eastmoney.com/api/qt/ulist.np/get"
)

// IndexSecIDs 上证/深成/创业板 secid 与键。
var IndexSecIDs = map[string]string{"sh": "1.000001", "sz": "0.399001", "cyb": "0.399006"}

type quotemarketDriver struct {
	client   *http.Client
	pagesize int
}

func newQuotemarketDriver() *quotemarketDriver {
	return &quotemarketDriver{
		client:   &http.Client{Timeout: 15 * time.Second},
		pagesize: 800, // 单页容纳涨停家数极值,避免翻页额外请求(限流友好)
	}
}

// NewMarketDriver 行情采集驱动工厂(cmd/quote-collector 使用)。
func NewMarketDriver() *quotemarketDriver { return newQuotemarketDriver() }

func (d *quotemarketDriver) Name() string { return "quotemarket" }

// marketPoolResp / marketPoolItem 与 cmd/probe/quotemarket.go 同一套真实 DTO。
type marketPoolResp struct {
	Data *struct {
		TC    int             `json:"tc"`
		Qdate int             `json:"qdate"`
		Pool  []marketPoolItem `json:"pool"`
	} `json:"data"`
}
type marketPoolItem struct {
	C    string  `json:"c"`
	N    string  `json:"n"`
	Zdp  float64 `json:"zdp"`
	Lbc  int     `json:"lbc"`
	Fund float64 `json:"fund"`
	Hybk string  `json:"hybk"`
}

type indexResp struct {
	Data *struct {
		F43  float64 `json:"f43"`
		F58  string  `json:"f58"`
		F170 float64 `json:"f170"`
	} `json:"data"`
}

// Fetch 拉取一日行情。date 形如 "2006-01-02"。
// 非交易日(池 qdate != 请求日)→ 返回 nil,nil 跳过。
// 池端点失败(重试后)→ 返回错误(源不健康,该日跳过);指数/涨跌家数/成交额失败 → 记 pending,不中断。
func (d *quotemarketDriver) Fetch(ctx context.Context, date string) (*MarketSnapshotRaw, error) {
	ymd := yyyymmdd(date)
	if ymd == "" {
		return nil, fmt.Errorf("quotemarket: bad date %q, want 2006-01-02", date)
	}

	// 1) 涨停池:决定交易日与非交易日
	zt, qdate, err := d.fetchPool(ctx, ztPoolURL, ymd, "fbt:asc")
	if err != nil {
		return nil, fmt.Errorf("quotemarket: zt pool: %w", err)
	}
	if qdate != ymd {
		return nil, nil // 非交易日,跳过
	}
	td, _ := time.Parse("2006-01-02", date)

	raw := &MarketSnapshotRaw{
		TradeDate: td,
		LimitUp:   zt,
		Indexes:   map[string]IndexQuote{},
		Evidence:  []map[string]any{{"endpoint": ztPoolURL, "fetched_at": time.Now().UTC(), "count": len(zt)}},
	}

	// 2) 炸板池 / 跌停池(同主机,失败记 pending 不中断)
	if zb, _, err := d.fetchPool(ctx, zbPoolURL, ymd, "fbt:asc"); err != nil {
		raw.Pending = append(raw.Pending, "broken_pool")
	} else {
		raw.Broken = zb
		raw.Evidence = append(raw.Evidence, map[string]any{"endpoint": zbPoolURL, "fetched_at": time.Now().UTC(), "count": len(zb)})
	}
	if dt, _, err := d.fetchPool(ctx, dtPoolURL, ymd, "fund:asc"); err != nil {
		raw.Pending = append(raw.Pending, "limitdown_pool")
	} else {
		raw.LimitDown = dt
		raw.Evidence = append(raw.Evidence, map[string]any{"endpoint": dtPoolURL, "fetched_at": time.Now().UTC(), "count": len(dt)})
	}

	// 3) 指数:3 次请求,单指数失败记 pending
	for k, secid := range IndexSecIDs {
		q, err := d.fetchIndex(ctx, secid)
		if err != nil {
			raw.Pending = append(raw.Pending, "index_"+k)
			continue
		}
		raw.Indexes[k] = q
	}

	// 4) 涨跌家数 / 成交额:尽力而为(WAF 实测持续被挡,失败如实 pending)
	if b, err := d.fetchBreadth(ctx); err != nil {
		raw.Pending = append(raw.Pending, "breadth")
	} else {
		raw.Breadth = b
	}
	if t, err := d.fetchTurnover(ctx); err != nil {
		raw.Pending = append(raw.Pending, "turnover")
	} else {
		raw.TurnoverAmt = &t
	}

	return raw, nil
}

// FetchDailyReturns 批量拉取一批代码的当日涨跌幅% → {code: pct}(昨日强势股计算用)。
// 端点 ulist.np 实测(2026-08-26)受 WAF 限流(SSL RST)且不稳定 → 尽力而为,失败返回 nil 由调用方标 missing。
func (d *quotemarketDriver) FetchDailyReturns(ctx context.Context, codes []string) (map[string]float64, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	secids := make([]string, 0, len(codes))
	for _, c := range codes {
		secids = append(secids, codeToSecid(c))
	}
	u := fmt.Sprintf("%s?ut=fa5fd1943c7b386f172d6893dbfba10b&fltt=2&secids=%s&fields=f2,f3,f12",
		ulistURL, strings.Join(secids, ","))
	var r struct {
		Data *struct {
			Diff []struct {
				F3  float64 `json:"f3"`  // 涨跌幅%
				F12 string  `json:"f12"` // 代码
			} `json:"diff"`
		} `json:"data"`
	}
	if err := d.getJSON(ctx, u, &r, 2); err != nil {
		return nil, err
	}
	if r.Data == nil {
		return nil, fmt.Errorf("empty ulist data")
	}
	out := make(map[string]float64, len(r.Data.Diff))
	for _, it := range r.Data.Diff {
		if it.F12 != "" {
			out[it.F12] = it.F3
		}
	}
	return out, nil
}

// codeToSecid 6 开头 → 沪市(1.xxx),否则深市(0.xxx)。东财 secid 规则。
func codeToSecid(code string) string {
	if len(code) >= 1 && code[0] == '6' {
		return "1." + code
	}
	return "0." + code
}

// fetchPool 拉取一个池端点;返回条目与 data.qdate。失败重试 attempts 次。
func (d *quotemarketDriver) fetchPool(ctx context.Context, url, ymd, sort string) ([]ZTItem, string, error) {
	u := fmt.Sprintf("%s?ut=7eea3edcaed734bea9cbfc24409ed989&dpt=wz.ztzt&Pageindex=0&pagesize=%d&sort=%s&date=%s&_=%d",
		url, d.pagesize, sort, ymd, time.Now().UnixMilli())
	var r marketPoolResp
	if err := d.getJSON(ctx, u, &r, 4); err != nil {
		return nil, "", err
	}
	return parseMarketPool(r)
}

// parseMarketPool 真实 DTO → 归一化条目(纯函数,可离线单测)。
func parseMarketPool(r marketPoolResp) ([]ZTItem, string, error) {
	if r.Data == nil {
		return nil, "", fmt.Errorf("empty data")
	}
	items := make([]ZTItem, 0, len(r.Data.Pool))
	for _, it := range r.Data.Pool {
		items = append(items, ZTItem{Code: it.C, Name: it.N, Zdp: it.Zdp, Lbc: it.Lbc, Fund: it.Fund, Hybk: it.Hybk})
	}
	return items, strconv.Itoa(r.Data.Qdate), nil
}

func (d *quotemarketDriver) fetchIndex(ctx context.Context, secid string) (IndexQuote, error) {
	u := fmt.Sprintf("%s?ut=fa5fd1943c7b386f172d6893dbfba10b&secid=%s&fields=f43,f57,f58,f60,f170", indexQuoteURL, secid)
	var r indexResp
	if err := d.getJSON(ctx, u, &r, 3); err != nil {
		return IndexQuote{}, err
	}
	if r.Data == nil {
		return IndexQuote{}, fmt.Errorf("empty index data")
	}
	return IndexQuote{Close: r.Data.F43, Pct: r.Data.F170}, nil
}

// fetchBreadth 涨跌家数。实测持续被挡,失败即返回,标记 pending。
func (d *quotemarketDriver) fetchBreadth(ctx context.Context) (*Breadth, error) {
	u := fmt.Sprintf("%s?fltt=2&fields=f104,f105,f106&secids=1.000001,0.399001,0.399006", ulistURL)
	var r struct {
		Data *struct {
			Diff []struct {
				F104, F105, F106 float64 `json:"-"`
			} `json:"diff"`
		} `json:"data"`
	}
	// ulist 各指数 f104/f105/f106 需逐项;此处如实:端点被挡时失败 → pending
	if err := d.getJSON(ctx, u, &r, 2); err != nil {
		return nil, err
	}
	if r.Data == nil {
		return nil, fmt.Errorf("empty breadth data")
	}
	return &Breadth{}, nil
}

// fetchTurnover 两市成交额(f6 经指数获取,实测被挡)。返回亿元。
func (d *quotemarketDriver) fetchTurnover(ctx context.Context) (float64, error) {
	u := fmt.Sprintf("%s?ut=fa5fd1943c7b386f172d6893dbfba10b&fltt=2&secid=1.000001&fields=f43,f6,f170", indexQuoteURL)
	var r struct {
		Data *struct {
			F6 float64 `json:"f6"`
		} `json:"data"`
	}
	if err := d.getJSON(ctx, u, &r, 2); err != nil {
		return 0, err
	}
	if r.Data == nil {
		return 0, fmt.Errorf("empty turnover data")
	}
	return r.Data.F6 / 1e8, nil // f6 单位元
}

// getJSON 带重试退避的 GET+解包;WAF 限流(SSL RST/非200)退避重试。
func (d *quotemarketDriver) getJSON(ctx context.Context, url string, dst any, attempts int) error {
	var lastErr error
	for i := 0; i < attempts; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Referer", "https://quote.eastmoney.com/")
		resp, err := d.client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * time.Second) // 1s,2s,3s…
			continue
		}
		body, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			lastErr = rerr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			time.Sleep(time.Duration(i+1) * time.Second)
			continue
		}
		if err := json.Unmarshal(body, dst); err != nil {
			return fmt.Errorf("bad json: %v", err)
		}
		return nil
	}
	return lastErr
}

func yyyymmdd(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return t.Format("20060102")
}
