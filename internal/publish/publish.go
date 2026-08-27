// Package publish 事件卡片渲染与 vault 同步。
// DB(Fact 层)→ Markdown(front matter + wikilink)→ Obsidian。遵守设计 §7 模板。
package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"piks/internal/model"
	"piks/internal/store"
)

// EventPath 卡片路径:05-Events/{event_type}/event-{uuid前8}.md
func EventPath(vault string, it store.EventForPublish) string {
	name := "event-" + shortID(it.ID)
	return filepath.Join(vault, "05-Events", it.EventType, name+".md")
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

// RenderEvent 按设计 §7 模板渲染单张事件卡片。
// 严格遵守 Fact≠Inference≠Belief:卡片只有 AI 抽取的事实;推测留给"我的理解"占位。
// resolve:affected 词 → 实体 wikilink 目标(迭代 3,设计 §3.3)。命中 → [[03-Entities/{type}/{name}|原词]],未命中保持纯文本(诚实)。
func RenderEvent(it store.EventForPublish, evs []model.Evidence, resolve func(string) (string, bool)) string {
	var b strings.Builder

	date := it.CreatedAt.Format("2006-01-02")
	if it.OccurredAt != nil {
		date = it.OccurredAt.Format("2006-01-02")
	}
	fmt.Fprintf(&b, "---\nid: event-%s\ntype: event\ndate: %s\n", shortID(it.ID), date)
	fmt.Fprintf(&b, "status: %s\n", it.Status)
	fmt.Fprintf(&b, "source: %s\n", it.SourceName)
	fmt.Fprintf(&b, "confidence: %.2f\n", it.Confidence)
	if it.PipelineVersion != nil {
		fmt.Fprintf(&b, "pipeline: %s\n", *it.PipelineVersion)
	}
	b.WriteString("---\n\n")

	fmt.Fprintf(&b, "# %s\n\n", it.Title)

	b.WriteString("## 发生了什么\n")
	if it.Summary != nil && strings.TrimSpace(*it.Summary) != "" {
		fmt.Fprintf(&b, "%s\n", strings.TrimSpace(*it.Summary))
	} else {
		b.WriteString("_暂无摘要_\n")
	}

	b.WriteString("\n## 事实\n")
	var facts []string
	_ = json.Unmarshal(it.Facts, &facts)
	if len(facts) == 0 {
		b.WriteString("- _无_\n")
	}
	for _, f := range facts {
		fmt.Fprintf(&b, "- %s\n", f)
	}

	b.WriteString("\n## 影响\n")
	var affected []string
	_ = json.Unmarshal(it.Affected, &affected)
	if len(affected) == 0 {
		b.WriteString("- _无(原文未提及实体)_\n")
	}
	for _, a := range affected {
		if target, ok := resolve(a); ok {
			fmt.Fprintf(&b, "- [[%s|%s]]\n", target, a)
		} else {
			fmt.Fprintf(&b, "- %s\n", a) // 未匹配到实体 → 纯文本(诚实,不假造链接)
		}
	}

	b.WriteString("\n## 证据\n")
	b.WriteString(renderEvidence(evs))
	b.WriteString("\n")

	b.WriteString("\n## AI 分析\n")
	b.WriteString("> 本卡片的 事实/影响 由 AI 抽取,未经人工复核。推测性判断不在本卡片,请填写到下方\"我的理解\"。\n")

	b.WriteString("\n## 我的理解\n")
	b.WriteString("> 等待填写(用户在此写 Inference / Belief,遵循 Fact≠Inference≠Belief)\n")

	return b.String()
}

func renderEvidence(evs []model.Evidence) string {
	if len(evs) == 0 {
		return "- _无(待补充)_"
	}
	var b strings.Builder
	for _, ev := range evs {
		title := ev.Claim
		if ev.Title != nil && strings.TrimSpace(*ev.Title) != "" {
			title = strings.TrimSpace(*ev.Title)
		}
		if ev.URL != nil && strings.TrimSpace(*ev.URL) != "" {
			fmt.Fprintf(&b, "- [%s](%s)\n", title, strings.TrimSpace(*ev.URL))
		} else {
			fmt.Fprintf(&b, "- %s\n", title)
		}
	}
	return b.String()
}

// vaultReadme 首次初始化时写入的 vault 说明。
// 设计(D6 细化):vault 根 = Generated 独立仓库(服务器生成,01-08);09-Personal = 个人独立仓库(本地维护)。
const vaultReadme = `# PIKS-Vault

用 Obsidian 打开本目录作为 vault。内含两个独立 git 仓库(架构文档 §18/§19):

- **Generated(本仓库根,服务器生成)**:01-08。如 05-Events 事件卡片(Fact 层,AI 抽取,未经人工复核)。
- **Personal(独立仓库,09-Personal/)**:你的个人认知,本地维护,服务器不写入、不推送。

## 约定
- Generated 内容请勿手动修改,修改会与下次发布冲突(git 层面也做了忽略隔离)。
- 我的理解/判断写进卡片"我的理解"节,或 09-Personal/ 下的个人笔记。
`

const vaultGitignore = `*:Zone.Identifier
.DS_Store
# 个人知识独立仓库,与 Generated 隔离
09-Personal/
`

// personalReadme 09-Personal 个人仓库的初始化说明。
const personalReadme = `# 09-Personal — 个人认知(User Knowledge)

> 独立 git 仓库,由你本人维护。服务器不写入、不推送本目录(架构文档 §18/§19)。

## 内容(遵循 Fact≠Inference≠Belief)
- 我的理解 / 判断 / 复盘 / 错误记录
- Belief(信念)、Case(案例)、Mistake(错误)——迭代 4 提供收割器回写

## 结构(随使用补充)
- Mistakes/  错误复盘(架构文档 §20 要求)

## 约定
- 与 Generated 内容用双链 [[...]] 互联(如事件卡片、实体)
- 本仓库不会被服务器发布逻辑触碰,可放心编辑
`

// ensurePersonalRepo 初始化 09-Personal 独立 git 仓库(幂等)。只 init 不提交——内容由用户维护。
func ensurePersonalRepo(vault string) error {
	dir := filepath.Join(vault, "09-Personal")
	if err := os.MkdirAll(filepath.Join(dir, "Mistakes"), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(personalReadme), 0o644); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		if err := run(dir, "git", "init", "-q"); err != nil {
			return err
		}
	}
	return nil
}

