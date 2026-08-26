// Package collector 数据采集:各种源适配器统一产出 RawNews(归一化),由调用方去重入库。
package collector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// RawNews 采集适配器的唯一输出(与具体源无关的归一化 DTO)。
type RawNews struct {
	ExternalID  string
	URL         string
	Title       string
	Content     string
	PublishedAt *time.Time
}

type Driver interface {
	Name() string
	Fetch(ctx context.Context) ([]RawNews, error)
}

func NewDriver(name, input string) (Driver, error) {
	switch name {
	case "file":
		return &fileDriver{path: input}, nil
	case "dongcai":
		return newDongcaiDriver(), nil
	default:
		return nil, fmt.Errorf("unknown driver: %s", name)
	}
}

// NormalizeContent 归一化:去首尾空白、折叠连续空白(去重前使用)。
func NormalizeContent(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

// ContentHash sha256(归一化内容),去重键。
func ContentHash(s string) string {
	sum := sha256.Sum256([]byte(NormalizeContent(s)))
	return hex.EncodeToString(sum[:])
}

// StrPtr 非空 string → *string(便于插入可空列)。
func StrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
