package dataprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// mcpDatasets 能力 → datahub 数据集 id。**只有**此处列出的能力由 McpProvider 支持；
// 其余能力（事件/热榜等已有链路）归 FreeSourceProvider/LocalPGProvider。
//
// datahub 的 MCP 工具名 = 数据集 id 点转下划线（如 `hotspot.indices` → `hotspot_indices`）。
var mcpDatasets = map[Capability]string{
	CapQuoteSeries:        "daily_price",
	CapFinancials:         "financial_indicator",
	CapValuationHistory:   "daily_valuation",
	CapCapitalFlow:        "capital_flow",
	CapIndustryMembers:    "industry_sw_cons",
	CapIndustryFinancials: "industry_sw_cons",
	CapMacro:              "macro",
	CapAnnouncements:      "announcement_by_stock",
}

// dsShape 描述一个数据集的返回形态（决定解析成 Series 还是 Table + 截断列）。
type dsShape struct {
	series    bool     // true = 时序（Series），false = 行列（Table）
	dateKey   string   // 行内承载时点的 JSON 键
	valueKeys []string // 时序的数值列（转 float 落 Point.Values）
	asOfParam string   // 请求参数里防未来函数的截断参数名（daily_price=`asof`，财务=`as_of`；空=不支持）
}

var mcpShapes = map[string]dsShape{
	"daily_price": {series: true, dateKey: "trade_date", asOfParam: "asof",
		valueKeys: []string{"open", "high", "low", "close", "volume", "amount", "turnover_rate"}},
	"daily_valuation": {series: true, dateKey: "trade_date", asOfParam: "asof",
		valueKeys: []string{"close", "total_mv", "float_mv", "pe_ttm", "pb"}},
	"industry_index": {series: true, dateKey: "trade_date", asOfParam: "asof",
		valueKeys: []string{"open", "high", "low", "close", "pct_change", "amplitude", "chg_amount"}},
	"macro": {series: true, dateKey: "period_end",
		valueKeys: []string{"level", "yoy", "mom"}},
	"financial_indicator":   {dateKey: "report_date", asOfParam: "as_of"},
	"capital_flow":          {dateKey: "trade_date"},
	"industry_sw_cons":      {dateKey: ""},
	"announcement_by_stock": {dateKey: "publish_date"},
}

// McpProvider 是 datahub 的 **MCP client**（phase13 §3.3 唯一接入面）。
//
// 确定性主链：PIKS 后端作为 MCP client **直接调用**（不经 LLM），从而可复现、可测、
// `as_of` 可控。防未来函数**在适配层做**（对返回结果裁剪），不依赖 datahub 侧。
type McpProvider struct {
	cli   *client.Client
	label string
	caps  map[Capability]bool
}

// NewMcpProvider 连接 datahub 的 MCP server（Streamable HTTP，`POST /mcp`），完成
// initialize 握手。`mcpURL` 形如 `http://datahub:8100/mcp`；`token` 非空则以
// `Authorization: Bearer` 下发（datahub 的 /mcp 当前未上鉴权，此处预置以备收紧）。
//
// ⚠️ 返回的 Provider 需由调用方在退出时 `Close`。
func NewMcpProvider(ctx context.Context, mcpURL, token string) (*McpProvider, error) {
	opts := []transport.StreamableHTTPCOption{}
	if token != "" {
		opts = append(opts, transport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + token,
		}))
	}
	cli, err := client.NewStreamableHttpClient(mcpURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("dataprovider: 创建 MCP client 失败: %w", err)
	}
	if err := cli.Start(ctx); err != nil {
		return nil, fmt.Errorf("dataprovider: 启动 MCP client 失败: %w", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "piks", Version: "v0.0.0"}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("dataprovider: MCP initialize 失败: %w", err)
	}
	caps := make(map[Capability]bool, len(mcpDatasets))
	for c := range mcpDatasets {
		caps[c] = true
	}
	return &McpProvider{cli: cli, label: "datahub:mcp", caps: caps}, nil
}

// Name 实现 Provider。
func (p *McpProvider) Name() string { return p.label }

// Supports 实现 Provider。仅认映射表内的能力。
func (p *McpProvider) Supports(cap Capability) bool { return p.caps[cap] }

// Close 释放 MCP 连接。
func (p *McpProvider) Close() error { return p.cli.Close() }

