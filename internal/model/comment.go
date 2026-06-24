package model

import "time"

// Comment is a message posted on an issue. AuthorName is denormalized from the
// users table for display; AuthorID is null if the author was deleted.
type Comment struct {
	ID         string    `json:"id"`
	IssueID    string    `json:"issue_id"`
	AuthorID   *string   `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
