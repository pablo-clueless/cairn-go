package work

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"cairn/internal/model"
)

func (s *Service) ListStatuses(ctx context.Context, orgID, spaceKey string) ([]model.WorkflowStatus, error) {
	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}
	return s.store.ListStatuses(ctx, orgID, sp.ID)
}

// CreateStatus appends a workflow status to the end of a space's board.
func (s *Service) CreateStatus(ctx context.Context, orgID, actorID, spaceKey, name, category string) (*model.WorkflowStatus, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: status name is required", ErrValidation)
	}
	if category == "" {
		category = model.CategoryTodo
	}
	if !slices.Contains(statusCategories, category) {
		return nil, fmt.Errorf("%w: invalid status category", ErrValidation)
	}

	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}
	maxPos, err := s.store.MaxStatusPosition(ctx, sp.ID)
	if err != nil {
		return nil, err
	}
	status, err := s.store.CreateStatus(ctx, orgID, sp.ID, strings.TrimSpace(name), category, maxPos+1)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "status.created", "status", status.ID, map[string]any{"name": status.Name})
	return status, nil
}

// UpdateStatus applies partial changes to a workflow status (rename, recategorize, reorder).
func (s *Service) UpdateStatus(ctx context.Context, orgID, actorID, statusID string, name, category *string, position *int) (*model.WorkflowStatus, error) {
	cur, err := s.store.GetStatus(ctx, orgID, statusID)
	if err != nil {
		return nil, err
	}

	name2 := cur.Name
	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return nil, fmt.Errorf("%w: status name is required", ErrValidation)
		}
		name2 = strings.TrimSpace(*name)
	}
	category2 := cur.Category
	if category != nil {
		if !slices.Contains(statusCategories, *category) {
			return nil, fmt.Errorf("%w: invalid status category", ErrValidation)
		}
		category2 = *category
	}
	position2 := cur.Position
	if position != nil {
		position2 = *position
	}

	status, err := s.store.UpdateStatus(ctx, orgID, statusID, name2, category2, position2)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "status.updated", "status", status.ID, nil)
	return status, nil
}

func (s *Service) DeleteStatus(ctx context.Context, orgID, actorID, statusID string) error {
	if err := s.store.DeleteStatus(ctx, orgID, statusID); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "status.deleted", "status", statusID, nil)
	return nil
}
