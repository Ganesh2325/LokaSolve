package admin

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/views"
)

type Service struct {
	DB *database.DB
}

func (s *Service) Users(ctx context.Context, principal authn.Principal, cursor string, limit int) (views.Page[views.AdminUser], error) {
	if !principal.Has("admin:users_read") {
		return views.Page[views.AdminUser]{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	createdBefore, idBefore, err := httpx.DecodeCursor(cursor)
	if err != nil {
		return views.Page[views.AdminUser]{}, err
	}
	limit = httpx.Limit(limit)
	page := views.Page[views.AdminUser]{Data: []views.AdminUser{}}
	err = s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT u.id::text, u.email, p.display_name, u.status, u.is_demo, u.created_at
			FROM users u
			JOIN user_profiles p ON p.user_id = u.id
			WHERE ($1::timestamptz IS NULL OR (u.created_at, u.id) < ($1, $2))
			ORDER BY u.created_at DESC, u.id DESC
			LIMIT $3
		`, nullableTime(createdBefore), nullableID(idBefore), limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		var items []views.AdminUser
		for rows.Next() {
			var item views.AdminUser
			var idText string
			if err := rows.Scan(&idText, &item.Email, &item.DisplayName, &item.Status, &item.IsDemo, &item.CreatedAt); err != nil {
				return err
			}
			item.ID = idText
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) > limit {
			last := items[limit-1]
			lastID, err := uuid.Parse(last.ID)
			if err != nil {
				return err
			}
			page.NextCursor = httpx.EncodeCursor(last.CreatedAt, lastID)
			items = items[:limit]
		}
		for i := range items {
			userID, err := uuid.Parse(items[i].ID)
			if err != nil {
				return err
			}
			roleRows, err := tx.Query(ctx, `
				SELECT r.code FROM user_role_assignments ura
				JOIN roles r ON r.id = ura.role_id
				WHERE ura.user_id = $1 AND ura.organization_id IS NULL
				ORDER BY r.code
			`, userID)
			if err != nil {
				return err
			}
			for roleRows.Next() {
				var code string
				if err := roleRows.Scan(&code); err != nil {
					roleRows.Close()
					return err
				}
				items[i].Roles = append(items[i].Roles, code)
			}
			if err := roleRows.Err(); err != nil {
				roleRows.Close()
				return err
			}
			roleRows.Close()
			if items[i].Roles == nil {
				items[i].Roles = []string{}
			}
		}
		page.Data = items
		return nil
	})
	return page, err
}

func (s *Service) GrantRole(ctx context.Context, principal authn.Principal, targetID uuid.UUID, roleCode, requestID string) error {
	if !principal.Has("admin:roles_grant") {
		return httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	if roleCode != "support_agent" && roleCode != "super_admin" {
		return httpx.Validation(map[string]string{"role_code": "Choose support_agent or super_admin."})
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, targetID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) || status == "deleted" {
			return httpx.New(404, "not_found", "That record was not found.")
		}
		if err != nil {
			return err
		}
		if err := identity.AssignRole(ctx, tx, targetID, roleCode, nil); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &principal.UserID, ActorRole: "super_admin", Action: "admin.role_grant",
			ResourceType: "user", ResourceID: &targetID, RequestID: requestID,
			Metadata: map[string]any{"role_code": roleCode},
		})
	})
}

func (s *Service) Audit(ctx context.Context, principal authn.Principal, cursor string, limit int) (views.Page[views.AuditRecord], error) {
	if !principal.Has("admin:audit_read") {
		return views.Page[views.AuditRecord]{}, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	createdBefore, idBefore, err := httpx.DecodeCursor(cursor)
	if err != nil {
		return views.Page[views.AuditRecord]{}, err
	}
	limit = httpx.Limit(limit)
	page := views.Page[views.AuditRecord]{Data: []views.AuditRecord{}}
	err = s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, actor_user_id::text, actor_role, action, resource_type, resource_id::text,
			       tenant_id::text, request_id, metadata, created_at
			FROM audit_logs
			WHERE ($1::timestamptz IS NULL OR (created_at, id) < ($1, $2))
			ORDER BY created_at DESC, id DESC
			LIMIT $3
		`, nullableTime(createdBefore), nullableID(idBefore), limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		var items []views.AuditRecord
		for rows.Next() {
			var item views.AuditRecord
			var raw []byte
			if err := rows.Scan(&item.ID, &item.ActorUserID, &item.ActorRole, &item.Action, &item.ResourceType, &item.ResourceID, &item.TenantID, &item.RequestID, &raw, &item.CreatedAt); err != nil {
				return err
			}
			item.Metadata = map[string]any{}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &item.Metadata); err != nil {
					return err
				}
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(items) > limit {
			last := items[limit-1]
			lastID, err := uuid.Parse(last.ID)
			if err != nil {
				return err
			}
			page.NextCursor = httpx.EncodeCursor(last.CreatedAt, lastID)
			items = items[:limit]
		}
		if items == nil {
			items = []views.AuditRecord{}
		}
		page.Data = items
		return nil
	})
	return page, err
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func nullableID(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	return &value
}
