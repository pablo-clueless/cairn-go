package work

import (
	"context"
	"sort"
	"time"

	"cairn/internal/model"
	"cairn/internal/store"
)

const dayFmt = "2006-01-02"

// Velocity returns completed-sprint throughput for a space.
func (s *Service) Velocity(ctx context.Context, orgID, spaceKey string) ([]model.VelocityPoint, error) {
	space, err := s.GetSpace(ctx, orgID, spaceKey)
	if err != nil {
		return nil, err
	}
	pts, err := s.store.VelocityBySpace(ctx, space.ID)
	if err != nil {
		return nil, err
	}
	if pts == nil {
		pts = []model.VelocityPoint{}
	}
	return pts, nil
}

// Burndown reconstructs a sprint's day-by-day remaining (not-done) issue count
// from status history, with an ideal straight line for reference.
func (s *Service) Burndown(ctx context.Context, orgID, spaceKey, sprintID string) ([]model.BurndownPoint, error) {
	if _, err := s.GetSpace(ctx, orgID, spaceKey); err != nil {
		return nil, err
	}
	sprint, err := s.store.GetSprintByID(ctx, orgID, sprintID)
	if err != nil {
		return nil, err
	}
	changes, err := s.store.SprintStatusHistory(ctx, sprintID)
	if err != nil {
		return nil, err
	}

	// The sprint window: explicit dates if set, else inferred from history.
	start, end := sprintWindow(sprint, changes)
	if start.IsZero() {
		return []model.BurndownPoint{}, nil
	}
	days := daysBetween(start, end)
	timelines := groupTimelines(changes)
	totalIssues := len(timelines)

	out := make([]model.BurndownPoint, 0, len(days))
	for idx, day := range days {
		remaining := 0
		for _, tl := range timelines {
			if cat := categoryAsOf(tl, endOfDay(day)); cat != "" && cat != model.CategoryDone {
				remaining++
			}
		}
		ideal := 0.0
		if n := len(days) - 1; n > 0 {
			ideal = float64(totalIssues) * float64(n-idx) / float64(n)
		}
		out = append(out, model.BurndownPoint{Date: day.Format(dayFmt), Remaining: remaining, Ideal: ideal})
	}
	return out, nil
}

// CFD reconstructs cumulative-flow (per-category counts) for a space over the
// last `days` days from status history.
func (s *Service) CFD(ctx context.Context, orgID, spaceKey string, days int) ([]model.CFDPoint, error) {
	space, err := s.GetSpace(ctx, orgID, spaceKey)
	if err != nil {
		return nil, err
	}
	if days <= 0 || days > 180 {
		days = 30
	}
	changes, err := s.store.SpaceStatusHistory(ctx, space.ID)
	if err != nil {
		return nil, err
	}
	timelines := groupTimelines(changes)

	today := time.Now()
	window := daysBetween(today.AddDate(0, 0, -(days - 1)), today)
	out := make([]model.CFDPoint, 0, len(window))
	for _, day := range window {
		pt := model.CFDPoint{Date: day.Format(dayFmt)}
		eod := endOfDay(day)
		for _, tl := range timelines {
			switch categoryAsOf(tl, eod) {
			case model.CategoryTodo:
				pt.Todo++
			case model.CategoryInProgress:
				pt.InProgress++
			case model.CategoryDone:
				pt.Done++
			}
		}
		out = append(out, pt)
	}
	return out, nil
}

// groupTimelines buckets status changes by issue, each sorted oldest-first.
func groupTimelines(changes []store.StatusChange) map[string][]store.StatusChange {
	byIssue := map[string][]store.StatusChange{}
	for _, c := range changes {
		byIssue[c.IssueID] = append(byIssue[c.IssueID], c)
	}
	for _, tl := range byIssue {
		sort.Slice(tl, func(i, j int) bool { return tl[i].ChangedAt.Before(tl[j].ChangedAt) })
	}
	return byIssue
}

// categoryAsOf returns an issue's category at time t (the latest change ≤ t), or
// "" if the issue did not yet exist.
func categoryAsOf(timeline []store.StatusChange, t time.Time) string {
	cat := ""
	for _, c := range timeline {
		if c.ChangedAt.After(t) {
			break
		}
		cat = c.Category
	}
	return cat
}

// sprintWindow returns the [start, end] dates for a sprint, falling back to the
// span of its status history when explicit dates are missing.
func sprintWindow(sprint *model.Sprint, changes []store.StatusChange) (time.Time, time.Time) {
	var start, end time.Time
	if sprint.StartDate != nil {
		start = *sprint.StartDate
	}
	if sprint.EndDate != nil {
		end = *sprint.EndDate
	}
	if start.IsZero() && len(changes) > 0 {
		start = changes[0].ChangedAt
	}
	now := time.Now()
	if end.IsZero() || end.After(now) {
		end = now
	}
	if !start.IsZero() && end.Before(start) {
		end = start
	}
	return truncDay(start), truncDay(end)
}

// daysBetween returns each calendar day in [start, end] inclusive.
func daysBetween(start, end time.Time) []time.Time {
	start, end = truncDay(start), truncDay(end)
	var days []time.Time
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		days = append(days, d)
	}
	return days
}

func truncDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return truncDay(t).Add(24*time.Hour - time.Nanosecond)
}
