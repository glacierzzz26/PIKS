package ths

// marketid(同花顺数字市场码)↔ 交易所缩写。映射取自参照实现 constant.py 的 __MARKET_CODE
// (实测值),**不臆造**。A 股白名单是本包唯一「哪些能进 PIKS entities」的判据。

import "strings"

// marketAbbr 数字市场码 → 缩写。未知码原样返回(留档,不猜)。
var marketAbbr = map[string]string{
	"17":  "SH",    // 上海证券交易所
	"20":  "SHETF", // 上海 ETF
	"22":  "ST",    // 上海 ST
	"33":  "SZ",    // 深圳证券交易所
	"36":  "SZETF", // 深圳 ETF
	"48":  "ZS",    // 指数
	"38":  "CYB",   // 创业板
	"18":  "KC",    // 科创板
	"71":  "BJ",    // 北京证券交易所
	"151": "BJ",    // 北交所备用码(参照实现手动补充)
	"55":  "HK",    // 港股
	"61":  "US",    // 美股
	"50":  "FT",    // 期货
	"51":  "QH",    // 期货主力
	"53":  "QZ",    // 期指
	"79":  "OP",    // 期权
	"39":  "JJ",    // 基金
	"45":  "ZQ",    // 债券
	"67":  "XSB",   // 新三板
}

// aShareMarkets A 股权益类市场码白名单(SH/SZ/创业板/科创板/BJ 及备用)。
// 只有这些码的条目才会进 PIKS(见 watchsync.Filter)。
var aShareMarkets = map[string]bool{
	"17": true, "33": true, "18": true, "38": true, "71": true, "151": true,
}

// MarketAbbr 数字市场码 → 缩写;未知原样返回。
func MarketAbbr(marketID string) string {
	id := strings.TrimSpace(marketID)
	if a, ok := marketAbbr[id]; ok {
		return a
	}
	return id
}

// IsAShare 是否 A 股权益类市场(白名单外一律 false —— 指数/港美股/期货全挡掉)。
func IsAShare(marketID string) bool {
	return aShareMarkets[strings.TrimSpace(marketID)]
}
