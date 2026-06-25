package model

import "time"

// Notification types.
const (
	NotificationMention  = "mention"
	NotificationComment  = "comment"
	NotificationAssigned = "assigned"
	NotificationActivity = "activity"
)

// Notification is one entry in a user's personal inbox. OrgSlug and IssueKey are
// resolved for building a deep link without extra round-trips.
type Notification struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	ActorID    *string    `json:"actor_id"`
	ActorName  *string    `json:"actor_name"`
	OrgSlug    string     `json:"org_slug"`
	IssueID    *string    `json:"issue_id"`
	IssueKey   string     `json:"issue_key"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// NotificationPreferences holds a user's email opt-ins. Defaults are all-on.
type NotificationPreferences struct {
	EmailMentions    bool `json:"email_mentions"`
	EmailComments    bool `json:"email_comments"`
	EmailAssignments bool `json:"email_assignments"`
}
