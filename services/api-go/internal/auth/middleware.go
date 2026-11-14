package auth

import (
	"context"
	"net/http"

	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type principalKey struct{}

func WithPrincipal(ctx context.Context, principal authn.Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, principal)
}

func Current(ctx context.Context) (authn.Principal, bool) {
	principal, ok := ctx.Value(principalKey{}).(authn.Principal)
	return principal, ok
}

func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Remote != nil {
			s.authenticateRemote(w, r, next)
			return
		}
		raw := httpx.Bearer(r)
		if raw == "" {
			httpx.WriteError(w, r, errInvalidSession)
			return
		}
		userID, sessionID, err := s.Tokens.ParseAccess(raw)
		if err != nil {
			httpx.WriteError(w, r, errInvalidSession)
			return
		}
		principal, err := s.LoadPrincipal(r.Context(), userID, sessionID)
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
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

func Require(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := Current(r.Context())
			if !ok || !principal.Has(permission) {
				httpx.WriteError(w, r, httpx.New(http.StatusForbidden, "forbidden", "You do not have access to that action."))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
