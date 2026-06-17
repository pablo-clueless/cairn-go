package work

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"cairn/internal/model"
)

var (
	ErrActiveSprintExists = errors.New("work: this space already has an active sprint")
	ErrInvalidTransition  = errors.New("work: invalid sprint status transition")
)

var sprintStatuses = []string{model.SprintPlanned, model.SprintActive, model.SprintCompleted}

// CreateSprint creates a planned sprint in a space.
func (s *Service) CreateSprint(ctx context.Context, orgID, actorID, spaceKey, name string, goal *string, startDate, endDate *time.Time) (*model.Sprint, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: sprint name is required", ErrValidation)
	}
	space, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}
	sprint, err := s.store.CreateSprint(ctx, orgID, space.ID, strings.TrimSpace(name), goal, startDate, endDate)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "sprint.created", "sprint", sprint.ID, map[string]any{"name": sprint.Name})
	return sprint, nil
}

func (s *Service) ListSprints(ctx context.Context, orgID, spaceKey string) ([]model.Sprint, error) {
	space, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}
	return s.store.ListSprintsBySpace(ctx, orgID, space.ID)
}

func (s *Service) GetSprint(ctx context.Context, orgID, sprintID string) (*model.Sprint, error) {
	return s.store.GetSprintByID(ctx, orgID, sprintID)
}

// SprintUpdate carries optional sprint changes (nil = unchanged).
type SprintUpdate struct {
	Name      *string
	Goal      *string
	Status    *string
	StartDate *time.Time
	EndDate   *time.Time
}

// UpdateSprint applies field edits and validated status transitions
// (planned→active, active→completed). Completing a sprint moves incomplete
// issues back to the backlog.
func (s *Service) UpdateSprint(ctx context.Context, orgID, actorID, sprintID string, u SprintUpdate) (*model.Sprint, error) {
	sprint, err := s.store.GetSprintByID(ctx, orgID, sprintID)
	if err != nil {
		return nil, err
	}

	if u.Name != nil {
		if strings.TrimSpace(*u.Name) == "" {
			return nil, fmt.Errorf("%w: sprint name is required", ErrValidation)
		}
		sprint.Name = strings.TrimSpace(*u.Name)
	}
	if u.Goal != nil {
		sprint.Goal = u.Goal
	}
	if u.StartDate != nil {
		sprint.StartDate = u.StartDate
	}
	if u.EndDate != nil {
		sprint.EndDate = u.EndDate
	}

	action := "sprint.updated"
	completing := false

	if u.Status != nil && *u.Status != sprint.Status {
		if !slices.Contains(sprintStatuses, *u.Status) {
			return nil, fmt.Errorf("%w: unknown status", ErrValidation)
		}
		switch {
		case sprint.Status == model.SprintPlanned && *u.Status == model.SprintActive:
			active, err := s.store.CountActiveSprints(ctx, sprint.SpaceID)
			if err != nil {
				return nil, err
			}
			if active > 0 {
				return nil, ErrActiveSprintExists
			}
			sprint.Status = model.SprintActive
			if sprint.StartDate == nil {
				now := time.Now()
				sprint.StartDate = &now
			}
			action = "sprint.started"
		case sprint.Status == model.SprintActive && *u.Status == model.SprintCompleted:
			now := time.Now()
			sprint.Status = model.SprintCompleted
			sprint.CompletedAt = &now
			completing = true
			action = "sprint.completed"
		default:
			return nil, ErrInvalidTransition
		}
	}

	updated, err := s.store.UpdateSprint(ctx, sprint)
	if err != nil {
		return nil, err
	}
	if completing {
		if err := s.store.MoveIncompleteIssuesToBacklog(ctx, updated.ID); err != nil {
			return nil, err
		}
	}
	s.audit(ctx, orgID, actorID, action, "sprint", updated.ID, nil)
	return updated, nil
}

func (s *Service) DeleteSprint(ctx context.Context, orgID, actorID, sprintID string) error {
	if err := s.store.DeleteSprint(ctx, orgID, sprintID); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "sprint.deleted", "sprint", sprintID, nil)
	return nil
}
