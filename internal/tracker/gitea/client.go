package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/local/symphony/internal/domain"
)

const defaultPageSize = 50
const maxCommentDetailLen = 1000

// Client is a Gitea REST client that implements tracker.Client.
type Client struct {
	endpoint   string
	token      string
	httpClient *http.Client
	repos      map[string]string // project ID → "owner/repo"
}

// New creates a Gitea client. The httpClient may be nil to use http.DefaultClient.
func New(endpoint, token string, projects []domain.ProjectConfig, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	repos := make(map[string]string, len(projects))
	for _, p := range projects {
		if slug := repoSlug(p.RepoURL); slug != "" {
			repos[p.ID] = slug
		}
	}
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		token:      token,
		httpClient: httpClient,
		repos:      repos,
	}
}

func repoSlug(repoURL string) string {
	u, err := url.Parse(strings.TrimSpace(repoURL))
	if err != nil || u.Host == "" {
		return ""
	}
	p := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.SplitN(p, "/", 3)
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return parts[0] + "/" + parts[1]
	}
	return ""
}

func splitSlug(slug string) (string, string) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// FetchPendingIssues fetches Task Issues from the Gitea repo associated with
// the Managed Project, restricted to configured active states and excluding
// issues that already carry a Completion Marker.
func (c *Client) FetchPendingIssues(ctx context.Context, project domain.ProjectConfig) ([]domain.Issue, error) {
	owner, repo := splitSlug(c.repos[project.ID])
	if owner == "" || repo == "" {
		owner, repo = splitSlug(repoSlug(project.RepoURL))
	}
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("gitea: cannot determine owner/repo for project %q", project.ID)
	}

	states, err := giteaStates(project.ActiveStates)
	if err != nil {
		return nil, err
	}

	var out []domain.Issue
	seen := map[string]struct{}{}
	for _, state := range states {
		issues, err := c.fetchIssuesByState(ctx, owner, repo, state)
		if err != nil {
			return nil, err
		}
		for _, issue := range issues {
			if hasManagedStatusLabel(issue.Labels) {
				continue
			}
			if _, ok := seen[issue.ID]; ok {
				continue
			}
			seen[issue.ID] = struct{}{}
			issue.ProjectID = project.ID
			out = append(out, issue)
		}
	}
	return out, nil
}

// MarkStatus adds a status label and comment to the Gitea issue.
func (c *Client) MarkStatus(ctx context.Context, issue domain.Issue, update domain.StatusUpdate) error {
	owner, repo := splitSlug(c.repos[issue.ProjectID])
	if owner == "" || repo == "" {
		return fmt.Errorf("gitea: unknown project %q for issue %s", issue.ProjectID, issue.ID)
	}

	number, err := strconv.Atoi(strings.TrimSpace(issue.ID))
	if err != nil || number <= 0 {
		return fmt.Errorf("gitea: issue id %q must be a positive issue number", issue.ID)
	}

	label := statusLabel(update.Status)
	if label == "" {
		return nil
	}
	if err := c.ensureLabel(ctx, owner, repo, label, statusDescription(update.Status)); err != nil {
		return err
	}
	if err := c.replaceIssueLabels(ctx, owner, repo, number, issue.Labels, label); err != nil {
		return err
	}
	return c.addIssueComment(ctx, owner, repo, number, statusComment(update))
}

func statusLabel(status domain.Status) string {
	switch status {
	case domain.StatusRunning:
		return "symphony-running"
	case domain.StatusDone:
		return "symphony-done"
	case domain.StatusFailed:
		return "symphony-failed"
	default:
		return ""
	}
}

func statusDescription(status domain.Status) string {
	switch status {
	case domain.StatusRunning:
		return "正在自动处理"
	case domain.StatusDone:
		return "自动处理完成"
	case domain.StatusFailed:
		return "自动处理失败"
	default:
		return ""
	}
}

func statusComment(update domain.StatusUpdate) string {
	switch update.Status {
	case domain.StatusRunning:
		return "任务已开始自动处理，请稍后查看后续结果。"
	case domain.StatusDone:
		if strings.TrimSpace(update.Publish.Branch) == "" && strings.TrimSpace(update.Publish.Commit) == "" {
			return "任务已自动处理完成。"
		}
		var b strings.Builder
		b.WriteString("任务已自动处理完成，execution branch 已推送，等待人工审核。")
		if branch := strings.TrimSpace(update.Publish.Branch); branch != "" {
			b.WriteString("\n\nBranch: `")
			b.WriteString(branch)
			b.WriteString("`")
		}
		if commit := strings.TrimSpace(update.Publish.Commit); commit != "" {
			b.WriteString("\nCommit: `")
			b.WriteString(commit)
			b.WriteString("`")
		}
		return b.String()
	case domain.StatusFailed:
		reason := commentDetail(update.FailureReason)
		workspace := commentDetail(update.WorkspacePath)
		if reason == "" && workspace == "" {
			return "任务自动处理失败，需要人工检查。"
		}
		var b strings.Builder
		b.WriteString("任务自动处理失败，需要人工检查。")
		if reason != "" {
			b.WriteString("\n\nReason: `")
			b.WriteString(reason)
			b.WriteString("`")
		}
		if workspace != "" {
			b.WriteString("\nWorkspace: `")
			b.WriteString(workspace)
			b.WriteString("`")
		}
		return b.String()
	default:
		return "任务状态已更新。"
	}
}

func commentDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxCommentDetailLen {
		return value
	}
	return value[:maxCommentDetailLen] + "...[truncated]"
}

func (c *Client) ensureLabel(ctx context.Context, owner, repo, label, description string) error {
	body := map[string]any{
		"name":        label,
		"color":       "ededed",
		"description": description,
	}
	return c.postJSON(ctx, c.labelsURL(owner, repo), body, http.StatusOK, http.StatusCreated, http.StatusConflict, http.StatusUnprocessableEntity)
}

