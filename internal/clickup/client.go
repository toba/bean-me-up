package clickup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

const baseURL = "https://api.clickup.com/api/v2"

// Default retry configuration for rate limit handling
const (
	defaultMaxRetries     = 5
	defaultBaseRetryDelay = 1 * time.Second
	defaultMaxRetryDelay  = 30 * time.Second
)

// RateLimitError represents a ClickUp rate limit error.
type RateLimitError struct {
	Message string
	Code    string
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit: %s (code: %s)", e.Message, e.Code)
}

// TransientError represents a transient error that can be retried.
type TransientError struct {
	Message string
}

func (e *TransientError) Error() string {
	return fmt.Sprintf("transient error: %s", e.Message)
}

// RetryConfig holds retry settings for rate limit handling.
type RetryConfig struct {
	MaxRetries     int
	BaseRetryDelay time.Duration
	MaxRetryDelay  time.Duration
}

// Client provides ClickUp API access via REST.
type Client struct {
	token      string
	httpClient *http.Client

	// Retry configuration (uses defaults if nil)
	retryConfig *RetryConfig

	// Cached list info
	listInfo *List
	// Cached authorized user
	authorizedUser *AuthorizedUser
}

func (c *Client) getRetryConfig() RetryConfig {
	if c.retryConfig != nil {
		return *c.retryConfig
	}
	return RetryConfig{
		MaxRetries:     defaultMaxRetries,
		BaseRetryDelay: defaultBaseRetryDelay,
		MaxRetryDelay:  defaultMaxRetryDelay,
	}
}

// NewClient creates a new ClickUp client.
// The token should be a ClickUp API token.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{},
	}
}

// GetList fetches list metadata including available statuses.
func (c *Client) GetList(ctx context.Context, listID string) (*List, error) {
	if c.listInfo != nil && c.listInfo.ID == listID {
		return c.listInfo, nil
	}

	url := fmt.Sprintf("%s/list/%s", baseURL, listID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var resp listResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, fmt.Errorf("getting list: %w", err)
	}

	c.listInfo = &List{
		ID:       resp.ID,
		Name:     resp.Name,
		Statuses: resp.Statuses,
	}

	return c.listInfo, nil
}

// GetTask fetches a task by ID.
func (c *Client) GetTask(ctx context.Context, taskID string) (*TaskInfo, error) {
	url := fmt.Sprintf("%s/task/%s", baseURL, taskID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var resp taskResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, fmt.Errorf("getting task: %w", err)
	}

	return resp.toTaskInfo(), nil
}

// CreateTask creates a new task in the given list.
func (c *Client) CreateTask(ctx context.Context, listID string, task *CreateTaskRequest) (*TaskInfo, error) {
	url := fmt.Sprintf("%s/list/%s/task", baseURL, listID)

	body, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var resp taskResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, fmt.Errorf("creating task: %w", err)
	}

	return resp.toTaskInfo(), nil
}

// UpdateTask updates an existing task.
func (c *Client) UpdateTask(ctx context.Context, taskID string, update *UpdateTaskRequest) (*TaskInfo, error) {
	url := fmt.Sprintf("%s/task/%s", baseURL, taskID)

	body, err := json.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	var resp taskResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, fmt.Errorf("updating task: %w", err)
	}

	return resp.toTaskInfo(), nil
}

// AddDependency adds a dependency to a task.
// This sets the task with taskID as waiting on (depends on) the task with dependsOnID.
// In other words: dependsOnID is blocking taskID.
func (c *Client) AddDependency(ctx context.Context, taskID, dependsOnID string) error {
	url := fmt.Sprintf("%s/task/%s/dependency", baseURL, taskID)

	body, err := json.Marshal(&AddDependencyRequest{
		DependsOn: dependsOnID,
	})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := c.doRequest(req, nil); err != nil {
		return fmt.Errorf("adding dependency: %w", err)
	}

	return nil
}

// GetAuthorizedUser fetches the user associated with the API token.
// Results are cached for the lifetime of the client.
func (c *Client) GetAuthorizedUser(ctx context.Context) (*AuthorizedUser, error) {
	if c.authorizedUser != nil {
		return c.authorizedUser, nil
	}

	url := fmt.Sprintf("%s/user", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var resp userResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, fmt.Errorf("getting authorized user: %w", err)
	}

	c.authorizedUser = &resp.User
	return c.authorizedUser, nil
}

// GetAccessibleCustomFields fetches available custom fields for a list.
func (c *Client) GetAccessibleCustomFields(ctx context.Context, listID string) ([]FieldInfo, error) {
	url := fmt.Sprintf("%s/list/%s/field", baseURL, listID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var resp fieldsResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, fmt.Errorf("getting custom fields: %w", err)
	}

	return resp.Fields, nil
}

