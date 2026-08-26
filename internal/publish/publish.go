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
func RenderEvent(it store.EventForPublish, ev *model.Evidence) string {
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
		fmt.Fprintf(&b, "- [[%s]]\n", a) // 未解析链接,迭代 3 建实体后自动可跳
	}

	b.WriteString("\n## 证据\n")
	b.WriteString(renderEvidence(ev))
	b.WriteString("\n")

	b.WriteString("\n## AI 分析\n")
	b.WriteString("> 本卡片的 事实/影响 由 AI 抽取,未经人工复核。推测性判断不在本卡片,请填写到下方\"我的理解\"。\n")

	b.WriteString("\n## 我的理解\n")
	b.WriteString("> 等待填写(用户在此写 Inference / Belief,遵循 Fact≠Inference≠Belief)\n")

	return b.String()
}

func renderEvidence(ev *model.Evidence) string {
	if ev == nil || ev.ID == "" {
		return "- _无(待补充)_"
	}
	title := ev.Claim
	if ev.Title != nil && strings.TrimSpace(*ev.Title) != "" {
		title = strings.TrimSpace(*ev.Title)
	}
	if ev.URL != nil && strings.TrimSpace(*ev.URL) != "" {
		return fmt.Sprintf("[%s](%s)", title, strings.TrimSpace(*ev.URL))
	}
	return "- " + title
}

// vaultReadme 首次初始化时写入的 vault 说明。
const vaultReadme = `# PIKS-Vault

本仓库由 PIKS 服务器自动生成(Generated Knowledge),与代码仓库分离。

- 05-Events/  事件卡片(Fact 层,AI 抽取,未经人工复核)
- 09-Personal/ 个人认知(User Knowledge,本地维护)

请勿手动修改 01-08 下的 Generated 文件,修改会与下次发布冲突。
我的理解/判断写进"我的理解"节或 09-Personal/。
`

const vaultGitignore = `*:Zone.Identifier
.DS_Store
`

// CommitVault 把生成的卡片提交进独立 git 仓库。返回提交数;err 非 nil 时 git 步失败(文件已落盘,仅记录)。
// 推送可选:设置 PIKS_VAULT_REMOTE 后自动 push。
func CommitVault(vault string) (int, error) {
	if err := os.MkdirAll(vault, 0o755); err != nil {
		return 0, err
	}
	for name, content := range map[string]string{
		"README.md":    vaultReadme,
		".gitignore":   vaultGitignore,
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
	// 无变更则跳过提交(幂等)。
	out, err := exec.Command("git", "-C", vault, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return 0, err
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return 0, nil
	}
	msg := fmt.Sprintf("publish: 事件卡片更新 %s", time.Now().Format("2006-01-02"))
	if err := run(vault, "git", "-c", "user.name="+gitName(), "-c", "user.email="+gitEmail(),
		"commit", "-q", "-m", msg); err != nil {
		return 0, err
	}
	if remote := os.Getenv("PIKS_VAULT_REMOTE"); remote != "" {
		// 推送失败不阻断发布,记录即可
		_ = run(vault, "git", "push", "-q")
	}
	return 1, nil
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
