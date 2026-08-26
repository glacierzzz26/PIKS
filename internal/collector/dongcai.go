package collector

import (
	"context"
	"fmt"
)

// dongcaiDriver 东方财富快讯(网页内部接口,非官方、无 SLA)。
// 真实 DTO 尚未验证(G1 缺口):先返回明确错误,验证后在此实现。
type dongcaiDriver struct{}

func (d *dongcaiDriver) Name() string { return "dongcai" }

func (d *dongcaiDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	return nil, fmt.Errorf("dongcai driver not implemented: 真实接口待验证(G1 缺口),请先用 file 驱动")
}
