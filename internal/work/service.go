// Package work implements spaces (projects) and issues, with audit logging.
package work

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"cairn/internal/model"
	"cairn/internal/store"
)

var (
	ErrSpaceKeyTaken = errors.New("work: space key already in use")
	ErrInvalidKey    = errors.New("work: key must be 2–10 chars, uppercase letters/digits, starting with a letter")
	ErrInvalidIssue  = errors.New("work: invalid issue reference")
	ErrValidation    = errors.New("work: validation failed")
)

var spaceKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

var issueTypes = []string{model.IssueEpic, model.IssueStory, model.IssueTask, model.IssueBug, model.IssueSubtask}
var statusCategories = []string{model.CategoryTodo, model.CategoryInProgress, model.CategoryDone}
var priorities = []string{model.PriorityLowest, model.PriorityLow, model.PriorityMedium, model.PriorityHigh, model.PriorityHighest}

// Service implements space/issue workflows.
type Service struct {
	store *store.DB
}

func NewService(db *store.DB) *Service { return &Service{store: db} }

// ---- Spaces ----

func (s *Service) CreateSpace(ctx context.Context, orgID, actorID, key, name string, description, leadID *string) (*model.Space, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if !spaceKeyRe.MatchString(key) {
		return nil, ErrInvalidKey
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}

	sp, err := s.store.CreateSpace(ctx, orgID, key, strings.TrimSpace(name), description, leadID, actorID)
	if err != nil {
		if errors.Is(err, store.ErrSpaceKeyTaken) {
			return nil, ErrSpaceKeyTaken
		}
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "space.created", "space", sp.ID, map[string]any{"key": sp.Key, "name": sp.Name})
	return sp, nil
}

func (s *Service) ListSpaces(ctx context.Context, orgID string) ([]model.Space, error) {
	return s.store.ListSpaces(ctx, orgID)
}

func (s *Service) GetSpace(ctx context.Context, orgID, key string) (*model.Space, error) {
	return s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(key))
}

func (s *Service) UpdateSpace(ctx context.Context, orgID, actorID, key, name string, description, leadID *string) (*model.Space, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	sp, err := s.store.UpdateSpace(ctx, orgID, strings.ToUpper(key), strings.TrimSpace(name), description, leadID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "space.updated", "space", sp.ID, nil)
	return sp, nil
}

func (s *Service) DeleteSpace(ctx context.Context, orgID, actorID, key string) error {
	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(key))
	if err != nil {
		return err
	}
	if err := s.store.DeleteSpace(ctx, orgID, sp.Key); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "space.deleted", "space", sp.ID, map[string]any{"key": sp.Key})
	return nil
}

// ---- Issues ----

// CreateIssueInput carries new-issue fields.
type CreateIssueInput struct {
	SpaceKey    string
	Type        string
	Title       string
	Description *string
	StatusID    *string // optional; defaults to the space's first workflow status
	Priority    string
	AssigneeID  *string
	DueDate     *string // optional YYYY-MM-DD; nil/"" means no due date
}

func (s *Service) CreateIssue(ctx context.Context, orgID, actorID string, in CreateIssueInput) (*model.Issue, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, fmt.Errorf("%w: title is required", ErrValidation)
	}
	if in.Type == "" {
		in.Type = model.IssueTask
	}
	if !slices.Contains(issueTypes, in.Type) {
		return nil, fmt.Errorf("%w: invalid issue type", ErrValidation)
	}
	if in.Priority == "" {
		in.Priority = model.PriorityMedium
	}
	if !slices.Contains(priorities, in.Priority) {
		return nil, fmt.Errorf("%w: invalid priority", ErrValidation)
	}

	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(in.SpaceKey))
	if err != nil {
		return nil, err
	}

	// New issues start in the space's first workflow status, unless the caller
	// picked a specific status (which must belong to this space).
	var statusID string
	if in.StatusID != nil && strings.TrimSpace(*in.StatusID) != "" {
		ok, err := s.store.StatusInSpace(ctx, *in.StatusID, sp.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: status does not belong to this space", ErrValidation)
		}
		statusID = strings.TrimSpace(*in.StatusID)
	} else {
		statusID, err = s.store.DefaultStatusID(ctx, sp.ID)
		if err != nil {
			return nil, err
		}
	}

	// An empty/blank due date is treated as "none" rather than an invalid date.
	var dueDate *string
	if in.DueDate != nil && strings.TrimSpace(*in.DueDate) != "" {
		d := strings.TrimSpace(*in.DueDate)
		dueDate = &d
	}

	issue, err := s.store.CreateIssue(ctx, orgID, sp.ID, statusID, in.Type, strings.TrimSpace(in.Title), in.Description, in.AssigneeID, in.Priority, actorID, dueDate)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "issue.created", "issue", issue.ID, map[string]any{"key": issue.Key, "title": issue.Title})
	return issue, nil
}

