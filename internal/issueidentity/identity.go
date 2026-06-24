package issueidentity

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/local/symphony/internal/domain"
)

// Identity derives stable branch and workspace names for one Task Issue.
type Identity struct {
	issue domain.Issue
}

// For returns the execution identity for a Task Issue.
func For(issue domain.Issue) Identity {
	return Identity{issue: issue}
}

// BranchName returns the deterministic execution branch name for the Task Issue.
func (i Identity) BranchName() string {
	prefix := strings.Trim(i.issue.ProjectID, "/")
	if prefix == "" {
		prefix = "symphony"
	} else {
		prefix = "symphony/" + branchSlug(prefix)
	}
	return prefix + "/" + issueKey(i.issue, branchSlug)
}

// WorkspaceKey returns the relative workspace path for the Task Issue.
func (i Identity) WorkspaceKey() string {
	project := strings.TrimSpace(i.issue.ProjectID)
	if project == "" {
		project = "unknown"
	}
	return filepath.Join(sanitizeProject(project), issueKey(i.issue, workspaceSlug))
}

func issueKey(issue domain.Issue, slug func(string) string) string {
	number := issueNumber(issue)
	if number == "" {
		number = "0"
	}
	titleSlug := slug(issue.Title)
	if titleSlug == "" {
		titleSlug = "task"
	}
	if len(titleSlug) > 48 {
		titleSlug = strings.Trim(titleSlug[:48], "-")
	}
	return "issue-" + number + "-" + titleSlug
}

var numberPattern = regexp.MustCompile(`(\d+)`)

func issueNumber(issue domain.Issue) string {
	for _, value := range []string{issue.ID, issue.Identifier} {
		if match := numberPattern.FindString(strings.TrimSpace(value)); match != "" {
			return match
		}
	}
	return ""
}

func branchSlug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func workspaceSlug(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func sanitizeProject(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
