package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const attachmentColumns = `a.id::text, a.organization_id::text, a.issue_id::text, a.uploaded_by::text,
	u.name, a.filename, a.content_type, a.size_bytes, a.storage_key, a.created_at`

func scanAttachment(row pgx.Row) (*model.Attachment, error) {
	a := &model.Attachment{}
	err := row.Scan(&a.ID, &a.OrganizationID, &a.IssueID, &a.UploadedBy, &a.UploaderName,
		&a.Filename, &a.ContentType, &a.SizeBytes, &a.StorageKey, &a.CreatedAt)
	return a, err
}

// CreateAttachment records an uploaded file's metadata.
func (db *DB) CreateAttachment(ctx context.Context, orgID, issueID, uploadedBy, filename, contentType, storageKey string, size int64) (*model.Attachment, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO attachments (organization_id, issue_id, uploaded_by, filename, content_type, size_bytes, storage_key)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7)
		RETURNING id::text`,
		orgID, issueID, uploadedBy, filename, contentType, size, storageKey,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("store: create attachment: %w", err)
	}
	return db.GetAttachmentByID(ctx, orgID, id)
}

// ListAttachmentsByIssue returns an issue's attachments, oldest first.
func (db *DB) ListAttachmentsByIssue(ctx context.Context, orgID, issueID string) ([]model.Attachment, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+attachmentColumns+`
		FROM attachments a LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE a.organization_id = $1::uuid AND a.issue_id = $2::uuid
		ORDER BY a.created_at ASC`, orgID, issueID)
	if err != nil {
		return nil, fmt.Errorf("store: list attachments: %w", err)
	}
	defer rows.Close()
	var out []model.Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan attachment: %w", err)
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GetAttachmentByID fetches one attachment scoped to the org.
func (db *DB) GetAttachmentByID(ctx context.Context, orgID, id string) (*model.Attachment, error) {
	a, err := scanAttachment(db.Pool.QueryRow(ctx, `
		SELECT `+attachmentColumns+`
		FROM attachments a LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE a.organization_id = $1::uuid AND a.id = $2::uuid`, orgID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get attachment: %w", err)
	}
	return a, nil
}

// DeleteAttachment removes an attachment's metadata row.
func (db *DB) DeleteAttachment(ctx context.Context, orgID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM attachments WHERE organization_id = $1::uuid AND id = $2::uuid`, orgID, id)
	if err != nil {
		return fmt.Errorf("store: delete attachment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