func (s *Service) ListIssues(ctx context.Context, orgID string, f store.IssueFilter) ([]model.Issue, error) {
	return s.store.ListIssues(ctx, orgID, f)
}

func (s *Service) GetIssue(ctx context.Context, orgID, issueKey string) (*model.Issue, error) {
	spaceKey, number, err := parseIssueKey(issueKey)
	if err != nil {
		return nil, err
	}
	return s.store.GetIssueByKey(ctx, orgID, spaceKey, number)
}

func (s *Service) UpdateIssue(ctx context.Context, orgID, actorID, issueKey string, u store.IssueUpdate) (*model.Issue, error) {
	if u.Type != nil && !slices.Contains(issueTypes, *u.Type) {
		return nil, fmt.Errorf("%w: invalid issue type", ErrValidation)
	}
	if u.Priority != nil && !slices.Contains(priorities, *u.Priority) {
		return nil, fmt.Errorf("%w: invalid priority", ErrValidation)
	}
	if u.Title != nil && strings.TrimSpace(*u.Title) == "" {
		return nil, fmt.Errorf("%w: title cannot be empty", ErrValidation)
	}

	existing, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return nil, err
	}
	if u.StatusID != nil {
		ok, err := s.store.StatusInSpace(ctx, *u.StatusID, existing.SpaceID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: status does not belong to this space", ErrValidation)
		}
		// Enforce the space's workflow: the transition from the issue's current
		// status to the new one must be permitted (open workflows allow all).
		allowed, err := s.store.TransitionAllowed(ctx, existing.SpaceID, existing.StatusID, *u.StatusID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrInvalidIssueTransition
		}
	}
	updated, err := s.store.UpdateIssue(ctx, orgID, existing.ID, u)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "issue.updated", "issue", updated.ID, nil)
	return updated, nil
}

func (s *Service) DeleteIssue(ctx context.Context, orgID, actorID, issueKey string) error {
	existing, err := s.GetIssue(ctx, orgID, issueKey)
	if err != nil {
		return err
	}
	if err := s.store.DeleteIssue(ctx, orgID, existing.ID); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "issue.deleted", "issue", existing.ID, map[string]any{"key": existing.Key})
	return nil
}

// parseIssueKey splits "ENG-123" into ("ENG", 123).
func parseIssueKey(key string) (string, int, error) {
	idx := strings.LastIndex(key, "-")
	if idx <= 0 || idx == len(key)-1 {
		return "", 0, ErrInvalidIssue
	}
	spaceKey := strings.ToUpper(key[:idx])
	number, err := strconv.Atoi(key[idx+1:])
	if err != nil || number <= 0 {
		return "", 0, ErrInvalidIssue
	}
	return spaceKey, number, nil
}

// audit writes an audit event best-effort (logs, never blocks the operation).
func (s *Service) audit(ctx context.Context, orgID, actorID, action, entityType, entityID string, metadata map[string]any) {
	if err := s.store.RecordAudit(ctx, orgID, actorID, action, entityType, entityID, metadata); err != nil {
		slog.Error("audit record failed", "action", action, "entity_id", entityID, "error", err)
	}
}
