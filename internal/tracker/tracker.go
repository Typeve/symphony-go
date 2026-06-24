package tracker

import (
	"context"

	"github.com/local/symphony/internal/domain"
)

// Client is the interface for interacting with an issue tracker.
type Client interface {
	// FetchPendingIssues returns Task Issues that match the Managed Project's
	// active states and do not already carry a Completion Marker.
	FetchPendingIssues(ctx context.Context, project domain.ProjectConfig) ([]domain.Issue, error)
	MarkStatus(ctx context.Context, issue domain.Issue, status domain.Status, publish ...domain.PublishResult) error
}