// Fetch 实现 Provider：按能力选出 datahub 数据集 → 调其 MCP tool → 解析 + 适配层截断。
func (p *McpProvider) Fetch(ctx context.Context, req FetchRequest) (Result, error) {
	dsID, ok := mcpDatasets[req.Capability]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, req.Capability)
	}
	shape, ok := mcpShapes[dsID]
	if !ok {
		return nil, fmt.Errorf("dataprovider: 数据集 %s 无解析形态定义", dsID)
	}

	args := buildArgs(dsID, shape, req)
	call := mcp.CallToolRequest{}
	call.Params.Name = strings.ReplaceAll(dsID, ".", "_")
	call.Params.Arguments = args

	res, err := p.cli.CallTool(ctx, call)
	if err != nil {
		return nil, fmt.Errorf("dataprovider: 调用 datahub tool %s 失败: %w", call.Params.Name, err)
	}
	if res.IsError {
		return nil, fmt.Errorf("dataprovider: datahub tool %s 返回错误: %s", call.Params.Name, firstText(res))
	}

	rows, upMeta, err := decodeEnvelope(res)
	if err != nil {
		return nil, fmt.Errorf("dataprovider: 解析 datahub tool %s 结果失败: %w", call.Params.Name, err)
	}
	asOf := req.asOf()
	meta := Meta{
		Source: p.label,
		Stale:  boolOf(upMeta["stale"]),
		Cached: boolOf(upMeta["cached"]),
		Rows:   len(rows),
		AsOf:   asOf,
	}
	if shape.series {
		return Series{Subject: req.Subject, Points: toPoints(rows, shape, asOf), Meta: meta}, nil
	}
	return Table{Columns: columnsOf(rows), Rows: trimRows(rows, shape, asOf), Meta: meta}, nil
}

// buildArgs 把 FetchRequest 翻译成 datahub 数据集参数（参数名对齐 DatasetSpec.params）。
func buildArgs(dsID string, shape dsShape, req FetchRequest) map[string]any {
	args := map[string]any{}
	if req.Subject.ID != "" {
		args["codes"] = req.Subject.ID
	}
	if !req.Start.IsZero() {
		args["start"] = req.Start.Format("2006-01-02")
	}
	if !req.End.IsZero() {
		args["end"] = req.End.Format("2006-01-02")
	}
	if req.Limit > 0 {
		args["last"] = req.Limit
	}
	if shape.asOfParam != "" {
		if a := req.asOf(); !a.IsZero() {
			args[shape.asOfParam] = a.Format("2006-01-02")
		}
	}
	// 宏观按维度 key 取（indicator），非 codes。
	if dsID == "macro" {
		delete(args, "codes")
		if req.Subject.ID != "" {
			args["indicator"] = req.Subject.ID
		}
	}
	for k, v := range req.Params {
		args[k] = v
	}
	return args
}

// decodeEnvelope 取出 datahub tool 返回的 `{dataset, rows, meta}` 信封。
// 优先 structuredContent；退回 content[0].text 的 JSON。
func decodeEnvelope(res *mcp.CallToolResult) ([]map[string]any, map[string]any, error) {
	var raw []byte
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return nil, nil, err
		}
		raw = b
	} else if txt := firstText(res); txt != "" {
		raw = []byte(txt)
	} else {
		return nil, nil, fmt.Errorf("响应既无 structuredContent 也无文本")
	}
	var env struct {
		Rows []map[string]any `json:"rows"`
		Meta map[string]any   `json:"meta"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, nil, err
	}
	return env.Rows, env.Meta, nil
}

// toPoints 行 → 时序点，并按 asOf 截断（防未来函数，落在适配层）。
func toPoints(rows []map[string]any, shape dsShape, asOf time.Time) []Point {
	pts := make([]Point, 0, len(rows))
	for _, r := range rows {
		t, ok := parseTime(strOf(r[shape.dateKey]))
		if !ok {
			continue // 无有效时点的行不进序列（不静默补当前日）
		}
		if !asOf.IsZero() && t.After(asOf) {
			continue // 适配层截断：不得取到基准日之后
		}
		vals := make(map[string]float64, len(shape.valueKeys))
		for _, k := range shape.valueKeys {
			if f, ok := floatOf(r[k]); ok {
				vals[k] = f
			}
		}
		pts = append(pts, Point{Period: t, Values: vals})
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].Period.Before(pts[j].Period) })
	return pts
}

// trimRows Table 路径同样按 asOf 截断（若该表有日期列）。
func trimRows(rows []map[string]any, shape dsShape, asOf time.Time) []map[string]any {
	if shape.dateKey == "" || asOf.IsZero() {
		return rows
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if t, ok := parseTime(strOf(r[shape.dateKey])); ok && t.After(asOf) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// columnsOf 稳定列序（按首行键排序；列集合相同即稳定）。
func columnsOf(rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func floatOf(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		var f float64
		if err := json.Unmarshal([]byte(n), &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

func boolOf(v any) bool {
	b, _ := v.(bool)
	return b
}

// parseTime 兼容 date（2006-01-02）与 datetime（含 T 或空格）两种上游格式。
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
