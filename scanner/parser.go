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

		// Detect start of on: section (handles workflow_dispatch on the same line).
		if isOnSectionStart(trimmed) {
			inOn = true
			if strings.Contains(trimmed, "workflow_dispatch") {
				return true
			}
			continue
		}

		if !inOn {
			continue
		}
		// A new top-level key ends the on: section.
		if isTopLevelKey(line, trimmed) {
			inOn = false
			continue
		}
		if strings.Contains(trimmed, "workflow_dispatch") {
			return true
		}
	}

	return false
}

// isOnSectionStart reports whether a trimmed line begins the on: section.
func isOnSectionStart(trimmed string) bool {
	return trimmed == "on:" || strings.HasPrefix(trimmed, "on:")
}

// isTopLevelKey reports whether a line is an unindented, non-empty entry, which
// marks the end of the current section.
func isTopLevelKey(line, trimmed string) bool {
	return len(line) > 0 && line[0] != ' ' && line[0] != '\t' && trimmed != ""
}
