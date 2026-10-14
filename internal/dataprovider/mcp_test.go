package dataprovider

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// dateUTC 固定时区构造基准日。
func dateUTC(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// newSmokeServer 起一个**进程内 MCP server**（真实 Streamable HTTP 协议），暴露一个
// `daily_price` tool，返回与 datahub 同形的信封 `{dataset, rows, meta}`。
//
// 内置两行：一行 ≤ as_of、一行 > as_of —— 用以断言**适配层截断**（防未来函数）。
func newSmokeServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := server.NewMCPServer("datahub-smoke", "1.0.0", server.WithToolCapabilities(false))
	tool := mcp.NewTool("daily_price", mcp.WithString("codes"))
	srv.AddTool(tool, func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		env := map[string]any{
			"dataset": "daily_price",
			"rows": []map[string]any{
				{"code": "600519", "trade_date": "2026-10-10", "open": 100.0,
					"high": 105.0, "low": 99.0, "close": 104.0,
					"volume": 12345.0, "amount": 1.0e8, "turnover_rate": 1.5},
				// 基准日之后 —— 必须被适配层裁掉
				{"code": "600519", "trade_date": "2026-10-20", "open": 110.0,
					"high": 115.0, "low": 109.0, "close": 114.0,
					"volume": 23456.0, "amount": 2.0e8, "turnover_rate": 2.0},
			},
			"meta": map[string]any{"stale": false, "cached": false, "rows": 2},
		}
		return mcp.NewToolResultStructured(env, ""), nil
	})
	httpSrv := server.NewStreamableHTTPServer(srv)
	ts := httptest.NewServer(httpSrv)
	t.Cleanup(ts.Close)
	return ts
}

func TestMcpProviderFetchQuoteSeries(t *testing.T) {
	ts := newSmokeServer(t)
	ctx := context.Background()

	p, err := NewMcpProvider(ctx, ts.URL+"/mcp", "")
	if err != nil {
		t.Fatalf("NewMcpProvider: %v", err)
	}
	defer p.Close()

	if p.Name() != "datahub:mcp" || !p.Supports(CapQuoteSeries) {
		t.Fatalf("Name/Supports 不符：name=%s supports=%v", p.Name(), p.Supports(CapQuoteSeries))
	}

	subj := Subject{Kind: SubjectStock, ID: "600519", Name: "贵州茅台", AsOf: dateUTC(2026, 10, 13)}
	res, err := p.Fetch(ctx, FetchRequest{
		Capability: CapQuoteSeries,
		Subject:    subj,
		Start:      dateUTC(2026, 10, 1),
		End:        dateUTC(2026, 10, 13),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	s, ok := res.(Series)
	if !ok {
		t.Fatalf("期望 Series，得到 %T", res)
	}
	if len(s.Points) != 1 {
		t.Fatalf("截断后期望 1 个点（10-20 应被裁掉），得到 %d", len(s.Points))
	}
	got := s.Points[0]
	if !got.Period.Equal(dateUTC(2026, 10, 10)) || got.Values["close"] != 104.0 ||
		got.Values["volume"] != 12345.0 {
		t.Fatalf("点不符：%+v", got)
	}
	if s.Meta.Source != "datahub:mcp" || s.Meta.Rows != 2 || s.Meta.Stale {
		t.Fatalf("Meta 不符：%+v", s.Meta)
	}
	if !s.Meta.AsOf.Equal(dateUTC(2026, 10, 13)) {
		t.Fatalf("Meta.AsOf 不符：%v", s.Meta.AsOf)
	}
}

func TestMcpProviderUnsupportedCapability(t *testing.T) {
	ts := newSmokeServer(t)
	ctx := context.Background()
	p, err := NewMcpProvider(ctx, ts.URL+"/mcp", "")
	if err != nil {
		t.Fatalf("NewMcpProvider: %v", err)
	}
	defer p.Close()

	if p.Supports(CapEvents) {
		t.Fatalf("McpProvider 不应支持 CapEvents（归已有链路）")
	}
	_, err = p.Fetch(ctx, FetchRequest{Capability: CapEvents, Subject: Subject{ID: "x"}})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("期望 ErrUnsupported，得到 %v", err)
	}
}

func TestPlaceholderProvidersNotWired(t *testing.T) {
	ctx := context.Background()
	for _, p := range []Provider{NewFreeSourceProvider(), NewLocalPGProvider(nil)} {
		if !p.Supports(CapEvents) {
			t.Errorf("%s 应声明支持 CapEvents", p.Name())
		}
		if _, err := p.Fetch(ctx, FetchRequest{Capability: CapEvents}); !errors.Is(err, ErrNotWired) {
			t.Errorf("%s 占位 Fetch 应返回 ErrNotWired，得到 %v", p.Name(), err)
		}
	}
}
