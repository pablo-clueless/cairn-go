package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const commentColumns = `c.id::text, c.issue_id::text, c.author_id::text, coalesce(u.name, ''), c.body, c.created_at, c.updated_at`

func scanComment(row pgx.Row) (*model.Comment, error) {
	cm := &model.Comment{}
	err := row.Scan(&cm.ID, &cm.IssueID, &cm.AuthorID, &cm.AuthorName, &cm.Body, &cm.CreatedAt, &cm.UpdatedAt)
	return cm, err
}

// CreateComment inserts a comment and returns it with the author name resolved.
func (db *DB) CreateComment(ctx context.Context, orgID, issueID, authorID, body string) (*model.Comment, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO comments (organization_id, issue_id, author_id, body)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
		RETURNING id::text`,
		orgID, issueID, authorID, body,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("store: create comment: %w", err)
	}
	return db.GetCommentByID(ctx, orgID, id)
}

// ListCommentsByIssue returns an issue's comments oldest-first.
func (db *DB) ListCommentsByIssue(ctx context.Context, orgID, issueID string) ([]model.Comment, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT `+commentColumns+`
		FROM comments c LEFT JOIN users u ON u.id = c.author_id
		WHERE c.organization_id = $1::uuid AND c.issue_id = $2::uuid
		ORDER BY c.created_at ASC`, orgID, issueID)
	if err != nil {
		return nil, fmt.Errorf("store: list comments: %w", err)
	}
	defer rows.Close()
	var out []model.Comment
	for rows.Next() {
		cm, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan comment: %w", err)
		}
		out = append(out, *cm)
	}
	return out, rows.Err()
}

// GetCommentByID fetches a comment scoped to an org. Returns ErrNotFound if absent.
func (db *DB) GetCommentByID(ctx context.Context, orgID, id string) (*model.Comment, error) {
	cm, err := scanComment(db.Pool.QueryRow(ctx, `
		SELECT `+commentColumns+`
		FROM comments c LEFT JOIN users u ON u.id = c.author_id
		WHERE c.organization_id = $1::uuid AND c.id = $2::uuid`, orgID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get comment: %w", err)
	}
	return cm, nil
}

// UpdateComment replaces a comment's body.
func (db *DB) UpdateComment(ctx context.Context, orgID, id, body string) (*model.Comment, error) {
	ct, err := db.Pool.Exec(ctx, `
		UPDATE comments SET body = $3, updated_at = now()
		WHERE organization_id = $1::uuid AND id = $2::uuid`, orgID, id, body)
	if err != nil {
		return nil, fmt.Errorf("store: update comment: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return db.GetCommentByID(ctx, orgID, id)
}

// DeleteComment removes a comment.
func (db *DB) DeleteComment(ctx context.Context, orgID, id string) error {
	ct, err := db.Pool.Exec(ctx, `DELETE FROM comments WHERE organization_id = $1::uuid AND id = $2::uuid`, orgID, id)
	if err != nil {
		return fmt.Errorf("store: delete comment: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
