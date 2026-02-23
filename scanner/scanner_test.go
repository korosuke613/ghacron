package scanner

import (
	"testing"

	"github.com/korosuke613/ghacron/github"
)

func TestParseFile_StandardCron(t *testing.T) {
	s := New(nil)
	repo := github.Repository{Owner: "test", Name: "repo", DefaultBranch: "main"}
	file := github.WorkflowFile{Name: "ci.yml", Path: ".github/workflows/ci.yml"}

	content := "on:\n  # ghacron: \"0 8 * * *\"\n  workflow_dispatch:\n"

	annotations, skipped := s.parseFile(repo, file, content)
	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}
	if annotations[0].CronExpr != "0 8 * * *" {
		t.Errorf("CronExpr = %q, want %q", annotations[0].CronExpr, "0 8 * * *")
	}
	if len(skipped) != 0 {
		t.Errorf("expected 0 skipped, got %d", len(skipped))
	}
}

func TestParseFile_CronTZ(t *testing.T) {
	s := New(nil)
	repo := github.Repository{Owner: "test", Name: "repo", DefaultBranch: "main"}
	file := github.WorkflowFile{Name: "ci.yml", Path: ".github/workflows/ci.yml"}

	content := "on:\n  # ghacron: \"CRON_TZ=Asia/Tokyo 0 8 * * *\"\n  workflow_dispatch:\n"

	annotations, skipped := s.parseFile(repo, file, content)
	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}
	if annotations[0].CronExpr != "CRON_TZ=Asia/Tokyo 0 8 * * *" {
		t.Errorf("CronExpr = %q, want %q", annotations[0].CronExpr, "CRON_TZ=Asia/Tokyo 0 8 * * *")
	}
	if len(skipped) != 0 {
		t.Errorf("expected 0 skipped, got %d", len(skipped))
	}
}

func TestParseFile_TZPrefix(t *testing.T) {
	s := New(nil)
	repo := github.Repository{Owner: "test", Name: "repo", DefaultBranch: "main"}
	file := github.WorkflowFile{Name: "ci.yml", Path: ".github/workflows/ci.yml"}

	content := "on:\n  # ghacron: \"TZ=UTC 30 6 * * 1-5\"\n  workflow_dispatch:\n"

	annotations, skipped := s.parseFile(repo, file, content)
	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}
	if annotations[0].CronExpr != "TZ=UTC 30 6 * * 1-5" {
		t.Errorf("CronExpr = %q, want %q", annotations[0].CronExpr, "TZ=UTC 30 6 * * 1-5")
	}
	if len(skipped) != 0 {
		t.Errorf("expected 0 skipped, got %d", len(skipped))
	}
}

func TestParseFile_InvalidTZ(t *testing.T) {
	s := New(nil)
	repo := github.Repository{Owner: "test", Name: "repo", DefaultBranch: "main"}
	file := github.WorkflowFile{Name: "ci.yml", Path: ".github/workflows/ci.yml"}

	content := "on:\n  # ghacron: \"CRON_TZ=Invalid/Zone 0 8 * * *\"\n  workflow_dispatch:\n"

	annotations, skipped := s.parseFile(repo, file, content)
	if len(annotations) != 0 {
		t.Fatalf("expected 0 annotations for invalid TZ, got %d", len(annotations))
	}
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped, got %d", len(skipped))
	}
	if skipped[0].CronExpr != "CRON_TZ=Invalid/Zone 0 8 * * *" {
		t.Errorf("skipped CronExpr = %q, want %q", skipped[0].CronExpr, "CRON_TZ=Invalid/Zone 0 8 * * *")
	}
	if skipped[0].Reason == "" {
		t.Error("skipped Reason should not be empty")
	}
}

func TestParseFile_NoWorkflowDispatch(t *testing.T) {
	s := New(nil)
	repo := github.Repository{Owner: "test", Name: "repo", DefaultBranch: "main"}
	file := github.WorkflowFile{Name: "ci.yml", Path: ".github/workflows/ci.yml"}

	content := "on:\n  # ghacron: \"0 8 * * *\"\n  push:\n"

	annotations, skipped := s.parseFile(repo, file, content)
	if len(annotations) != 0 {
		t.Fatalf("expected 0 annotations without workflow_dispatch, got %d", len(annotations))
	}
	if len(skipped) != 0 {
		t.Errorf("expected 0 skipped, got %d", len(skipped))
	}
}

func TestParseFile_AnnotationFields(t *testing.T) {
	s := New(nil)
	repo := github.Repository{Owner: "myorg", Name: "myrepo", DefaultBranch: "develop"}
	file := github.WorkflowFile{Name: "deploy.yml", Path: ".github/workflows/deploy.yml"}

	content := "on:\n  # ghacron: \"CRON_TZ=Asia/Tokyo 0 9 * * 1\"\n  workflow_dispatch:\n"

	annotations, _ := s.parseFile(repo, file, content)
	if len(annotations) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(annotations))
	}

	a := annotations[0]
	if a.Owner != "myorg" {
		t.Errorf("Owner = %q, want %q", a.Owner, "myorg")
	}
	if a.Repo != "myrepo" {
		t.Errorf("Repo = %q, want %q", a.Repo, "myrepo")
	}
	if a.WorkflowFile != "deploy.yml" {
		t.Errorf("WorkflowFile = %q, want %q", a.WorkflowFile, "deploy.yml")
	}
	if a.Ref != "develop" {
		t.Errorf("Ref = %q, want %q", a.Ref, "develop")
	}
}

func TestParseFile_SkippedFields(t *testing.T) {
	s := New(nil)
	repo := github.Repository{Owner: "myorg", Name: "myrepo", DefaultBranch: "main"}
	file := github.WorkflowFile{Name: "bad.yml", Path: ".github/workflows/bad.yml"}

	content := "on:\n  # ghacron: \"CRON_TZ=Asis/Tokyo 0 8 * * *\"\n  workflow_dispatch:\n"

	_, skipped := s.parseFile(repo, file, content)
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped, got %d", len(skipped))
	}
	sk := skipped[0]
	if sk.Owner != "myorg" {
		t.Errorf("Owner = %q, want %q", sk.Owner, "myorg")
	}
	if sk.Repo != "myrepo" {
		t.Errorf("Repo = %q, want %q", sk.Repo, "myrepo")
	}
	if sk.WorkflowFile != "bad.yml" {
		t.Errorf("WorkflowFile = %q, want %q", sk.WorkflowFile, "bad.yml")
	}
	if sk.CronExpr != "CRON_TZ=Asis/Tokyo 0 8 * * *" {
		t.Errorf("CronExpr = %q, want %q", sk.CronExpr, "CRON_TZ=Asis/Tokyo 0 8 * * *")
	}
}
