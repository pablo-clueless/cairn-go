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

const issueSelect = `
	SELECT i.id::text, i.organization_id::text, i.space_id::text, s.key AS space_key, i.number,
		(s.key || '-' || i.number) AS key, i.type, i.title, i.description,
		i.status_id::text, st.name, st.category, i.priority,
		i.assignee_id::text, ua.name, i.reporter_id::text, ur.name, i.sprint_id::text,
		i.parent_id::text, CASE WHEN p.id IS NULL THEN NULL ELSE (ps.key || '-' || p.number) END AS parent_key,
		i.due_date, i.rank, i.created_at, i.updated_at
	FROM issues i
	JOIN spaces s ON s.id = i.space_id
	JOIN workflow_statuses st ON st.id = i.status_id
	LEFT JOIN users ua ON ua.id = i.assignee_id
	LEFT JOIN users ur ON ur.id = i.reporter_id
	LEFT JOIN issues p ON p.id = i.parent_id
	LEFT JOIN spaces ps ON ps.id = p.space_id`

func scanIssue(row pgx.Row) (*model.Issue, error) {
	is := &model.Issue{}
	err := row.Scan(&is.ID, &is.OrganizationID, &is.SpaceID, &is.SpaceKey, &is.Number, &is.Key,
		&is.Type, &is.Title, &is.Description, &is.StatusID, &is.Status, &is.StatusCategory, &is.Priority,
		&is.AssigneeID, &is.AssigneeName, &is.ReporterID, &is.ReporterName, &is.SprintID,
		&is.ParentID, &is.ParentKey,
		&is.DueDate, &is.Rank, &is.CreatedAt, &is.UpdatedAt)
	return is, err
}

// CreateIssue allocates the next per-space number and inserts the issue atomically.
func (db *DB) CreateIssue(ctx context.Context, orgID, spaceID, statusID, issueType, title string, description, assigneeID *string, priority, reporterID string, dueDate *string) (*model.Issue, error) {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: begin create issue: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var number int
	err = tx.QueryRow(ctx,
		`UPDATE spaces SET issue_seq = issue_seq + 1, updated_at = now()
		 WHERE id = $1::uuid AND organization_id = $2::uuid RETURNING issue_seq`,
		spaceID, orgID,
	).Scan(&number)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: allocate issue number: %w", err)
	}

	// New issues sort to the bottom of the space: one rank step past the current max.
	var rank float64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(rank), 0) + 1024 FROM issues WHERE space_id = $1::uuid`, spaceID,
	).Scan(&rank); err != nil {
		return nil, fmt.Errorf("store: allocate issue rank: %w", err)
	}

	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO issues (organization_id, space_id, number, status_id, type, title, description, priority, assignee_id, reporter_id, due_date, rank)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid, $5, $6, $7, $8, $9::uuid, $10::uuid, $11::date, $12)
		RETURNING id::text`,
		orgID, spaceID, number, statusID, issueType, title, description, priority, assigneeID, reporterID, dueDate, rank,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("store: insert issue: %w", err)
	}

	// Seed the status history so reports have a starting point for this issue.
	if _, err := tx.Exec(ctx, `
		INSERT INTO issue_status_history (organization_id, issue_id, space_id, status_id, category)
		SELECT $1::uuid, $2::uuid, $3::uuid, st.id, st.category
		FROM workflow_statuses st WHERE st.id = $4::uuid`,
		orgID, id, spaceID, statusID); err != nil {
		return nil, fmt.Errorf("store: seed status history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("store: commit create issue: %w", err)
	}
	return db.GetIssueByID(ctx, orgID, id)
}

