package work

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"cairn/internal/model"
	"cairn/internal/store"
)

// mentionRe matches rich mention tokens the editor emits: @[Display Name](uuid).
var mentionRe = regexp.MustCompile(`@\[[^\]]+\]\(([0-9a-fA-F-]{36})\)`)

// parseMentions extracts the unique user ids referenced by @[Name](id) tokens.
func parseMentions(body string) []string {
	matches := mentionRe.FindAllStringSubmatch(body, -1)
	seen := map[string]bool{}
	var ids []string
	for _, m := range matches {
		id := strings.ToLower(m[1])
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// newMentions returns the user ids mentioned in newBody but not in oldBody.
func newMentions(oldBody, newBody string) []string {
	before := map[string]bool{}
	for _, id := range parseMentions(oldBody) {
		before[id] = true
	}
	var added []string
	for _, id := range parseMentions(newBody) {
		if !before[id] {
			added = append(added, id)
		}
	}
	return added
}

// notifyMentions sends "mention" notifications to a specific set of user ids
// (and auto-watches them). The actor is skipped.
func (s *Service) notifyMentions(ctx context.Context, orgID, actorID string, issue *model.Issue, body string, ids []string) {
	actorName := s.actorName(ctx, actorID)
	for _, uid := range ids {
		if uid == actorID {
			continue
		}
		s.autoWatch(ctx, orgID, issue.ID, uid)
		s.emit(ctx, store.NewNotification{
			OrgID: orgID, UserID: uid, ActorID: actorID, Type: model.NotificationMention,
			IssueID: issue.ID, IssueKey: issue.Key,
			Title: fmt.Sprintf("%s mentioned you on %s", actorName, issue.Key),
			Body:  snippet(body),
		}, "mention")
	}
}

// notifyComment fans out notifications for a new comment: @mentioned users get a
// "mention" notification (and auto-watch), and watchers get a "comment" one. The
// actor never notifies themselves; a mentioned user isn't double-notified.
func (s *Service) notifyComment(ctx context.Context, orgID, actorID string, issue *model.Issue, body string) {
	actorName := s.actorName(ctx, actorID)
	notified := map[string]bool{actorID: true}

	mentions := parseMentions(body)
	s.notifyMentions(ctx, orgID, actorID, issue, body, mentions)
	for _, uid := range mentions {
		notified[uid] = true
	}

	watchers, err := s.store.WatcherIDs(ctx, orgID, issue.ID)
	if err != nil {
		slog.Error("notify comment: watcher ids", "issue_id", issue.ID, "error", err)
		return
	}
	for _, uid := range watchers {
		if notified[uid] {
			continue
		}
		notified[uid] = true
		s.emit(ctx, store.NewNotification{
			OrgID: orgID, UserID: uid, ActorID: actorID, Type: model.NotificationComment,
			IssueID: issue.ID, IssueKey: issue.Key,
			Title: fmt.Sprintf("%s commented on %s", actorName, issue.Key),
			Body:  snippet(body),
		}, "comment")
	}
}

// notifyAssignment notifies a newly-assigned user (unless they assigned themselves).
func (s *Service) notifyAssignment(ctx context.Context, orgID, actorID string, issue *model.Issue, assigneeID string) {
	if assigneeID == "" || assigneeID == actorID {
		return
	}
	actorName := s.actorName(ctx, actorID)
	s.emit(ctx, store.NewNotification{
		OrgID: orgID, UserID: assigneeID, ActorID: actorID, Type: model.NotificationAssigned,
		IssueID: issue.ID, IssueKey: issue.Key,
		Title: fmt.Sprintf("%s assigned %s to you", actorName, issue.Key),
		Body:  issue.Title,
	}, "assigned")
}

// emit persists a notification and sends an email when the recipient's prefs
// allow it. Best-effort: failures are logged, never propagated.
func (s *Service) emit(ctx context.Context, n store.NewNotification, kind string) {
	if _, err := s.store.CreateNotification(ctx, n); err != nil {
		slog.Error("emit notification", "user_id", n.UserID, "type", n.Type, "error", err)
		return
	}
	if s.mailer == nil {
		return
	}
	prefs, err := s.store.GetNotificationPreferences(ctx, n.UserID)
	if err != nil {
		slog.Error("emit notification: prefs", "user_id", n.UserID, "error", err)
		return
	}
	switch {
	case kind == "mention" && !prefs.EmailMentions:
		return
	case kind == "comment" && !prefs.EmailComments:
		return
	case kind == "assigned" && !prefs.EmailAssignments:
		return
	}
	recipient, err := s.store.GetUserByID(ctx, n.UserID)
	if err != nil {
		slog.Error("emit notification: recipient", "user_id", n.UserID, "error", err)
		return
	}
	link := s.issueLink(ctx, n.OrgID, n.IssueKey)
	if err := s.mailer.SendNotification(recipient.Email, n.Title, n.Title, link, "View issue "+n.IssueKey); err != nil {
		slog.Error("emit notification: email", "user_id", n.UserID, "error", err)
	}
}

// actorName resolves a user's display name, falling back to "Someone".
func (s *Service) actorName(ctx context.Context, actorID string) string {
	if actorID == "" {
		return "Someone"
	}
	u, err := s.store.GetUserByID(ctx, actorID)
	if err != nil || u == nil {
		return "Someone"
	}
	return u.Name
}

// issueLink builds the frontend deep link to an issue.
func (s *Service) issueLink(ctx context.Context, orgID, issueKey string) string {
	base := strings.TrimRight(s.frontendURL, "/")
	org, err := s.store.GetOrganizationByIDOrSlug(ctx, orgID)
	if err != nil || org == nil {
		return base
	}
	return fmt.Sprintf("%s/org/%s/issues/%s", base, org.Slug, issueKey)
}

// snippet trims a comment body to a short plain-ish preview for notifications.
func snippet(body string) string {
	body = mentionRe.ReplaceAllString(body, "")
	body = strings.TrimSpace(body)
	if len(body) > 140 {
		return body[:140] + "…"
	}
	return body
}
