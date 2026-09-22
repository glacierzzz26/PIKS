package ths

// 「我的自选」只读拉取:名单(getSelfStockWithMarket)+ 元数据(selfstock_detail)。
// 🔴 只读 —— 绝不调 modifySelfStock。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// SelfStock 名单条目(只有 code + marketid —— 上游不给名字)。
type SelfStock struct {
	Code     string
	MarketID string
}

// selfStockResp getSelfStockWithMarket 的 JSON DTO(实测 2026-09-22)。
type selfStockResp struct {
	ErrorCode *int   `json:"errorCode"` // 指针:区分「0(成功)」与「字段缺失」
	ErrorMsg  string `json:"errorMsg"`
	Result    []struct {
		Code     string `json:"code"`
		MarketID string `json:"marketid"`
	} `json:"result"`
}

// SelfStocks 拉「我的自选」名单。
// 🔴 errorCode != 0 → error(鉴权失败/协议变更,**不空成功** —— #64 教训)。
// 🔴 result 为空 → 也返回空切片 + nil;「空列表是否算异常」由调用方(watchsync)判定。
func (c *Client) SelfStocks(ctx context.Context) ([]SelfStock, error) {
	body, err := c.getJSON(ctx, selfStockListURL, nil)
	if err != nil {
		return nil, err
	}
	var r selfStockResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("ths 自选名单: 非 JSON: %w", err)
	}
	if r.ErrorCode == nil {
		return nil, fmt.Errorf("ths 自选名单: 响应缺 errorCode(结构漂移): %s", truncate(body, 200))
	}
	if *r.ErrorCode != 0 {
		return nil, fmt.Errorf("%w: errorCode=%d errorMsg=%s", ErrAuth, *r.ErrorCode, r.ErrorMsg)
	}
	out := make([]SelfStock, 0, len(r.Result))
	for _, e := range r.Result {
		out = append(out, SelfStock{Code: e.Code, MarketID: e.MarketID})
	}
	return out, nil
}

// SelfStockDetails 拉加入价/加入日。与名单是**独立请求** —— 它失败时调用方应降级
// (名单照常同步,价/日不更新),见 watchsync。
func (c *Client) SelfStockDetails(ctx context.Context) ([]Detail, error) {
	uid := c.UserID()
	if uid == "" {
		return nil, fmt.Errorf("ths 自选元数据: 无 userid(未登录?)")
	}
	u := selfStockDetailURL + "?reqtype=download&app_flag=0E&userid=" + url.QueryEscape(uid)
	body, err := c.getText(ctx, u, map[string]string{"userid": uid})
	if err != nil {
		return nil, err
	}
	var root struct {
		Item *struct {
			Version string `xml:"version,attr"`
			Blob    string `xml:"selfstock_detail,attr"`
		} `xml:"item"`
	}
	if err := newXMLDecoder(bytes.NewReader(body)).Decode(&root); err != nil {
		return nil, fmt.Errorf("ths 自选元数据: XML 解析失败: %w", err)
	}
	if _, _, err := parseRet(body); err != nil {
		return nil, fmt.Errorf("ths 自选元数据: %w", err)
	}
	if root.Item == nil {
		return nil, fmt.Errorf("ths 自选元数据: %w", errItemMissing)
	}
	raws, err := decodeDetailBlob(root.Item.Blob)
	if err != nil {
		return nil, fmt.Errorf("ths 自选元数据: %w", err)
	}
	out := make([]Detail, 0, len(raws))
	for _, r := range raws {
		d := Detail{Code: r.C, MarketID: r.M, Raw: r}
		if p, ok := ParseAddedPrice(r.P); ok {
			d.Price = p
		}
		if t, ok := ParseAddedOn(r.T); ok {
			d.AddedOn = &t
		}
		out = append(out, d)
	}
	return out, nil
}