// GetIssueByID fetches one issue scoped to the org. ErrNotFound if absent.
func (db *DB) GetIssueByID(ctx context.Context, orgID, id string) (*model.Issue, error) {
	is, err := scanIssue(db.Pool.QueryRow(ctx,
		issueSelect+` WHERE i.id = $1::uuid AND i.organization_id = $2::uuid`, id, orgID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get issue: %w", err)
	}
	return is, nil
}

// GetIssueByKey fetches one issue by space key + number. ErrNotFound if absent.
func (db *DB) GetIssueByKey(ctx context.Context, orgID, spaceKey string, number int) (*model.Issue, error) {
	is, err := scanIssue(db.Pool.QueryRow(ctx,
		issueSelect+` WHERE i.organization_id = $1::uuid AND s.key = $2 AND i.number = $3`,
		orgID, spaceKey, number,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("store: get issue by key: %w", err)
	}
	return is, nil
}

// IssueFilter narrows a list query. Empty fields are ignored.
// Sprint: "backlog" matches issues with no sprint; a UUID matches that sprint.
type IssueFilter struct {
	SpaceID    string
	AssigneeID string
	StatusID   string
	Sprint     string
	ParentID   string   // matches issues whose parent_id is this issue (children)
	SpaceIDs   []string // restrict to these spaces (per-space visibility); nil = no restriction
}

// ListIssues returns issues for an org, applying optional filters.
func (db *DB) ListIssues(ctx context.Context, orgID string, f IssueFilter) ([]model.Issue, error) {
	conds := []string{"i.organization_id = $1::uuid"}
	args := []any{orgID}
	if f.SpaceID != "" {
		args = append(args, f.SpaceID)
		conds = append(conds, fmt.Sprintf("i.space_id = $%d::uuid", len(args)))
	}
	if f.AssigneeID != "" {
		args = append(args, f.AssigneeID)
		conds = append(conds, fmt.Sprintf("i.assignee_id = $%d::uuid", len(args)))
	}
	if f.StatusID != "" {
		args = append(args, f.StatusID)
		conds = append(conds, fmt.Sprintf("i.status_id = $%d::uuid", len(args)))
	}
	if f.Sprint == "backlog" {
		conds = append(conds, "i.sprint_id IS NULL")
	} else if f.Sprint != "" {
		args = append(args, f.Sprint)
		conds = append(conds, fmt.Sprintf("i.sprint_id = $%d::uuid", len(args)))
	}
	if f.ParentID != "" {
		args = append(args, f.ParentID)
		conds = append(conds, fmt.Sprintf("i.parent_id = $%d::uuid", len(args)))
	}
	if f.SpaceIDs != nil {
		args = append(args, f.SpaceIDs)
		conds = append(conds, fmt.Sprintf("i.space_id = ANY($%d::uuid[])", len(args)))
	}

	rows, err := db.Pool.Query(ctx,
		issueSelect+" WHERE "+strings.Join(conds, " AND ")+" ORDER BY i.rank ASC, i.created_at ASC", args...,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list issues: %w", err)
	}
	defer rows.Close()

	var issues []model.Issue
	for rows.Next() {
		is, err := scanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan issue: %w", err)
		}
		issues = append(issues, *is)
	}
	return issues, rows.Err()
}

// SearchIssues finds issues by full-text (title + description), partial title,
// or issue-key prefix, ranked by text relevance then recency.
func (db *DB) SearchIssues(ctx context.Context, orgID, query string, limit int) ([]model.Issue, error) {
	if limit <= 0 {
		limit = 20
	}
	const tsv = `to_tsvector('english', coalesce(i.title,'') || ' ' || coalesce(i.description,''))`
	rows, err := db.Pool.Query(ctx, issueSelect+`
		WHERE i.organization_id = $1::uuid AND (
			`+tsv+` @@ websearch_to_tsquery('english', $2)
			OR i.title ILIKE '%' || $2 || '%'
			OR (s.key || '-' || i.number) ILIKE $2 || '%'
		)
		ORDER BY ts_rank(`+tsv+`, websearch_to_tsquery('english', $2)) DESC, i.updated_at DESC
		LIMIT $3`, orgID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("store: search issues: %w", err)
	}
	defer rows.Close()
	var issues []model.Issue
	for rows.Next() {
		is, err := scanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan search issue: %w", err)
		}
		issues = append(issues, *is)
	}
	return issues, rows.Err()
}

// IssueUpdate carries optional issue field changes. A nil field is unchanged;
// an empty AssigneeID ("") unassigns.
type IssueUpdate struct {
	Type        *string
	Title       *string
	Description *string
	StatusID    *string
	Priority    *string
	AssigneeID  *string
	SprintID    *string  // "" moves to backlog (NULL)
	ParentID    *string  // "" detaches from parent (NULL); otherwise a parent issue id
	DueDate     *string  // "" clears the due date (NULL); otherwise a YYYY-MM-DD date
	Rank        *float64 // fractional sort key within the space
}

// UpdateIssue applies a partial update and returns the updated issue.
func (db *DB) UpdateIssue(ctx context.Context, orgID, id string, u IssueUpdate) (*model.Issue, error) {
	sets := []string{"updated_at = now()"}
	args := []any{}
	add := func(col, cast string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d%s", col, len(args), cast))
	}

	if u.Type != nil {
		add("type", "", *u.Type)
	}
	if u.Title != nil {
		add("title", "", *u.Title)
	}
	if u.Description != nil {
		add("description", "", *u.Description)
	}
	if u.StatusID != nil {
		add("status_id", "::uuid", *u.StatusID)
	}
	if u.Priority != nil {
		add("priority", "", *u.Priority)
	}
	if u.AssigneeID != nil {
		if *u.AssigneeID == "" {
			sets = append(sets, "assignee_id = NULL")
		} else {
			add("assignee_id", "::uuid", *u.AssigneeID)
		}
	}
	if u.SprintID != nil {
		if *u.SprintID == "" {
			sets = append(sets, "sprint_id = NULL")
		} else {
			add("sprint_id", "::uuid", *u.SprintID)
		}
	}
	if u.ParentID != nil {
		if *u.ParentID == "" {
			sets = append(sets, "parent_id = NULL")
		} else {
			add("parent_id", "::uuid", *u.ParentID)
		}
	}
	if u.DueDate != nil {
		if *u.DueDate == "" {
			sets = append(sets, "due_date = NULL")
		} else {
			add("due_date", "::date", *u.DueDate)
		}
	}
	if u.Rank != nil {
		add("rank", "", *u.Rank)
	}

	args = append(args, id)
	idPos := "$" + strconv.Itoa(len(args))
	args = append(args, orgID)
	orgPos := "$" + strconv.Itoa(len(args))

	tag, err := db.Pool.Exec(ctx,
		fmt.Sprintf("UPDATE issues SET %s WHERE id = %s::uuid AND organization_id = %s::uuid",
			strings.Join(sets, ", "), idPos, orgPos),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("store: update issue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return db.GetIssueByID(ctx, orgID, id)
}

// DeleteIssue removes an issue. ErrNotFound if absent.
func (db *DB) DeleteIssue(ctx context.Context, orgID, id string) error {
	tag, err := db.Pool.Exec(ctx,
		`DELETE FROM issues WHERE id = $1::uuid AND organization_id = $2::uuid`, id, orgID,
	)
	if err != nil {
		return fmt.Errorf("store: delete issue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
