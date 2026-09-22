package ths

// selfstock_detail 的 base64 blob 解码 + 加入价/日解析。
// **全部纯函数**(fixture 驱动单测,零网络)—— 协议一改,单测先红。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// rawDetail 上游 detail 条目。**全用 string 承接**:P 空串 / "0" / 非法值 与「缺失」
// 语义不同,须由 ParseAddedPrice 统一判 NULL(绝不填 0 —— 项目纪律:宁缺毋假)。
type rawDetail struct {
	C string `json:"C"` // code
	M string `json:"M"` // marketid
	P string `json:"P"` // price(加入价,字符串)
	T string `json:"T"` // added date,形如 "20260922"
}

// Detail 归一后的自选元数据。
type Detail struct {
	Code     string // 原始 code(未经 NormalizeCode)
	MarketID string
	Price    *float64   // nil = 上游未给 / ≤0 / 不可解析
	AddedOn  *time.Time // nil = 上游未给 / 不可解析
	Raw      rawDetail  // 原样留档(审计)
}

// decodeDetailBlob base64 → rawDetail 切片。空串 → 空切片(非错误,上游可能真没元数据);
// 坏 base64 / 坏 JSON → error(结构漂移,不空成功)。
func decodeDetailBlob(blob string) ([]rawDetail, error) {
	if strings.TrimSpace(blob) == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		return nil, fmt.Errorf("base64 解码失败: %w", err)
	}
	if strings.TrimSpace(string(decoded)) == "" {
		return nil, nil
	}
	var out []rawDetail
	if err := json.Unmarshal(decoded, &out); err != nil {
		return nil, fmt.Errorf("detail JSON 解析失败: %w", err)
	}
	return out, nil
}

// ParseAddedPrice 解析加入价字符串。
// 空 / 不可解析 / ≤0 → (nil, false) —— **0 不是有效 A 股价格,不填 0 也不臆造**。
func ParseAddedPrice(s string) (*float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return nil, false
	}
	return &v, true
}

// ParseAddedOn 解析加入日。支持 "20260922"(上游格式)与 "2026-09-22"。
// 空 / 非法 → (零值, false)。
func ParseAddedOn(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"20060102", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, beijing); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// beijing 加入日是「同花顺口径的日历日」,不带时刻;用北京时区解析以免跨日漂移。
var beijing = time.FixedZone("CST", 8*3600)