// GetWorkspaceMembers fetches members from all accessible workspaces.
func (c *Client) GetWorkspaceMembers(ctx context.Context) ([]Member, error) {
	url := fmt.Sprintf("%s/team", baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	var resp teamsResponse
	if err := c.doRequest(req, &resp); err != nil {
		return nil, fmt.Errorf("getting teams: %w", err)
	}

	// Collect unique members from all teams
	seen := make(map[int]bool)
	var members []Member
	for _, team := range resp.Teams {
		for _, tm := range team.Members {
			if !seen[tm.User.ID] {
				seen[tm.User.ID] = true
				members = append(members, tm.User)
			}
		}
	}

	return members, nil
}

// GetCustomItems fetches custom task types from all accessible workspaces.
// Returns custom items with their IDs, names, and descriptions.
func (c *Client) GetCustomItems(ctx context.Context) ([]CustomItem, error) {
	// First get all teams to iterate through workspaces
	teamsURL := fmt.Sprintf("%s/team", baseURL)
	teamsReq, err := http.NewRequestWithContext(ctx, "GET", teamsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating teams request: %w", err)
	}

	var teamsResp teamsResponse
	if err := c.doRequest(teamsReq, &teamsResp); err != nil {
		return nil, fmt.Errorf("getting teams: %w", err)
	}

	// Collect custom items from all teams
	seen := make(map[int]bool)
	var items []CustomItem
	for _, team := range teamsResp.Teams {
		url := fmt.Sprintf("%s/team/%s/custom_item", baseURL, team.ID)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		var resp customItemsResponse
		if err := c.doRequest(req, &resp); err != nil {
			// Skip teams that don't support custom items
			continue
		}

		for _, item := range resp.CustomItems {
			if !seen[item.ID] {
				seen[item.ID] = true
				items = append(items, item)
			}
		}
	}

	return items, nil
}

// CreateTaskComment creates a comment on a task with structured content (supports mentions).
func (c *Client) CreateTaskComment(ctx context.Context, taskID string, commentItems []CommentItem) error {
	url := fmt.Sprintf("%s/task/%s/comment", baseURL, taskID)

	body, err := json.Marshal(&createCommentRequest{
		Comment: commentItems,
	})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := c.doRequest(req, nil); err != nil {
		return fmt.Errorf("creating comment: %w", err)
	}

	return nil
}

// SetCustomFieldValue sets a custom field value on a task.
func (c *Client) SetCustomFieldValue(ctx context.Context, taskID, fieldID string, value any) error {
	url := fmt.Sprintf("%s/task/%s/field/%s", baseURL, taskID, fieldID)

	body, err := json.Marshal(map[string]any{"value": value})
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if err := c.doRequest(req, nil); err != nil {
		return fmt.Errorf("setting custom field: %w", err)
	}

	return nil
}

// doRequest executes an HTTP request and decodes the response.
// It automatically retries on rate limit errors and transient errors with exponential backoff.
func (c *Client) doRequest(req *http.Request, result any) error {
	cfg := c.getRetryConfig()

	// We need to be able to retry the request, so we need to save the body
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("reading request body: %w", err)
		}
		_ = req.Body.Close()
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff and jitter
			delay := min(cfg.BaseRetryDelay*time.Duration(1<<(attempt-1)), cfg.MaxRetryDelay)
			// Add jitter (0-25% of delay)
			jitter := time.Duration(rand.Int64N(int64(delay / 4)))
			delay += jitter

			select {
			case <-req.Context().Done():
				return req.Context().Err()
			case <-time.After(delay):
			}

			// Reset the body for retry
			if bodyBytes != nil {
				req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
		}

		req.Header.Set("Authorization", c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			// Check for transient network errors (stream errors, connection resets, etc.)
			if isTransientNetworkError(err) {
				lastErr = &TransientError{Message: err.Error()}
				continue // Retry
			}
			return fmt.Errorf("executing request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode >= 400 {
			// Check for rate limit errors
			var errResp errorResponse
			if err := json.Unmarshal(body, &errResp); err == nil && errResp.Err != "" {
				if resp.StatusCode == 429 || errResp.ECODE == "APP_002" {
					lastErr = &RateLimitError{Message: errResp.Err, Code: errResp.ECODE}
					continue // Retry
				}
				return fmt.Errorf("API error: %s (code: %s)", errResp.Err, errResp.ECODE)
			}

			// Check for transient HTTP errors (5xx, CloudFront errors, etc.)
			if isTransientHTTPError(resp.StatusCode, body) {
				lastErr = &TransientError{Message: fmt.Sprintf("HTTP %d", resp.StatusCode)}
				continue // Retry
			}

			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		if result != nil && len(body) > 0 {
			if err := json.Unmarshal(body, result); err != nil {
				return fmt.Errorf("decoding response: %w", err)
			}
		}

		return nil
	}

	// All retries exhausted
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// isTransientNetworkError checks if an error is a transient network error that should be retried.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// HTTP/2 stream errors
	if strings.Contains(errStr, "stream error") || strings.Contains(errStr, "INTERNAL_ERROR") {
		return true
	}
	// Connection reset/refused
	if strings.Contains(errStr, "connection reset") || strings.Contains(errStr, "connection refused") {
		return true
	}
	// EOF errors (connection closed unexpectedly)
	if strings.Contains(errStr, "EOF") {
		return true
	}
	// Timeout errors
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "Timeout") {
		return true
	}
	return false
}

// isTransientHTTPError checks if an HTTP error is transient and should be retried.
func isTransientHTTPError(statusCode int, body []byte) bool {
	// 5xx server errors are always transient
	if statusCode >= 500 && statusCode < 600 {
		return true
	}
	// Some 4xx errors from CDN/infrastructure are transient
	if statusCode == 400 || statusCode == 502 || statusCode == 503 || statusCode == 504 {
		bodyStr := string(body)
		// CloudFront errors
		if strings.Contains(bodyStr, "CloudFront") || strings.Contains(bodyStr, "cloudfront") {
			return true
		}
		// Generic "try again later" messages
		if strings.Contains(bodyStr, "try again") || strings.Contains(bodyStr, "Try again") {
			return true
		}
	}
	return false
}
