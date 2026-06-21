package work

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cairn/internal/model"
	"cairn/internal/store"
)

// ErrInvalidIssueTransition is returned when an issue status change is not
// permitted by the space's configured workflow.
var ErrInvalidIssueTransition = errors.New("work: status change not allowed by the workflow")

// ListTransitions returns a space's configured status transitions.
func (s *Service) ListTransitions(ctx context.Context, orgID, spaceKey string) ([]model.StatusTransition, error) {
	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}
	return s.store.ListTransitions(ctx, orgID, sp.ID)
}

// TransitionInput is one allowed edge in a workflow. From nil (or empty) means
// "from any status".
type TransitionInput struct {
	From *string
	To   string
}

// SetTransitions replaces a space's entire workflow transition set. Passing an
// empty list clears the workflow back to "open" (any status to any other).
func (s *Service) SetTransitions(ctx context.Context, orgID, actorID, spaceKey string, in []TransitionInput) ([]model.StatusTransition, error) {
	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}

	pairs := make([]store.TransitionPair, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, t := range in {
		to := strings.TrimSpace(t.To)
		if to == "" {
			return nil, fmt.Errorf("%w: to_status_id is required", ErrValidation)
		}
		// Treat a nil or empty from as the global "any status" source.
		var from *string
		if t.From != nil && strings.TrimSpace(*t.From) != "" {
			f := strings.TrimSpace(*t.From)
			from = &f
		}
		fromKey := ""
		if from != nil {
			fromKey = *from
		}
		if key := fromKey + "->" + to; seen[key] {
			continue
		} else {
			seen[key] = true
		}
		pairs = append(pairs, store.TransitionPair{From: from, To: to})
	}

	out, err := s.store.SetTransitions(ctx, orgID, sp.ID, pairs)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "transitions.updated", "space", sp.ID, map[string]any{"count": len(pairs)})
	return out, nil
}
