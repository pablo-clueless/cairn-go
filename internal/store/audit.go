package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// RecordAudit writes an audit event. actorID may be empty (system action);
// metadata may be nil.
func (db *DB) RecordAudit(ctx context.Context, orgID, actorID, action, entityType, entityID string, metadata map[string]any) error {
	var meta *string
	if len(metadata) > 0 {
		b, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("store: marshal audit metadata: %w", err)
		}
		s := string(b)
		meta = &s
	}

	var actor *string
	if actorID != "" {
		actor = &actorID
	}

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_id, action, entity_type, entity_id, metadata)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5::uuid, $6::jsonb)`,
		orgID, actor, action, entityType, entityID, meta,
	)
	if err != nil {
		return fmt.Errorf("store: record audit: %w", err)
	}
	return nil
}
