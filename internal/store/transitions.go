package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

const transitionColumns = `id::text, organization_id::text, space_id::text, from_status_id::text, to_status_id::text, created_at`

func scanTransition(row pgx.Row) (*model.StatusTransition, error) {
	t := &model.StatusTransition{}
	err := row.Scan(&t.ID, &t.OrganizationID, &t.SpaceID, &t.FromStatusID, &t.ToStatusID, &t.CreatedAt)
	return t, err
}

// TransitionPair is one allowed edge. From nil means "from any status".
type TransitionPair struct {
	From *string
	To   string
}

// ListTransitions returns a space's configured status transitions.
func (db *DB) ListTransitions(ctx context.Context, orgID, spaceID string) ([]model.StatusTransition, error) {
	rows, err := db.Pool.Query(ctx,
		`SELECT `+transitionColumns+` FROM status_transitions
		 WHERE organization_id = $1::uuid AND space_id = $2::uuid ORDER BY created_at`,
		orgID, spaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list transitions: %w", err)
	}
	defer rows.Close()

	var transitions []model.StatusTransition
	for rows.Next() {
		t, err := scanTransition(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan transition: %w", err)
		}
		transitions = append(transitions, *t)
	}
	return transitions, rows.Err()
}

// TransitionAllowed reports whether an issue may move from one status to another
// in a space. An open workflow (no rows for the space) allows anything; a no-op
// (from == to) is always allowed; otherwise a matching edge must exist (the edge
// may be global, i.e. from_status_id IS NULL).
func (db *DB) TransitionAllowed(ctx context.Context, spaceID, fromID, toID string) (bool, error) {
	if fromID == toID {
		return true, nil
	}
	var ok bool
	err := db.Pool.QueryRow(ctx, `
		SELECT
			NOT EXISTS (SELECT 1 FROM status_transitions WHERE space_id = $1::uuid)
			OR EXISTS (
				SELECT 1 FROM status_transitions
				WHERE space_id = $1::uuid AND to_status_id = $3::uuid
				  AND (from_status_id = $2::uuid OR from_status_id IS NULL)
			)`,
		spaceID, fromID, toID,
	).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("store: transition allowed: %w", err)
	}
	return ok, nil
}

// SetTransitions replaces a space's entire transition set in one transaction.
// Every referenced status must belong to the space, else ErrNotFound. Returns
// the resulting transitions.
func (db *DB) SetTransitions(ctx context.Context, orgID, spaceID string, pairs []TransitionPair) ([]model.StatusTransition, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: begin set transitions: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`DELETE FROM status_transitions WHERE organization_id = $1::uuid AND space_id = $2::uuid`,
		orgID, spaceID,
	); err != nil {
		return nil, fmt.Errorf("store: clear transitions: %w", err)
	}

	for _, p := range pairs {
		// INSERT...SELECT guards that to_status (and from_status, when given)
		// belong to this space; RowsAffected 0 means one of them did not.
		tag, err := tx.Exec(ctx, `
			INSERT INTO status_transitions (organization_id, space_id, from_status_id, to_status_id)
			SELECT $1::uuid, $2::uuid, $3::uuid, t.id
			FROM workflow_statuses t
			WHERE t.id = $4::uuid AND t.space_id = $2::uuid
			  AND ($3::uuid IS NULL OR EXISTS (
			        SELECT 1 FROM workflow_statuses f WHERE f.id = $3::uuid AND f.space_id = $2::uuid))`,
			orgID, spaceID, p.From, p.To,
		)
		if err != nil {
			return nil, fmt.Errorf("store: insert transition: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil, ErrNotFound // a status that isn't in this space
		}
	}

	rows, err := tx.Query(ctx,
		`SELECT `+transitionColumns+` FROM status_transitions
		 WHERE organization_id = $1::uuid AND space_id = $2::uuid ORDER BY created_at`,
		orgID, spaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("store: reload transitions: %w", err)
	}
	defer rows.Close()

	var transitions []model.StatusTransition
	for rows.Next() {
		t, err := scanTransition(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan transition: %w", err)
		}
		transitions = append(transitions, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("store: commit set transitions: %w", err)
	}
	return transitions, nil
}
