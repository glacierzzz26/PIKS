package research

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 各步骤超时(设计 §4.4):采集最慢(akshare 多次网络调用),合成是 LLM,机检纯本地计算。
const (
	TimeoutGather = 180 * time.Second
	TimeoutSynth  = 120 * time.Second
	TimeoutVerify = 30 * time.Second
	// TimeoutTotal 整轮编排的兜底上限(web 触发端用;步骤超时之和再加余量)。
	TimeoutTotal = 6 * time.Minute
)

// cliRunner 编排器依赖的 Python CLI 执行面(D-6:os/exec,零胶水)。
// *runner 是生产实现;测试注入固定产物 fixture 的假实现,
// 使「产物契约 + 状态机」可在无 Python 运行时下验证(§5.6 最小版本测试)。
type cliRunner interface {
	gather(ctx context.Context, code, profile string, days int, outDir, runID string) (string, error)
	// priorMetricsPath 非空 = 该文件的历史数字并入 Number Lint 的 known 集(issue #8)。
	synthesize(ctx context.Context, dir, code, synthFile, priorMetricsPath string) (string, error)
	gate(ctx context.Context, dir, code string) (string, error)
}

// runner 执行 research 的 Python CLI。
type runner struct {
	pythonBin string // venv 解释器(PIKS_PYTHON_BIN 覆盖)
	srcDir    string // research/ 根(含 src/),作为 cwd 让 `-m src.cli` 可解析
}

// newRunner 解析解释器与源码目录。
// dev:research/.venv/bin/python3;生产镜像:python3 + /app/research(见 Dockerfile research target)。
func newRunner() *runner {
	root := pythonRoot()
	bin := os.Getenv("PIKS_PYTHON_BIN")
	if bin == "" {
		// 优先 venv(dev),不存在则退回 PATH 上的 python3(生产镜像 venv 直装系统 site-packages)。
		venv := filepath.Join(root, ".venv", "bin", "python3")
		if _, err := os.Stat(venv); err == nil {
			bin = venv
		} else {
			bin = "python3"
		}
	}
	return &runner{pythonBin: bin, srcDir: root}
}

// pythonRoot 定位 research/ 目录:环境变量优先,否则相对可执行文件/当前目录向上找。
func pythonRoot() string {
	if v := os.Getenv("PIKS_RESEARCH_DIR"); v != "" {
		return v
	}
	// 从 cwd 向上最多 4 层找含 research/src/cli.py 的目录(dev 在仓库根跑)。
	dir, _ := os.Getwd()
	for i := 0; i < 5 && dir != "/" && dir != ""; i++ {
		cand := filepath.Join(dir, "research")
		if _, err := os.Stat(filepath.Join(cand, "src", "cli.py")); err == nil {
			return cand
		}
		dir = filepath.Dir(dir)
	}
	return "research"
}

// exec 跑 python -m src.cli <args...>,错误时把 stderr 摘要带回(如实,不吞)。
// exitCodes 中的非零码视为"有效失败"由调用方处理(如 lint 未过 = 2、gate 未过 = 3),
// 不算执行错误——脚本已把结果写进产物文件。
func (r *runner) exec(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	full := append([]string{"-m", "src.cli"}, args...)
	cmd := exec.CommandContext(ctx, r.pythonBin, full...)
	cmd.Dir = r.srcDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	// 让子进程用 UTF-8 输出(容器 locale 常为 POSIX)。
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUNBUFFERED=1")

	err := cmd.Run()
	out := stdout.String()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			detail := stderr.String()
			if strings.TrimSpace(detail) == "" {
				detail = out
			}
			return out, fmt.Errorf("超时(%s): %s", timeout, tail(detail))
		}
		var ee *exec.ExitError
		if isExitError(err, &ee) && allowNonZero(ee.ExitCode()) {
			// lint/gate 未过:脚本已落产物,交调用方按结果处理。
			return out, nil
		}
		// python CLI 的诊断多走 stdout(print),stderr 常为空 —— 空则回退 stdout,
		// 否则 error 列只剩 "exit status 1",排查无门。
		detail := stderr.String()
		if strings.TrimSpace(detail) == "" {
			detail = out
		}
		return out, fmt.Errorf("cli %s 失败: %v; 输出: %s", strings.Join(args, " "), err, tail(detail))
	}
	return out, nil
}

// gather 采集 + 确定性分析 → 骨架报告 + 指标卡 + 合成提示 + run_meta。
// runID 由 Go 生成并回传(--run-id),使 run_meta.json 的幂等键与 research_runs.run_id 一致。
func (r *runner) gather(ctx context.Context, code, profile string, days int, outDir, runID string) (string, error) {
	args := []string{"research", code, "--profile", profile, "--out-dir", outDir}
	if days > 0 {
		args = append(args, "--days", fmt.Sprint(days))
	}
	if runID != "" {
		args = append(args, "--run-id", runID)
	}
	return r.exec(ctx, TimeoutGather, args...)
}

// synthesize 把 LLM 三段定性渲染进报告 + Number Lint。
// priorMetricsPath 非空时透传 --prior-metrics,让历史研报的数字并入 known 集(issue #8);
// 为空则命令行与改动前逐字节一致(首次研报 / 未开 PriorRuns)。
func (r *runner) synthesize(ctx context.Context, dir, code, synthFile, priorMetricsPath string) (string, error) {
	args := []string{"synthesize", dir, code, "--synthesis-file", synthFile}
	if priorMetricsPath != "" {
		args = append(args, "--prior-metrics", priorMetricsPath)
	}
	return r.exec(ctx, TimeoutSynth, args...)
}

// gate 六项 Quality Gate 机检。
func (r *runner) gate(ctx context.Context, dir, code string) (string, error) {
	return r.exec(ctx, TimeoutVerify, "gate", dir, code, "--json")
}

// allowNonZero `synthesize`(lint 未过=2)/`gate`(机检未过=3)的约定退出码:
// 非零是"结果未通过"而非"执行失败",结果已在产物文件里。
func allowNonZero(code int) bool { return code == 2 || code == 3 }

func isExitError(err error, out **exec.ExitError) bool {
	ee, ok := err.(*exec.ExitError)
	if ok {
		*out = ee
	}
	return ok
}

// tail 截取末尾若干字符(错误摘要用,避免把整段日志塞进 error 列)。
func tail(s string) string {
	s = strings.TrimSpace(s)
	const n = 600
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
