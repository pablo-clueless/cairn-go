package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cairn/internal/model"
)

// NewNotification carries the fields needed to create a notification.
type NewNotification struct {
	OrgID    string
	UserID   string // recipient
	ActorID  string // may be empty for system actions
	Type     string
	IssueID  string // may be empty
	IssueKey string
	Title    string
	Body     string
}

const notificationSelect = `
	SELECT n.id::text, n.type, n.actor_id::text, ua.name, o.slug, n.issue_id::text, n.issue_key,
		n.title, n.body, n.read_at, n.created_at
	FROM notifications n
	LEFT JOIN users ua ON ua.id = n.actor_id
	JOIN organizations o ON o.id = n.organization_id`

// CreateNotification inserts a notification row.
func (db *DB) CreateNotification(ctx context.Context, n NewNotification) (string, error) {
	var actor, issue *string
	if n.ActorID != "" {
		actor = &n.ActorID
	}
	if n.IssueID != "" {
		issue = &n.IssueID
	}
	var id string
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (organization_id, user_id, actor_id, type, issue_id, issue_key, title, body)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::uuid, $6, $7, $8)
		RETURNING id::text`,
		n.OrgID, n.UserID, actor, n.Type, issue, n.IssueKey, n.Title, n.Body,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("store: create notification: %w", err)
	}
	return id, nil
}

// ListNotifications returns a user's notifications, newest first. unreadOnly
// limits to unread.
func (db *DB) ListNotifications(ctx context.Context, userID string, unreadOnly bool, limit int) ([]model.Notification, error) {
	q := notificationSelect + ` WHERE n.user_id = $1::uuid`
	if unreadOnly {
		q += ` AND n.read_at IS NULL`
	}
	q += ` ORDER BY n.created_at DESC LIMIT $2`
	rows, err := db.Pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list notifications: %w", err)
	}
	defer rows.Close()
	var out []model.Notification
	for rows.Next() {
		var n model.Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.ActorID, &n.ActorName, &n.OrgSlug, &n.IssueID,
			&n.IssueKey, &n.Title, &n.Body, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan notification: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// UnreadCount returns how many unread notifications a user has.
func (db *DB) UnreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1::uuid AND read_at IS NULL`, userID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: unread count: %w", err)
	}
	return n, nil
}

// MarkNotificationRead marks one notification read (scoped to the owner).
func (db *DB) MarkNotificationRead(ctx context.Context, userID, id string) error {
	tag, err := db.Pool.Exec(ctx, `
		UPDATE notifications SET read_at = now()
		WHERE id = $1::uuid AND user_id = $2::uuid AND read_at IS NULL`, id, userID)
	if err != nil {
		return fmt.Errorf("store: mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either it doesn't exist/belong to the user, or it was already read.
		var exists bool
		if err := db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM notifications WHERE id = $1::uuid AND user_id = $2::uuid)`,
			id, userID).Scan(&exists); err != nil {
			return fmt.Errorf("store: mark notification read exists: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

// MarkAllNotificationsRead marks all of a user's unread notifications read.
func (db *DB) MarkAllNotificationsRead(ctx context.Context, userID string) error {
	_, err := db.Pool.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1::uuid AND read_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("store: mark all notifications read: %w", err)
	}
	return nil
}

// GetNotificationPreferences returns a user's email preferences, defaulting to
// all-on when no row exists.
func (db *DB) GetNotificationPreferences(ctx context.Context, userID string) (model.NotificationPreferences, error) {
	prefs := model.NotificationPreferences{EmailMentions: true, EmailComments: true, EmailAssignments: true}
	err := db.Pool.QueryRow(ctx, `
		SELECT email_mentions, email_comments, email_assignments
		FROM notification_preferences WHERE user_id = $1::uuid`, userID,
	).Scan(&prefs.EmailMentions, &prefs.EmailComments, &prefs.EmailAssignments)
	if err != nil {
		// No row yet → return the all-on defaults.
		if errors.Is(err, pgx.ErrNoRows) {
			return prefs, nil
		}
		return prefs, fmt.Errorf("store: get notification prefs: %w", err)
	}
	return prefs, nil
}

// UpsertNotificationPreferences sets a user's email preferences.
func (db *DB) UpsertNotificationPreferences(ctx context.Context, userID string, p model.NotificationPreferences) (model.NotificationPreferences, error) {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO notification_preferences (user_id, email_mentions, email_comments, email_assignments, updated_at)
		VALUES ($1::uuid, $2, $3, $4, now())
		ON CONFLICT (user_id) DO UPDATE SET
			email_mentions = EXCLUDED.email_mentions,
			email_comments = EXCLUDED.email_comments,
			email_assignments = EXCLUDED.email_assignments,
			updated_at = now()`,
		userID, p.EmailMentions, p.EmailComments, p.EmailAssignments)
	if err != nil {
		return p, fmt.Errorf("store: upsert notification prefs: %w", err)
	}
	return p, nil
}
