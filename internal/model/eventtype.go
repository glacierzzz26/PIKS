package model

import "strings"

// EventType 一个事件类型的枚举项:机器 key + 中文展示 label。
//
// ⚠️ **本切片是全仓事件类型枚举的唯一真源**(issue #61)。此前枚举散落三处且互相漂移:
// 后端 `internal/extract` 的 `allowedEventTypes` / JSON Schema `enum` / `0001_init.sql` 注释
// 是一套 9 值;前端 `lib/constants.ts` / `lib/format.ts` / `EventTable.tsx` 是另一套**8 值**
// (其中 6 个 key 全仓零出现,是前端臆造的)。后果:92% 事件在表格里露英文原值、
// 类型下拉 8 项里 6 项永远筛不出东西。
//
// 现在只有这一处定义;`extract` 的校验集与 JSON Schema 由它派生,前端经
// `GET /api/v1/event-types` 消费它 —— 后端加类型时前端**零改动**即跟上。
//
// ⚠️ 新增类型时**必须同时**给出中文 label:守卫 `scripts/check-event-type-parity.sh`
// 断言 label 非空,漏配即 CI 失败(否则前端会静默回落英文,漂移只修一半)。
//
// ⚠️ 类型是**语义分类**,不是重要度 —— 与 confidence(抽取置信度)无关。
var EventTypes = []EventType{
	{Key: "policy", Label: "政策"},
	{Key: "earnings", Label: "财报业绩"},
	{Key: "industry", Label: "行业动态"},
	{Key: "accident", Label: "突发事故"},
	{Key: "international", Label: "国际时事"},
	{Key: "tech", Label: "科技进展"},
	{Key: "macro", Label: "宏观经济"},
	{Key: "company", Label: "公司动向"},
	{Key: "other", Label: "其他"},
}

// EventType 见 EventTypes。
type EventType struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// EventTypeFallback 是 `EventTypes` 里的兜底桶 key:LLM 给出未受支持的类型时归到这里。
//
// ⚠️ 由真源**按 key 查出来**而非字面量再抄一遍 —— 守卫
// `scripts/check-event-type-parity.sh` 断言 `internal/extract` 里不出现任何
// key 字面量,连兜底桶也不例外(否则「真源」又多了一个可漂移的副本)。
var EventTypeFallback = mustEventType("other")

// mustEventType 按 key 取枚举项;取不到即 panic —— 这是**构建期不变量**
// (包初始化时即失败),不是运行时可恢复错误:真源里删掉 "other" 而不改这里,
// 就是枚举被改坏了,应当立刻炸掉而不是静默回落。
func mustEventType(key string) string {
	for _, t := range EventTypes {
		if t.Key == key {
			return t.Key
		}
	}
	panic("model.EventTypeFallback: 真源 EventTypes 里没有 key " + key)
}

// EventTypeKeys 返回全部 key,顺序与 EventTypes 一致。
func EventTypeKeys() []string {
	out := make([]string, 0, len(EventTypes))
	for _, t := range EventTypes {
		out = append(out, t.Key)
	}
	return out
}

// EventTypeCSV 返回逗号分隔的 key 串(供 JSON Schema 的 enum 拼装)。
func EventTypeCSV() string { return strings.Join(EventTypeKeys(), ",") }

// IsEventType 报告 key 是否为受支持的事件类型。
func IsEventType(key string) bool {
	for _, t := range EventTypes {
		if t.Key == key {
			return true
		}
	}
	return false
}
