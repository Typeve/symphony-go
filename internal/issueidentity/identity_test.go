package issueidentity

import (
	"path/filepath"
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

func TestIdentityUsesIssueIDBeforeIdentifier(t *testing.T) {
	identity := For(domain.Issue{
		ProjectID:  "p",
		ID:         "42",
		Identifier: "acme/app#99",
		Title:      "Do work",
	})

	if got := identity.BranchName(); got != "symphony/p/issue-42-do-work" {
		t.Fatalf("BranchName() = %q", got)
	}
	if got := identity.WorkspaceKey(); got != filepath.Join("p", "issue-42-do-work") {
		t.Fatalf("WorkspaceKey() = %q", got)
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
