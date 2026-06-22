package work

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"cairn/internal/model"
	"cairn/internal/store"
)

func (s *Service) ListStatuses(ctx context.Context, orgID, spaceKey string) ([]model.WorkflowStatus, error) {
	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}
	return s.store.ListStatuses(ctx, orgID, sp.ID)
}

// CreateStatus appends a workflow status to the end of a space's board.
func (s *Service) CreateStatus(ctx context.Context, orgID, actorID, spaceKey, name, category, color string) (*model.WorkflowStatus, error) {
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
	status, err := s.store.CreateStatus(ctx, orgID, sp.ID, strings.TrimSpace(name), category, strings.TrimSpace(color), maxPos+1)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "status.created", "status", status.ID, map[string]any{"name": status.Name})
	return status, nil
}

// UpdateStatus applies partial changes to a workflow status (rename, recategorize, recolor, reorder).
func (s *Service) UpdateStatus(ctx context.Context, orgID, actorID, statusID string, name, category, color *string, position, wipLimit *int) (*model.WorkflowStatus, error) {
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
	color2 := cur.Color
	if color != nil {
		color2 = strings.TrimSpace(*color)
	}
	position2 := cur.Position
	if position != nil {
		position2 = *position
	}
	wip2 := cur.WIPLimit
	if wipLimit != nil {
		if *wipLimit < 0 {
			return nil, fmt.Errorf("%w: WIP limit cannot be negative", ErrValidation)
		}
		wip2 = *wipLimit
	}

	status, err := s.store.UpdateStatus(ctx, orgID, statusID, name2, category2, color2, position2, wip2)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "status.updated", "status", status.ID, nil)
	return status, nil
}

// StatusPatchInput is a partial change to one status in a bulk update.
type StatusPatchInput struct {
	ID       string
	Name     *string
	Category *string
	Color    *string
	Position *int
	WIPLimit *int
}

// BulkUpdateStatuses applies changes to multiple statuses of a space at once
// (e.g. reordering columns, where two or more positions change together).
func (s *Service) BulkUpdateStatuses(ctx context.Context, orgID, actorID, spaceKey string, patches []StatusPatchInput) ([]model.WorkflowStatus, error) {
	if len(patches) == 0 {
		return nil, fmt.Errorf("%w: no statuses provided", ErrValidation)
	}

	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}

	out := make([]store.StatusPatch, 0, len(patches))
	for _, p := range patches {
		if strings.TrimSpace(p.ID) == "" {
			return nil, fmt.Errorf("%w: status id is required", ErrValidation)
		}
		patch := store.StatusPatch{ID: p.ID, Position: p.Position, WIPLimit: p.WIPLimit}
		if p.Name != nil {
			n := strings.TrimSpace(*p.Name)
			if n == "" {
				return nil, fmt.Errorf("%w: status name is required", ErrValidation)
			}
			patch.Name = &n
		}
		if p.Category != nil {
			if !slices.Contains(statusCategories, *p.Category) {
				return nil, fmt.Errorf("%w: invalid status category", ErrValidation)
			}
			patch.Category = p.Category
		}
		if p.Color != nil {
			c := strings.TrimSpace(*p.Color)
			patch.Color = &c
		}
		out = append(out, patch)
	}

	statuses, err := s.store.BulkUpdateStatuses(ctx, orgID, sp.ID, out)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "status.reordered", "space", sp.ID, map[string]any{"count": len(out)})
	return statuses, nil
}

func (s *Service) DeleteStatus(ctx context.Context, orgID, actorID, statusID string) error {
	if err := s.store.DeleteStatus(ctx, orgID, statusID); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "status.deleted", "status", statusID, nil)
	return nil
}
