package work

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"cairn/internal/model"
	"cairn/internal/store"
)

var documentTypes = []string{model.DocumentPage, model.DocumentLive, model.DocumentWhiteboard}
var documentStatuses = []string{model.DocumentDraft, model.DocumentPublished}

// CreateDocumentInput carries new-document fields.
type CreateDocumentInput struct {
	SpaceKey string
	Title    string
	Type     string
	Status   string  // optional; live docs default to published, pages to draft
	Content  string
	ParentID *string // optional; must belong to the same space
}

func (s *Service) CreateDocument(ctx context.Context, orgID, actorID string, in CreateDocumentInput) (*model.Document, error) {
	if in.Type == "" {
		in.Type = model.DocumentPage
	}
	if !slices.Contains(documentTypes, in.Type) {
		return nil, fmt.Errorf("%w: invalid document type", ErrValidation)
	}

	// Live docs are "always live" (published on create); pages start as drafts.
	if in.Status == "" {
		if in.Type == model.DocumentLive {
			in.Status = model.DocumentPublished
		} else {
			in.Status = model.DocumentDraft
		}
	}
	if !slices.Contains(documentStatuses, in.Status) {
		return nil, fmt.Errorf("%w: invalid document status", ErrValidation)
	}

	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(in.SpaceKey))
	if err != nil {
		return nil, err
	}

	var parentID *string
	if in.ParentID != nil && strings.TrimSpace(*in.ParentID) != "" {
		pid := strings.TrimSpace(*in.ParentID)
		ok, err := s.store.DocumentInSpace(ctx, pid, sp.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: parent document does not belong to this space", ErrValidation)
		}
		parentID = &pid
	}

	doc, err := s.store.CreateDocument(ctx, orgID, sp.ID, parentID, strings.TrimSpace(in.Title), in.Type, in.Status, in.Content, actorID)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "document.created", "document", doc.ID, map[string]any{"title": doc.Title, "type": doc.Type})
	return doc, nil
}

func (s *Service) ListDocuments(ctx context.Context, orgID, spaceKey string) ([]model.Document, error) {
	sp, err := s.store.GetSpaceByKey(ctx, orgID, strings.ToUpper(spaceKey))
	if err != nil {
		return nil, err
	}
	return s.store.ListDocumentsBySpace(ctx, orgID, sp.ID)
}

func (s *Service) GetDocument(ctx context.Context, orgID, id string) (*model.Document, error) {
	return s.store.GetDocumentByID(ctx, orgID, id)
}

func (s *Service) UpdateDocument(ctx context.Context, orgID, actorID, id string, u store.DocumentUpdate) (*model.Document, error) {
	if u.Status != nil && !slices.Contains(documentStatuses, *u.Status) {
		return nil, fmt.Errorf("%w: invalid document status", ErrValidation)
	}

	existing, err := s.store.GetDocumentByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	// A reparent must stay within the same space and not point at itself.
	if u.ParentID != nil && strings.TrimSpace(*u.ParentID) != "" {
		pid := strings.TrimSpace(*u.ParentID)
		if pid == id {
			return nil, fmt.Errorf("%w: a document cannot be its own parent", ErrValidation)
		}
		ok, err := s.store.DocumentInSpace(ctx, pid, existing.SpaceID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%w: parent document does not belong to this space", ErrValidation)
		}
	}

	doc, err := s.store.UpdateDocument(ctx, orgID, id, u)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, orgID, actorID, "document.updated", "document", doc.ID, nil)
	return doc, nil
}

func (s *Service) DeleteDocument(ctx context.Context, orgID, actorID, id string) error {
	existing, err := s.store.GetDocumentByID(ctx, orgID, id)
	if err != nil {
		return err
	}
	if err := s.store.DeleteDocument(ctx, orgID, id); err != nil {
		return err
	}
	s.audit(ctx, orgID, actorID, "document.deleted", "document", existing.ID, map[string]any{"title": existing.Title})
	return nil
}
