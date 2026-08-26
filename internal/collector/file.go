package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// fileDriver 读取本地 JSON(保底驱动,迭代0 验收用;不依赖任何外部 API)。
// 输入格式:{"items":[{"external_id","url","title","content","published_at"}]}
type fileDriver struct {
	path string
}

func (d *fileDriver) Name() string { return "file" }

func (d *fileDriver) Fetch(ctx context.Context) ([]RawNews, error) {
	if d.path == "" {
		return nil, fmt.Errorf("file driver requires -input path")
	}
	data, err := os.ReadFile(d.path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", d.path, err)
	}
	var payload struct {
		Items []struct {
			ExternalID  string `json:"external_id"`
			URL         string `json:"url"`
			Title       string `json:"title"`
			Content     string `json:"content"`
			PublishedAt string `json:"published_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse %s: %w", d.path, err)
	}
	out := make([]RawNews, 0, len(payload.Items))
	for _, it := range payload.Items {
		if NormalizeContent(it.Content) == "" {
			continue // 空内容跳过
		}
		n := RawNews{
			ExternalID: it.ExternalID,
			URL:        it.URL,
			Title:      it.Title,
			Content:    NormalizeContent(it.Content),
		}
		if it.PublishedAt != "" {
			if t, err := time.Parse(time.RFC3339, it.PublishedAt); err == nil {
				n.PublishedAt = &t
			}
		}
		out = append(out, n)
	}
	return out, nil
}
