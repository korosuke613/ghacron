package scanner

import (
	"regexp"
	"strings"
)

// Regex for extracting annotations.
// Format: # ghacron: "0 8 * * *" or # ghacron: '0 8 * * *'
var annotationRe = regexp.MustCompile(`^\s*#\s*ghacron:\s*["'](.+?)["']\s*$`)

// ParseAnnotations extracts cron annotations from workflow file content.
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

// HasWorkflowDispatch checks if workflow_dispatch is in the on: section.
func HasWorkflowDispatch(content string) bool {
	lines := strings.Split(content, "\n")
	inOn := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of on: section
		if trimmed == "on:" || strings.HasPrefix(trimmed, "on:") {
			inOn = true
			// workflow_dispatch on the same line as on:
			if strings.Contains(trimmed, "workflow_dispatch") {
				return true
			}
			continue
		}

		if inOn {
			// End of on: section when indentation stops
			if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && trimmed != "" {
				inOn = false
				continue
			}
			// Detect workflow_dispatch
			if strings.Contains(trimmed, "workflow_dispatch") {
				return true
			}
		}
	}

	return false
}
