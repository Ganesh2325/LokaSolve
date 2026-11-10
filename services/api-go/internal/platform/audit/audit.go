package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Entry struct {
	ActorID      *uuid.UUID
	ActorRole    string
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	TenantID     *uuid.UUID
	RequestID    string
	Metadata     map[string]any
}

func Write(ctx context.Context, tx pgx.Tx, entry Entry) error {
	if entry.Metadata == nil {
		entry.Metadata = map[string]any{}
	}
	raw, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (
			actor_user_id, actor_role, action, resource_type, resource_id, tenant_id, request_id, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, entry.ActorID, entry.ActorRole, entry.Action, entry.ResourceType, entry.ResourceID, entry.TenantID, entry.RequestID, raw)
	return err
}
