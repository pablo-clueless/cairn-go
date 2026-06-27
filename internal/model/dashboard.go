package model

import (
	"encoding/json"
	"time"
)

// Dashboard is a user's named collection of widgets. Widgets is an opaque JSON
// array the frontend writes and renders.
type Dashboard struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Widgets   json.RawMessage `json:"widgets"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
