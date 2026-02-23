package github

// CronAnnotation represents a cron annotation extracted from a workflow file.
type CronAnnotation struct {
	Owner        string // repository owner
	Repo         string // repository name
	WorkflowFile string // workflow file name (e.g. "build.yml")
	CronExpr     string // cron expression (5-field standard format)
	Ref          string // default branch
}

// CronJobKey uniquely identifies a cron job.
type CronJobKey struct {
	Owner        string
	Repo         string
	WorkflowFile string
	CronExpr     string
}

// Key generates a CronJobKey from a CronAnnotation.
func (a *CronAnnotation) Key() CronJobKey {
	return CronJobKey{
		Owner:        a.Owner,
		Repo:         a.Repo,
		WorkflowFile: a.WorkflowFile,
		CronExpr:     a.CronExpr,
	}
}

// Repository represents a GitHub App installation repository.
type Repository struct {
	Owner         string
	Name          string
	DefaultBranch string
}

// WorkflowFile represents a workflow file in a repository.
type WorkflowFile struct {
	Name string // file name (e.g. "build.yml")
	Path string // full path (e.g. ".github/workflows/build.yml")
}
