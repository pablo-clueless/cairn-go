// Package realtime defines the broadcast seam used by HTTP handlers to push
// live events to connected clients. The Socket.IO-backed implementation lives
// alongside this interface; handlers depend only on Broadcaster so they stay
// transport-agnostic and testable (the no-op is used in tests).
package realtime

// Event names follow "<entity>.<action>", e.g. "comment.created".
const (
	EventCommentCreated = "comment.created"
	EventCommentUpdated = "comment.updated"
	EventCommentDeleted = "comment.deleted"
)

// Broadcaster pushes an event with a JSON-serializable payload to a room.
type Broadcaster interface {
	// EmitToIssue sends to everyone subscribed to a single issue's room.
	EmitToIssue(issueID, event string, payload any)
	// EmitToOrg sends to everyone in an organization's room.
	EmitToOrg(orgID, event string, payload any)
}

// NoopBroadcaster discards all events. Used until the Socket.IO hub is wired
// and in tests.
type NoopBroadcaster struct{}

func (NoopBroadcaster) EmitToIssue(string, string, any) {}
func (NoopBroadcaster) EmitToOrg(string, string, any)   {}
