package model

import "time"

// Issue types.
const (
	IssueEpic    = "epic"
	IssueStory   = "story"
	IssueTask    = "task"
	IssueBug     = "bug"
	IssueSubtask = "subtask"
)

// Workflow status categories. Statuses themselves are user-defined per space;
// the category drives board grouping/coloring and "done" semantics.
const (
	CategoryTodo       = "todo"
	CategoryInProgress = "in_progress"
	CategoryDone       = "done"
)

// WorkflowStatus is a user-defined status within a space's workflow.
type WorkflowStatus struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	SpaceID        string    `json:"space_id"`
	Name           string    `json:"name"`
	Category       string    `json:"category"`
	Color          string    `json:"color"`
	Position       int       `json:"position"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Issue priorities.
const (
	PriorityLowest  = "lowest"
	PriorityLow     = "low"
	PriorityMedium  = "medium"
	PriorityHigh    = "high"
	PriorityHighest = "highest"
)

// Sprint statuses.
const (
	SprintPlanned   = "planned"
	SprintActive    = "active"
	SprintCompleted = "completed"
)

// Sprint is a time-boxed set of issues within a space.
// Nullable fields are emitted as JSON null (never omitted) so the response shape
// is stable: every sprint object always carries the full set of keys.
type Sprint struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	SpaceID        string     `json:"space_id"`
	Name           string     `json:"name"`
	Goal           *string    `json:"goal"`
	Status         string     `json:"status"`
	StartDate      *time.Time `json:"start_date"`
	EndDate        *time.Time `json:"end_date"`
	CompletedAt    *time.Time `json:"completed_at"`
	IssueCount     int        `json:"issue_count"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Space is a top-level work container (a "project"): it has a key (e.g. ENG)
// and holds issues.
type Space struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Key            string    `json:"key"`
	Name           string    `json:"name"`
	Description    *string   `json:"description,omitempty"`
	LeadID         *string   `json:"lead_id,omitempty"`
	CreatedBy      *string   `json:"created_by,omitempty"`
	IssueCount     int       `json:"issue_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Issue is a unit of work within a space. Key is derived as "<spaceKey>-<number>".
type Issue struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	SpaceID        string     `json:"space_id"`
	SpaceKey       string     `json:"space_key"`
	Number         int        `json:"number"`
	Key            string     `json:"key"` // e.g. "ENG-123"
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	Description    *string    `json:"description,omitempty"`
	StatusID       string     `json:"status_id"`
	Status         string     `json:"status"`          // status name, e.g. "To Do"
	StatusCategory string     `json:"status_category"` // todo | in_progress | done
	Priority       string     `json:"priority"`
	AssigneeID     *string    `json:"assignee_id,omitempty"`
	AssigneeName   *string    `json:"assignee_name,omitempty"`
	ReporterID     *string    `json:"reporter_id,omitempty"`
	ReporterName   *string    `json:"reporter_name,omitempty"`
	SprintID       *string    `json:"sprint_id,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
