package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cairn/internal/model"
)

// ErrLinkExists is returned when an identical link already exists.
var ErrLinkExists = errors.New("store: issue link already exists")

// CreateIssueLink inserts a directed link between two issues. A duplicate of an
// existing (source, target, type) returns ErrLinkExists.
func (db *DB) CreateIssueLink(ctx context.Context, orgID, sourceID, targetID, linkType, createdBy string) (*model.IssueLink, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO issue_links (organization_id, source_issue_id, target_issue_id, type, created_by)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::uuid)
		RETURNING id::text`,
		orgID, sourceID, targetID, linkType, createdBy,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrLinkExists
		}
		return nil, fmt.Errorf("store: create issue link: %w", err)
	}
	return &model.IssueLink{ID: id, Type: linkType, SourceIssueID: sourceID, TargetIssueID: targetID}, nil
}

// ListIssueLinks returns every link touching an issue, as views from that
// issue's perspective (the other end populated, direction set).
func (db *DB) ListIssueLinks(ctx context.Context, orgID, issueID string) ([]model.IssueLinkView, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT l.id::text, l.type,
			CASE WHEN l.source_issue_id = $2::uuid THEN 'outward' ELSE 'inward' END AS direction,
			CASE WHEN l.source_issue_id = $2::uuid THEN l.target_issue_id ELSE l.source_issue_id END AS other_id
		FROM issue_links l
		WHERE l.organization_id = $1::uuid
		  AND (l.source_issue_id = $2::uuid OR l.target_issue_id = $2::uuid)
		ORDER BY l.created_at ASC`, orgID, issueID)
	if err != nil {
		return nil, fmt.Errorf("store: list issue links: %w", err)
	}
	defer rows.Close()

	type linkRow struct {
		id, linkType, direction, otherID string
	}
	var raw []linkRow
	for rows.Next() {
		var lr linkRow
		if err := rows.Scan(&lr.id, &lr.linkType, &lr.direction, &lr.otherID); err != nil {
			return nil, fmt.Errorf("store: scan issue link: %w", err)
		}
		raw = append(raw, lr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	views := make([]model.IssueLinkView, 0, len(raw))
	for _, lr := range raw {
		other, err := db.GetIssueByID(ctx, orgID, lr.otherID)
		if err != nil {
			return nil, err
		}
		views = append(views, model.IssueLinkView{
			ID:        lr.id,
			Type:      lr.linkType,
			Direction: lr.direction,
			Issue:     *other,
		})
	}
	return views, nil
}

// GetIssueLink fetches a link by id, scoped to the org.
func (db *DB) GetIssueLink(ctx context.Context, orgID, id string) (*model.IssueLink, error) {
	l := &model.IssueLink{}
	err := db.Pool.QueryRow(ctx, `
		SELECT id::text, type, source_issue_id::text, target_issue_id::text, created_at
		FROM issue_links WHERE organization_id = $1::uuid AND id = $2::uuid`, orgID, id,
	).Scan(&l.ID, &l.Type, &l.SourceIssueID, &l.TargetIssueID, &l.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get issue link: %w", err)
	}
	return l, nil
}

// DeleteIssueLink removes a link by id. ErrNotFound if absent.
func (db *DB) DeleteIssueLink(ctx context.Context, orgID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM issue_links WHERE organization_id = $1::uuid AND id = $2::uuid`, orgID, id)
	if err != nil {
		return fmt.Errorf("store: delete issue link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// IssueAncestorIDs returns the chain of parent ids above an issue (nearest
// first), bounded to avoid runaway loops on corrupt data. Used for cycle checks.
func (db *DB) IssueAncestorIDs(ctx context.Context, orgID, issueID string) ([]string, error) {
	rows, err := db.Pool.Query(ctx, `
		WITH RECURSIVE chain AS (
			SELECT id, parent_id, 1 AS depth FROM issues
			WHERE id = $2::uuid AND organization_id = $1::uuid
			UNION ALL
			SELECT i.id, i.parent_id, c.depth + 1
			FROM issues i JOIN chain c ON i.id = c.parent_id
			WHERE c.depth < 50
		)
		SELECT id::text FROM chain WHERE id <> $2::uuid`, orgID, issueID)
	if err != nil {
		return nil, fmt.Errorf("store: issue ancestors: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
