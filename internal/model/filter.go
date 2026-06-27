package model

import (
	"encoding/json"
	"time"
)

// SavedFilter is a user's named, reusable issue filter. Criteria is an opaque
// JSON blob the frontend writes and applies.
type SavedFilter struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Criteria  json.RawMessage `json:"criteria"`
	IsStarred bool            `json:"is_starred"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
