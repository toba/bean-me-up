// Package clickup provides ClickUp API integration.
package clickup

// TaskInfo holds task data returned from ClickUp.
type TaskInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      Status  `json:"status"`
	URL         string  `json:"url"`
	Parent      *string `json:"parent"` // Parent task ID if subtask
}

// Status represents a ClickUp task status.
type Status struct {
	Status string `json:"status"`
	Color  string `json:"color,omitempty"`
}

// List holds ClickUp list metadata.
type List struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Statuses []Status `json:"statuses"`
}

// CreateTaskRequest is the request body for creating a task.
type CreateTaskRequest struct {
	Name                string        `json:"name"`
	Description         string        `json:"description,omitempty"`
	MarkdownDescription string        `json:"markdown_description,omitempty"`
	Status              string        `json:"status,omitempty"`
	Priority            *int          `json:"priority,omitempty"`
	Assignees           []int         `json:"assignees,omitempty"` // User IDs to assign
	Parent              *string       `json:"parent,omitempty"`    // Parent task ID for subtasks
	CustomFields        []CustomField `json:"custom_fields,omitempty"`
}

// CustomField represents a custom field value for task creation/update.
type CustomField struct {
	ID    string `json:"id"`
	Value any    `json:"value"`
}

// UpdateTaskRequest is the request body for updating a task.
type UpdateTaskRequest struct {
	Name                *string `json:"name,omitempty"`
	Description         *string `json:"description,omitempty"`
	MarkdownDescription *string `json:"markdown_description,omitempty"`
	Status              *string `json:"status,omitempty"`
	Priority            *int    `json:"priority,omitempty"`
	Parent              *string `json:"parent,omitempty"`
}

// Dependency represents a task dependency in ClickUp.
type Dependency struct {
	TaskID      string `json:"task_id"`
	DependsOn   string `json:"depends_on"`
	Type        int    `json:"type"` // 0 = waiting on, 1 = blocking
	DateCreated string `json:"date_created,omitempty"`
	UserID      string `json:"userid,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// AddDependencyRequest is the request body for adding a dependency.
type AddDependencyRequest struct {
	DependsOn string `json:"depends_on"`
}

// taskResponse is the API response wrapper for task operations.
type taskResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      Status  `json:"status"`
	URL         string  `json:"url"`
	Parent      *string `json:"parent"`
}

// listResponse is the API response for getting list details.
type listResponse struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Statuses []Status `json:"statuses"`
}

// errorResponse represents a ClickUp API error.
type errorResponse struct {
	Err   string `json:"err"`
	ECODE string `json:"ECODE"`
}

// FieldInfo represents a custom field available on a list.
type FieldInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	TypeConfig any    `json:"type_config,omitempty"`
	Required   bool   `json:"required,omitempty"`
}

// fieldsResponse is the API response for getting list fields.
type fieldsResponse struct {
	Fields []FieldInfo `json:"fields"`
}

// Member represents a workspace member.
type Member struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// teamMember is used for parsing the team members API response.
type teamMember struct {
	User Member `json:"user"`
}

// teamsResponse is the API response for getting teams.
type teamsResponse struct {
	Teams []teamInfo `json:"teams"`
}

// teamInfo represents a team/workspace.
type teamInfo struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Members []teamMember `json:"members"`
}

// CommentItem represents an item in a structured comment.
type CommentItem struct {
	Text string       `json:"text,omitempty"`
	Type string       `json:"type,omitempty"` // "tag" for mentions
	User *CommentUser `json:"user,omitempty"`
}

// CommentUser represents a user reference in a comment mention.
type CommentUser struct {
	ID int `json:"id"`
}

// createCommentRequest is the request body for creating a task comment.
// For mentions to work, we need to use "comment" as an array, not "comment_text".
type createCommentRequest struct {
	Comment []CommentItem `json:"comment"`
}

// AuthorizedUser represents the authenticated user from the API token.
type AuthorizedUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// userResponse is the API response for getting the authorized user.
type userResponse struct {
	User AuthorizedUser `json:"user"`
}
