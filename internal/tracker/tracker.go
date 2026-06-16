package tracker

import (
	"context"

	"github.com/local/symphony/internal/domain"
)

// Client is the interface for interacting with an issue tracker.
type Client interface {
	FetchIssues(ctx context.Context, project domain.ProjectConfig) ([]domain.Issue, error)
	MarkStatus(ctx context.Context, issue domain.Issue, status domain.Status) error
}
