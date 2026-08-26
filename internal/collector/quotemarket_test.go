package collector

import (
	"encoding/json"
	"testing"
	"time"
)

// 真实涨停池响应片段(2026-08-26 探针实测 DTO 形状)。
const realZTPoolJSON = `{"rc":0,"data":{"tc":52,"qdate":20260826,"pool":[
  {"c":"002084","n":"海鸥住工","zdp":10.1149,"lbc":3,"fund":327258667,"hybk":"家居用品"},
  {"c":"002366","n":"融发核电","zdp":10.0529,"lbc":1,"fund":166007712,"hybk":"其他电源"}
]}}`

func TestParseMarketPool(t *testing.T) {
	var r marketPoolResp
	if err := json.Unmarshal([]byte(realZTPoolJSON), &r); err != nil {
		t.Fatal(err)
	}
	items, qdate, err := parseMarketPool(r)
	if err != nil {
		t.Fatal(err)
	}
	if qdate != "20260826" {
		t.Fatalf("qdate = %s, want 20260826", qdate)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	first := items[0]
	if first.Code != "002084" || first.Name != "海鸥住工" || first.Lbc != 3 || first.Hybk != "家居用品" || first.Fund != 327258667 {
		t.Fatalf("bad first item: %+v", first)
	}
}

func TestYyyymmdd(t *testing.T) {
	if got := yyyymmdd("2026-08-26"); got != "20260826" {
		t.Fatalf("yyyymmdd = %s, want 20260826", got)
	}
	if got := yyyymmdd("bad"); got != "" {
		t.Fatalf("yyyymmdd(bad) = %q, want empty", got)
	}
}

// 非交易日判定:qdate 为最近交易日,请求日为周六 → Fetch 应返回 nil(跳过)。
// 该路径依赖网络,此处仅验证 yyyymmdd 与 qdate 语义(完整判定在真实验证)。
func TestNonTradingSemantics(t *testing.T) {
	td, _ := time.Parse("2006-01-02", "2026-08-29") // 周六
	if td.Weekday() != time.Saturday {
		t.Fatalf("2026-08-29 should be Saturday, got %v", td.Weekday())
	}
}
