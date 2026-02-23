package scanner

import (
	"regexp"
	"strings"
)

// アノテーション抽出用正規表現
// 形式: # gh-cron-trigger: "0 8 * * *" または # gh-cron-trigger: '0 8 * * *'
var annotationRe = regexp.MustCompile(`^\s*#\s*gh-cron-trigger:\s*["'](.+?)["']\s*$`)

// ParseAnnotations ワークフローファイルの内容からcronアノテーションを抽出
func ParseAnnotations(content string) []string {
	var exprs []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		matches := annotationRe.FindStringSubmatch(line)
		if len(matches) >= 2 {
			expr := strings.TrimSpace(matches[1])
			if expr != "" {
				exprs = append(exprs, expr)
			}
		}
	}

	return exprs
}

// HasWorkflowDispatch on: セクションに workflow_dispatch が含まれているかチェック
func HasWorkflowDispatch(content string) bool {
	lines := strings.Split(content, "\n")
	inOn := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// on: セクションの開始を検出
		if trimmed == "on:" || strings.HasPrefix(trimmed, "on:") {
			inOn = true
			// on: の同一行に workflow_dispatch がある場合
			if strings.Contains(trimmed, "workflow_dispatch") {
				return true
			}
			continue
		}

		if inOn {
			// インデントがなくなったら on: セクション終了
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && trimmed != "" {
				inOn = false
				continue
			}
			// workflow_dispatch を検出
			if strings.Contains(trimmed, "workflow_dispatch") {
				return true
			}
		}
	}

	return false
}
