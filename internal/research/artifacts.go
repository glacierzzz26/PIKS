// Package research 深研编排:exec Python CLI → 读产物 → LLM 合成 → 落 PG。
//
// 独立性契约(design research-merge.md §4.10 D-11):本包只与 research/ 经
// 「运行时产物 + CLI 参数」交互,不 import research 的源文件。
package research

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ContractVersion 本端支持的产物契约版本(§4.10 G2)。
// research 侧 run_meta.json 的 contract 高于本值 → 如实失败,不猜测不硬解。
const ContractVersion = 1

// run_meta.json 的 Go 侧映射(§4.10 G2;未列字段按契约"只做加法"规则忽略)。
type runMeta struct {
	RunID         string         `json:"run_id"`
	Symbol        string         `json:"symbol"`
	Profile       string         `json:"profile"`
	Mode          string         `json:"mode"`
	AsOf          string         `json:"as_of"`
	Sections      []string       `json:"sections"`
	ProviderCalls map[string]int `json:"provider_calls"`
	Contract      int            `json:"contract"`
}

// artifacts 一次深研的产物目录句柄(文件名契约见设计 §4.5 / research/README.md)。
type artifacts struct {
	dir    string
	code   string // research 侧输入的 6 位代码(文件名前缀)
	symbol string // run_meta.symbol 回填的 full_code
	meta   runMeta
}

// openArtifacts 读 run_meta.json 并校验契约版本(§4.10 G2)。
// 版本高于本端支持 → 明确报错,调用方置 status=failed,不进后续步骤。
func openArtifacts(dir, code string) (*artifacts, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "run_meta.json"))
	if err != nil {
		return nil, fmt.Errorf("读 run_meta.json 失败(采集未产出?): %w", err)
	}
	var m runMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("解析 run_meta.json 失败: %w", err)
	}
	if m.Contract > ContractVersion {
		return nil, fmt.Errorf(
			"research 产物契约 v%d 高于本端支持的 v%d,请升级 PIKS", m.Contract, ContractVersion)
	}
	return &artifacts{dir: dir, code: code, symbol: m.Symbol, meta: m}, nil
}

// path 产物绝对路径(文件名冻结,见 research/README.md 契约表)。
func (a *artifacts) path(suffix string) string {
	return filepath.Join(a.dir, a.code+"_"+suffix)
}

func (a *artifacts) promptPath() string   { return a.path("synthesis_prompt.txt") }
func (a *artifacts) metricsPath() string  { return a.path("metrics.json") }
func (a *artifacts) skeletonPath() string { return a.path("skeleton.md") }
func (a *artifacts) finalPath() string    { return a.path("final.md") }
func (a *artifacts) lintPath() string     { return a.path("lint.json") }
func (a *artifacts) synthPath() string    { return a.path("synthesis.json") }
func (a *artifacts) gatePath() string     { return a.path("gate.json") }

// exists 判断某产物是否已存在(断点重跑:已产出的步骤跳过)。
func (a *artifacts) exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Size() > 0
}

// readJSON 读 JSON 产物;文件缺失返回 nil(不是错误,调用方按需处理)。
func readJSON(p string) (json.RawMessage, error) {
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// readText 读文本产物(缺失 → 空串,不报错)。
func readText(p string) (string, error) {
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// writeJSON 写 LLM 合成结果供 `synthesize` 子命令读(--synthesis-file)。
func writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}
