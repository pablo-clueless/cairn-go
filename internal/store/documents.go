package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const documentSelect = `
	SELECT d.id::text, d.organization_id::text, d.space_id::text, d.parent_id::text,
		d.title, d.type, d.status, d.content,
		d.created_by::text, u.name,
		d.created_at, d.updated_at
	FROM documents d
	LEFT JOIN users u ON u.id = d.created_by`

func scanDocument(row pgx.Row) (*model.Document, error) {
	d := &model.Document{}
	err := row.Scan(&d.ID, &d.OrganizationID, &d.SpaceID, &d.ParentID,
		&d.Title, &d.Type, &d.Status, &d.Content,
		&d.OwnerID, &d.OwnerName, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// CreateDocument inserts a document and returns it (with owner name resolved).
func (db *DB) CreateDocument(ctx context.Context, orgID, spaceID string, parentID *string, title, docType, status, content, createdBy string) (*model.Document, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO documents (organization_id, space_id, parent_id, title, type, status, content, created_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8::uuid)
		RETURNING id::text`,
		orgID, spaceID, parentID, title, docType, status, content, createdBy,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("store: insert document: %w", err)
	}
	return db.GetDocumentByID(ctx, orgID, id)
}

// GetDocumentByID fetches one document scoped to the org. ErrNotFound if absent.
func (db *DB) GetDocumentByID(ctx context.Context, orgID, id string) (*model.Document, error) {
	d, err := scanDocument(db.Pool.QueryRow(ctx,
		documentSelect+` WHERE d.id = $1::uuid AND d.organization_id = $2::uuid`, id, orgID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get document: %w", err)
	}
	return d, nil
}

// ListDocumentsBySpace returns all documents in a space, ordered for stable tree
// building (parents before children isn't guaranteed, but title order is stable).
func (db *DB) ListDocumentsBySpace(ctx context.Context, orgID, spaceID string) ([]model.Document, error) {
	rows, err := db.Pool.Query(ctx,
		documentSelect+` WHERE d.organization_id = $1::uuid AND d.space_id = $2::uuid
		ORDER BY d.created_at ASC`,
		orgID, spaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list documents: %w", err)
	}
	defer rows.Close()

	var docs []model.Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan document: %w", err)
		}
		docs = append(docs, *d)
	}
	return docs, rows.Err()
}

// DocumentUpdate carries optional document changes. A nil field is unchanged;
// an empty ParentID ("") moves the document to the top level (NULL).
type DocumentUpdate struct {
	Title    *string
	Content  *string
	Status   *string
	ParentID *string
}

// UpdateDocument applies a partial update and returns the updated document.
func (db *DB) UpdateDocument(ctx context.Context, orgID, id string, u DocumentUpdate) (*model.Document, error) {
	sets := []string{"updated_at = now()"}
	args := []any{}
	add := func(col, cast string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d%s", col, len(args), cast))
	}

	if u.Title != nil {
		add("title", "", *u.Title)
	}
	if u.Content != nil {
		add("content", "", *u.Content)
	}
	if u.Status != nil {
		add("status", "", *u.Status)
	}
	if u.ParentID != nil {
		if *u.ParentID == "" {
			sets = append(sets, "parent_id = NULL")
		} else {
			add("parent_id", "::uuid", *u.ParentID)
		}
	}

	args = append(args, id)
	idPos := "$" + strconv.Itoa(len(args))
	args = append(args, orgID)
	orgPos := "$" + strconv.Itoa(len(args))

	tag, err := db.Pool.Exec(ctx,
		fmt.Sprintf("UPDATE documents SET %s WHERE id = %s::uuid AND organization_id = %s::uuid",
			strings.Join(sets, ", "), idPos, orgPos),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("store: update document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return db.GetDocumentByID(ctx, orgID, id)
}

// DeleteDocument removes a document (and, via ON DELETE CASCADE, its descendants).
func (db *DB) DeleteDocument(ctx context.Context, orgID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM documents WHERE id = $1::uuid AND organization_id = $2::uuid`, id, orgID,
	)
	if err != nil {
		return fmt.Errorf("store: delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DocumentInSpace reports whether a document id belongs to the given space.
func (db *DB) DocumentInSpace(ctx context.Context, id, spaceID string) (bool, error) {
	var ok bool
	err := db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM documents WHERE id = $1::uuid AND space_id = $2::uuid)`,
		id, spaceID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("store: document in space: %w", err)
	}
	return ok, nil
}
