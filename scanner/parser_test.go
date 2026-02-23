package scanner

import (
	"testing"
)

func TestParseAnnotations(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "standard 5-field cron",
			content:  `# ghacron: "0 8 * * *"`,
			expected: []string{"0 8 * * *"},
		},
		{
			name:     "CRON_TZ prefix",
			content:  `# ghacron: "CRON_TZ=Asia/Tokyo 0 8 * * *"`,
			expected: []string{"CRON_TZ=Asia/Tokyo 0 8 * * *"},
		},
		{
			name:     "TZ prefix",
			content:  `# ghacron: "TZ=UTC 30 6 * * 1-5"`,
			expected: []string{"TZ=UTC 30 6 * * 1-5"},
		},
		{
			name:     "single quotes",
			content:  `# ghacron: 'CRON_TZ=America/New_York 0 9 * * *'`,
			expected: []string{"CRON_TZ=America/New_York 0 9 * * *"},
		},
		{
			name: "multiple annotations",
			content: "# ghacron: \"0 8 * * *\"\n" +
				"# ghacron: \"CRON_TZ=Asia/Tokyo 30 18 * * *\"",
			expected: []string{"0 8 * * *", "CRON_TZ=Asia/Tokyo 30 18 * * *"},
		},
		{
			name:     "no annotations",
			content:  "on:\n  workflow_dispatch:\n",
			expected: nil,
		},
		{
			name:     "indented annotation",
			content:  `  # ghacron: "0 8 * * *"`,
			expected: []string{"0 8 * * *"},
		},
		{
			name:     "inline comment breaks match",
			content:  `# ghacron: "0 8 * * *"  # some comment`,
			expected: nil,
		},
		{
			name:     "empty expression ignored",
			content:  `# ghacron: ""`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAnnotations(tt.content)
			if len(got) != len(tt.expected) {
				t.Fatalf("got %d annotations, want %d: %v", len(got), len(tt.expected), got)
			}
			for i, expr := range got {
				if expr != tt.expected[i] {
					t.Errorf("annotation[%d] = %q, want %q", i, expr, tt.expected[i])
				}
			}
		})
	}
}

func TestHasWorkflowDispatch(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "standard on section",
			content:  "on:\n  workflow_dispatch:\n",
			expected: true,
		},
		{
			name:     "no workflow_dispatch",
			content:  "on:\n  push:\n    branches:\n      - main\n",
			expected: false,
		},
		{
			name:     "workflow_dispatch with other triggers",
			content:  "on:\n  push:\n  workflow_dispatch:\n",
			expected: true,
		},
		{
			name:     "empty file",
			content:  "",
			expected: false,
		},
		{
			name:     "workflow_dispatch outside on section",
			content:  "on:\n  push:\njobs:\n  workflow_dispatch:\n",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasWorkflowDispatch(tt.content)
			if got != tt.expected {
				t.Errorf("HasWorkflowDispatch() = %v, want %v", got, tt.expected)
			}
		})
	}
}