// CommitVault 把生成的卡片提交进独立 git 仓库(Generated 侧)。返回提交数;err 非 nil 时 git 步失败(文件已落盘,仅记录)。
func CommitVault(vault string) (int, error) {
	return CommitVaultWithMsg(vault, fmt.Sprintf("publish: 事件卡片更新 %s", time.Now().Format("2006-01-02")))
}

// CommitVaultWithMsg 同 CommitVault,但允许自定义提交信息(每日复盘用独立消息)。
// 幂等:git add -A 后 status --porcelain 为空 → 0 提交(重跑零提交,设计 §5.6)。
func CommitVaultWithMsg(vault, msg string) (int, error) {
	if err := os.MkdirAll(vault, 0o755); err != nil {
		return 0, err
	}
	if err := ensurePersonalRepo(vault); err != nil {
		return 0, err
	}
	for name, content := range map[string]string{
		"README.md":  vaultReadme,
		".gitignore": vaultGitignore,
	} {
		p := filepath.Join(vault, name)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				return 0, err
			}
		}
	}
	if _, err := os.Stat(filepath.Join(vault, ".git")); os.IsNotExist(err) {
		if err := run(vault, "git", "init", "-q"); err != nil {
			return 0, err
		}
	}
	if err := run(vault, "git", "add", "-A"); err != nil {
		return 0, err
	}
	// 无变更则跳过提交(幂等);但仍推送一次,把上次可能未推上去的提交补推(up-to-date 时 push 为 no-op)。
	out, err := exec.Command("git", "-C", vault, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return 0, err
	}
	remote := os.Getenv("PIKS_VAULT_REMOTE")
	if len(strings.TrimSpace(string(out))) == 0 {
		if remote != "" {
			pushVault(vault)
		}
		return 0, nil
	}
	if err := run(vault, "git", "-c", "user.name="+gitName(), "-c", "user.email="+gitEmail(),
		"commit", "-q", "-m", msg); err != nil {
		return 0, err
	}
	if remote != "" {
		pushVault(vault)
	}
	return 1, nil
}

// pushVault 推送 Generated 仓到远端。失败不阻断发布(设计 D-P10),但写入 stderr 进管线日志,下次运行重试。
func pushVault(vault string) {
	if err := run(vault, "git", "push", "-q"); err != nil {
		fmt.Fprintf(os.Stderr, "warn: vault push failed (will retry next run): %v\n", err)
	}
}

// GitShort 代码仓库当前短哈希(最佳努力;失败返回空)。用于卡片 pipeline 血缘。
// 生产容器内无 .git → 优先取 PIKS_GIT_SHORT 环境变量(deploy 时随镜像烘焙)。
func GitShort() string {
	if v := os.Getenv("PIKS_GIT_SHORT"); v != "" {
		return v
	}
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func gitName() string {
	if v := os.Getenv("PIKS_VAULT_GIT_NAME"); v != "" {
		return v
	}
	return "glacierzzz26"
}

func gitEmail() string {
	if v := os.Getenv("PIKS_VAULT_GIT_EMAIL"); v != "" {
		return v
	}
	return "glacierzzz26@gmail.com"
}

func run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
