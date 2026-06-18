package issueidentity

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/local/symphony/internal/domain"
)

func TestIdentityDerivesBranchNameAndWorkspaceKey(t *testing.T) {
	identity := For(domain.Issue{
		ProjectID:  "project one",
		Identifier: "acme/app#42",
		Title:      "Fix Login Error!",
	})

	if got := identity.BranchName(); got != "symphony/project-one/issue-42-fix-login-error" {
		t.Fatalf("BranchName() = %q", got)
	}
	if got := identity.WorkspaceKey(); got != filepath.Join("project_one", "issue-42-fix-login-error") {
		t.Fatalf("WorkspaceKey() = %q", got)
	}
}

func TestIdentityUsesSourceIDBeforeIssueID(t *testing.T) {
	identity := For(domain.Issue{
		ProjectID: "p",
		SourceID:  "source-99",
		ID:        "42",
		Title:     "Do work",
	})

	if !strings.Contains(identity.BranchName(), "issue-99-do-work") {
		t.Fatalf("BranchName() = %q, want source issue number", identity.BranchName())
	}
	if !strings.Contains(identity.WorkspaceKey(), "issue-99-do-work") {
		t.Fatalf("WorkspaceKey() = %q, want source issue number", identity.WorkspaceKey())
	}
}

func TestIdentityDefaultsMissingProjectNumberAndTitle(t *testing.T) {
	identity := For(domain.Issue{})

	if got := identity.BranchName(); got != "symphony/issue-0-task" {
		t.Fatalf("BranchName() = %q", got)
	}
	if got := identity.WorkspaceKey(); got != filepath.Join("unknown", "issue-0-task") {
		t.Fatalf("WorkspaceKey() = %q", got)
	}
}