func (c *Client) replaceIssueLabels(ctx context.Context, owner, repo string, number int, labels []string, statusLabel string) error {
	next := make([]string, 0, len(labels)+1)
	seen := map[string]struct{}{}
	for _, label := range labels {
		trimmed := strings.TrimSpace(label)
		key := strings.ToLower(trimmed)
		if key == "" || isManagedStatusLabel(key) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		next = append(next, trimmed)
	}
	next = append(next, statusLabel)

	body := map[string]any{"labels": next}
	return c.putJSON(ctx, c.issueLabelsURL(owner, repo, number), body, http.StatusOK)
}

func (c *Client) addIssueComment(ctx context.Context, owner, repo string, number int, comment string) error {
	body := map[string]any{"body": comment}
	return c.postJSON(ctx, c.issueCommentsURL(owner, repo, number), body, http.StatusOK, http.StatusCreated)
}

func (c *Client) fetchIssuesByState(ctx context.Context, owner, repo, state string) ([]domain.Issue, error) {
	page := 1
	var out []domain.Issue
	for {
		var raw []giteaIssue
		if err := c.getJSON(ctx, c.issuesURL(owner, repo, state, page), &raw); err != nil {
			return nil, err
		}
		for _, issue := range raw {
			if issue.PullRequest != nil {
				continue
			}
			mapped, err := c.normalizeIssue(owner, repo, issue)
			if err != nil {
				return nil, err
			}
			out = append(out, mapped)
		}
		if len(raw) < defaultPageSize {
			return out, nil
		}
		page++
	}
}

func (c *Client) issuesURL(owner, repo, state string, page int) string {
	u, _ := url.Parse(c.endpoint)
	u.Path = path.Join(u.Path, "/api/v1/repos", owner, repo, "issues")
	q := u.Query()
	q.Set("state", state)
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(defaultPageSize))
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Client) labelsURL(owner, repo string) string {
	u, _ := url.Parse(c.endpoint)
	u.Path = path.Join(u.Path, "/api/v1/repos", owner, repo, "labels")
	return u.String()
}

func (c *Client) issueLabelsURL(owner, repo string, number int) string {
	u, _ := url.Parse(c.endpoint)
	u.Path = path.Join(u.Path, "/api/v1/repos", owner, repo, "issues", strconv.Itoa(number), "labels")
	return u.String()
}

func (c *Client) issueCommentsURL(owner, repo string, number int) string {
	u, _ := url.Parse(c.endpoint)
	u.Path = path.Join(u.Path, "/api/v1/repos", owner, repo, "issues", strconv.Itoa(number), "comments")
	return u.String()
}

func (c *Client) getJSON(ctx context.Context, targetURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return fmt.Errorf("gitea request error: create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitea request error: send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitea status error: unexpected HTTP status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("gitea payload error: decode JSON: %w", err)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, targetURL string, body any, allowedStatuses ...int) error {
	requestBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("gitea request error: encode JSON: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("gitea request error: create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitea request error: send request: %w", err)
	}
	defer resp.Body.Close()
	for _, status := range allowedStatuses {
		if resp.StatusCode == status {
			return nil
		}
	}
	return fmt.Errorf("gitea status error: unexpected HTTP status %d", resp.StatusCode)
}

func (c *Client) putJSON(ctx context.Context, targetURL string, body any, allowedStatuses ...int) error {
	requestBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("gitea request error: encode JSON: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("gitea request error: create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitea request error: send request: %w", err)
	}
	defer resp.Body.Close()
	for _, status := range allowedStatuses {
		if resp.StatusCode == status {
			return nil
		}
	}
	return fmt.Errorf("gitea status error: unexpected HTTP status %d", resp.StatusCode)
}

// --- Gitea API types ---

type giteaIssue struct {
	Number      int          `json:"number"`
	Title       string       `json:"title"`
	Body        *string      `json:"body"`
	Labels      []giteaLabel `json:"labels"`
	PullRequest any          `json:"pull_request"`
}

type giteaLabel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *Client) normalizeIssue(owner, repo string, raw giteaIssue) (domain.Issue, error) {
	if raw.Number <= 0 {
		return domain.Issue{}, fmt.Errorf("missing number")
	}
	if strings.TrimSpace(raw.Title) == "" {
		return domain.Issue{}, fmt.Errorf("issue %d missing title", raw.Number)
	}
	id := strconv.Itoa(raw.Number)
	return domain.Issue{
		ID:          id,
		Identifier:  owner + "/" + repo + "#" + id,
		Title:       raw.Title,
		Description: raw.Body,
		Labels:      normalizeLabels(raw.Labels),
	}, nil
}

func normalizeLabels(labels []giteaLabel) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		name := strings.ToLower(strings.TrimSpace(label.Name))
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func normalizeStates(states []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, state := range states {
		normalized := strings.ToLower(strings.TrimSpace(state))
		if normalized == "" {
			continue
		}
		if normalized != "open" && normalized != "closed" && normalized != "all" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func giteaStates(states []string) ([]string, error) {
	if len(states) == 0 {
		return []string{"open"}, nil
	}
	normalized := normalizeStates(states)
	if len(normalized) == 0 {
		return nil, fmt.Errorf("gitea active_states must use native issue states open, closed, or all")
	}
	return normalized, nil
}

func isManagedStatusLabel(label string) bool {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "symphony-running", "symphony-done", "symphony-failed":
		return true
	default:
		return false
	}
}

func hasManagedStatusLabel(labels []string) bool {
	for _, label := range labels {
		if isManagedStatusLabel(label) {
			return true
		}
	}
	return false
}
