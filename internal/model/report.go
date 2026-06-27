package model

import "time"

// VelocityPoint is one completed sprint's throughput.
type VelocityPoint struct {
	SprintID    string     `json:"sprint_id"`
	SprintName  string     `json:"sprint_name"`
	CompletedAt *time.Time `json:"completed_at"`
	Completed   int        `json:"completed"` // issues that ended done
	Total       int        `json:"total"`     // issues in the sprint
}

// BurndownPoint is the remaining (not-done) issue count on a given day of a
// sprint, alongside the ideal straight-line burndown for reference.
type BurndownPoint struct {
	Date      string  `json:"date"` // YYYY-MM-DD
	Remaining int     `json:"remaining"`
	Ideal     float64 `json:"ideal"`
}

// CFDPoint is the per-category issue count on a given day (cumulative flow).
type CFDPoint struct {
	Date       string `json:"date"` // YYYY-MM-DD
	Todo       int    `json:"todo"`
	InProgress int    `json:"in_progress"`
	Done       int    `json:"done"`
}
