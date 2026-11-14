package auth

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/identityclient"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/httpx"
)

func (s *Service) authenticateRemote(w http.ResponseWriter, r *http.Request, next http.Handler) {
	raw := httpx.Bearer(r)
	if raw == "" {
		httpx.WriteError(w, r, errInvalidSession)
		return
	}
	principal, memberships, err := s.Remote.Resolve(r.Context(), raw)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if principal.Status == "deleted" {
		httpx.WriteError(w, r, errInvalidSession)
		return
	}
	if principal.Locked || principal.Status == "locked" {
		httpx.WriteError(w, r, errAccountLocked)
		return
	}
	if principal.Status != "active" && principal.Status != "pending_deletion" {
		httpx.WriteError(w, r, httpx.New(http.StatusForbidden, "forbidden", "This account cannot sign in."))
		return
	}
	s.projectMemberships(r.Context(), principal, memberships)
	next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
}

func (s *Service) projectMemberships(ctx context.Context, principal authn.Principal, memberships []identityclient.Membership) {
	if s.DB == nil || len(memberships) == 0 {
		return
	}
	err := s.DB.WithFlags(ctx, &principal.UserID, nil, func(tx pgx.Tx) error {
		for _, item := range memberships {
			if _, err := tx.Exec(ctx, `
				INSERT INTO identity_org_projection (id, org_type, status)
				VALUES ($1, $2, $3)
				ON CONFLICT (id) DO UPDATE SET org_type = EXCLUDED.org_type, status = EXCLUDED.status, projected_at = now()
			`, item.OrganizationID, item.OrgType, item.Status); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO identity_member_projection (organization_id, user_id, member_role, status)
				VALUES ($1, $2, $3, 'active')
				ON CONFLICT (organization_id, user_id) DO UPDATE
				SET member_role = EXCLUDED.member_role, status = EXCLUDED.status, projected_at = now()
			`, item.OrganizationID, principal.UserID, item.MemberRole); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("identity projection failed", "error", err.Error())
	}
}
