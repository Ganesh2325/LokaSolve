package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/events"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/identityclient"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/platform/ratelimit"
	"lokasolve/services/api-go/internal/platform/validate"
	"lokasolve/services/api-go/internal/views"
)

var (
	errInvalidCredentials = httpx.New(401, "invalid_credentials", "Invalid email or password.")
	errInvalidSession     = httpx.New(401, "invalid_session", "Sign in again.")
	errAccountLocked      = httpx.New(423, "account_locked", "This account is temporarily locked. Try again later.")
	errInvalidCode        = httpx.New(401, "invalid_code", "That code is not valid. Request a new one if it has expired.")
	errRateLimited        = httpx.New(429, "rate_limited", "Too many attempts. Wait a moment and try again.")
)

type Meta struct {
	IP        string
	UserAgent string
	RequestID string
}

type RegisterInput struct {
	Email             string
	Password          string
	DisplayName       string
	PreferredLanguage string
	AccountKind       string
	OrganizationName  string
	OrganizationType  string
}

type Service struct {
	DB               *database.DB
	Tokens           *authn.TokenManager
	OTP              authn.Sender
	Pepper           string
	Limiter          ratelimit.Limiter
	LockoutThreshold int
	LockoutFor       time.Duration
	Remote           *identityclient.Client
}

func (s *Service) Register(ctx context.Context, in RegisterInput, requestHash, idemKey string, meta Meta) (views.Session, error) {
	fields := map[string]string{}
	email, emailErr := validate.Email(in.Email)
	fields["email"] = emailErr
	fields["password"] = validate.Password(in.Password)
	name, nameErr := validate.DisplayName(in.DisplayName)
	fields["display_name"] = nameErr
	language, languageErr := validate.Language(in.PreferredLanguage)
	fields["preferred_language"] = languageErr
	if in.AccountKind == "" {
		in.AccountKind = "resident"
	}
	fields["account_kind"] = validate.AccountKind(in.AccountKind)
	orgName := strings.TrimSpace(in.OrganizationName)
	if in.AccountKind == "provider" {
		in.OrganizationType = "service_provider"
		if orgName == "" {
			orgName = name
		}
	}
	if in.AccountKind == "provider" || in.AccountKind == "organization" {
		if utf8.RuneCountInString(orgName) < 2 || utf8.RuneCountInString(orgName) > 120 {
			fields["organization_name"] = "Enter an organization name between 2 and 120 characters."
		}
		fields["organization_type"] = validate.OrgType(in.OrganizationType)
	}
	if hasFields(fields) {
		return views.Session{}, httpx.Validation(fields)
	}

	var session views.Session
	err := s.DB.WithFlags(ctx, nil, []string{"app.auth_lookup"}, func(tx pgx.Tx) error {
		replayID, _, err := takeIdempotency(ctx, tx, idemKey, "POST", "/api/v1/auth/register", requestHash)
		if err != nil {
			return err
		}
		if replayID != nil {
			user, err := identity.LoadUser(ctx, tx, *replayID)
			if err != nil {
				return err
			}
			issued, err := s.issue(ctx, tx, *replayID, meta)
			if err != nil {
				return err
			}
			issued.User = user
			session = issued.Session
			return nil
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return httpx.New(409, "email_taken", "An account with this email already exists.")
		}
		hash, err := authn.HashPassword(in.Password)
		if err != nil {
			return err
		}
		var userID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id
		`, email, hash).Scan(&userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_profiles (user_id, display_name, preferred_language)
			VALUES ($1, $2, $3)
		`, userID, name, language); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_contacts (user_id, channel, value, is_primary)
			VALUES ($1, 'email', $2, true)
		`, userID, email); err != nil {
			return err
		}
		if err := identity.AssignRole(ctx, tx, userID, "resident", nil); err != nil {
			return err
		}
		if in.AccountKind == "provider" || in.AccountKind == "organization" {
			if err := database.SetLocal(ctx, tx, "app.user_id", userID.String()); err != nil {
				return err
			}
			if err := database.SetLocal(ctx, tx, "app.org_create", "on"); err != nil {
				return err
			}
			if err := database.SetLocal(ctx, tx, "app.org_bootstrap", "on"); err != nil {
				return err
			}
			orgID := uuid.New()
			if _, err := tx.Exec(ctx, `
				INSERT INTO organizations (id, name, org_type) VALUES ($1, $2, $3)
			`, orgID, orgName, in.OrganizationType); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO organization_members (organization_id, user_id, member_role)
				VALUES ($1, $2, 'owner')
			`, orgID, userID); err != nil {
				return err
			}
			if err := identity.AssignRole(ctx, tx, userID, "org_admin", &orgID); err != nil {
				return err
			}
			if in.AccountKind == "provider" {
				if err := identity.AssignRole(ctx, tx, userID, "provider", nil); err != nil {
					return err
				}
			}
			if err := events.Enqueue(ctx, tx, "organization", orgID, "organization.created", map[string]any{
				"organization_id": orgID.String(),
				"org_type":        in.OrganizationType,
				"request_id":      meta.RequestID,
			}); err != nil {
				return err
			}
		}
		issued, err := s.issue(ctx, tx, userID, meta)
		if err != nil {
			return err
		}
		user, err := identity.LoadUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		issued.User = user
		session = issued.Session
		if err := audit.Write(ctx, tx, audit.Entry{
			ActorID:      &userID,
			ActorRole:    "resident",
			Action:       "auth.register",
			ResourceType: "user",
			ResourceID:   &userID,
			RequestID:    meta.RequestID,
			Metadata:     map[string]any{"account_kind": in.AccountKind},
		}); err != nil {
			return err
		}
		if err := events.Enqueue(ctx, tx, "user", userID, "user.registered", map[string]any{
			"user_id":      userID.String(),
			"account_kind": in.AccountKind,
			"request_id":   meta.RequestID,
		}); err != nil {
			return err
		}
		if err := finishIdempotency(ctx, tx, idemKey, "POST", "/api/v1/auth/register", userID, 201); err != nil {
			return err
		}
		return nil
	})
	return session, err
}

func (s *Service) Login(ctx context.Context, email, password string, meta Meta) (views.Session, error) {
	normalized, emailErr := validate.Email(email)
	if emailErr != "" || validate.Password(password) != "" && len(password) == 0 {
		return views.Session{}, errInvalidCredentials
	}
	if emailErr != "" {
		return views.Session{}, errInvalidCredentials
	}
	if err := s.limit(ctx, "login:ip:"+meta.IP, 20, time.Minute); err != nil {
		return views.Session{}, err
	}
	if err := s.limit(ctx, "login:email:"+normalized, 10, time.Minute); err != nil {
		return views.Session{}, err
	}
	var session views.Session
	var failure error
	err := s.DB.WithFlags(ctx, nil, []string{"app.auth_lookup"}, func(tx pgx.Tx) error {
		var userID uuid.UUID
		var hash *string
		var status string
		var attempts int
		var lockedUntil *time.Time
		err := tx.QueryRow(ctx, `
			SELECT id, password_hash, status, failed_login_attempts, locked_until
			FROM users WHERE email = $1
		`, normalized).Scan(&userID, &hash, &status, &attempts, &lockedUntil)
		if errors.Is(err, pgx.ErrNoRows) || status == "deleted" || hash == nil {
			authn.VerifyDummy(password)
			if auditErr := audit.Write(ctx, tx, audit.Entry{
				Action: "auth.login_failed", ResourceType: "user", RequestID: meta.RequestID,
				Metadata: map[string]any{"result": "unknown_user"},
			}); auditErr != nil {
				return auditErr
			}
			failure = errInvalidCredentials
			return nil
		}
		if err != nil {
			return err
		}
		if lockedUntil != nil && lockedUntil.After(time.Now()) || status == "locked" {
			_, _ = authn.VerifyPassword(*hash, password)
			failure = errAccountLocked
			return nil
		}
		ok, err := authn.VerifyPassword(*hash, password)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := tx.Exec(ctx, `
				UPDATE users
				SET failed_login_attempts = failed_login_attempts + 1,
				    locked_until = CASE
				      WHEN failed_login_attempts + 1 >= $2 THEN now() + ($3 * interval '1 second')
				      ELSE locked_until
				    END
				WHERE id = $1
			`, userID, s.LockoutThreshold, int(s.LockoutFor.Seconds())); err != nil {
				return err
			}
			if err := audit.Write(ctx, tx, audit.Entry{
				ActorID: &userID, Action: "auth.login_failed", ResourceType: "user", ResourceID: &userID,
				RequestID: meta.RequestID, Metadata: map[string]any{"result": "invalid_password"},
			}); err != nil {
				return err
			}
			failure = errInvalidCredentials
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = $1
		`, userID); err != nil {
			return err
		}
		issued, err := s.issue(ctx, tx, userID, meta)
		if err != nil {
			return err
		}
		user, err := identity.LoadUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		issued.User = user
		if err := audit.Write(ctx, tx, audit.Entry{
			ActorID: &userID, ActorRole: "resident", Action: "auth.login", ResourceType: "user",
			ResourceID: &userID, RequestID: meta.RequestID, Metadata: map[string]any{"result": "success"},
		}); err != nil {
			return err
		}
		session = issued.Session
		_ = attempts
		return nil
	})
	if err != nil {
		return views.Session{}, err
	}
	if failure != nil {
		return views.Session{}, failure
	}
	return session, nil
}

// refreshGrace is the overlap for one legitimate rotation. A second request that
// is still holding the token just replaced joins that rotation. A later replay,
// a revoked session, or a successor that has itself been replaced still revokes
// the family. Expired tokens are rejected and do not revoke other sessions.
const refreshGrace = 30 * time.Second

func (s *Service) Refresh(ctx context.Context, refreshToken string, meta Meta) (views.Session, error) {
	if refreshToken == "" {
		return views.Session{}, errInvalidSession
	}
	var session views.Session
	var failure error
	err := s.DB.WithFlags(ctx, nil, []string{"app.auth_lookup"}, func(tx pgx.Tx) error {
		var sessionID, userID uuid.UUID
		var expires time.Time
		var revokedAt *time.Time
		var replacedBy *uuid.UUID
		var sealed *string
		err := tx.QueryRow(ctx, `
			SELECT id, user_id, expires_at, revoked_at, replaced_by, successor_refresh_sealed
			FROM sessions WHERE refresh_token_hash = $1
			FOR UPDATE
		`, authn.HashRefresh(refreshToken)).Scan(&sessionID, &userID, &expires, &revokedAt, &replacedBy, &sealed)
		if errors.Is(err, pgx.ErrNoRows) {
			failure = errInvalidSession
			return nil
		}
		if err != nil {
			return err
		}
		if revokedAt != nil || replacedBy != nil {
			if joined, ok, err := s.joinRecentRotation(ctx, tx, userID, revokedAt, replacedBy, sealed); err != nil {
				return err
			} else if ok {
				session = joined
				return nil
			}
			if _, err := tx.Exec(ctx, `
				UPDATE sessions
				SET revoked_at = COALESCE(revoked_at, now()), successor_refresh_sealed = NULL
				WHERE user_id = $1
			`, userID); err != nil {
				return err
			}
			if err := audit.Write(ctx, tx, audit.Entry{
				ActorID: &userID, Action: "auth.refresh_reuse", ResourceType: "session", ResourceID: &sessionID,
				RequestID: meta.RequestID, Metadata: map[string]any{"result": "family_revoked"},
			}); err != nil {
				return err
			}
			failure = errInvalidSession
			return nil
		}
		if time.Now().After(expires) {
			failure = errInvalidSession
			return nil
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status); err != nil {
			return err
		}
		if status == "deleted" || status == "locked" {
			failure = errInvalidSession
			return nil
		}
		issued, err := s.issue(ctx, tx, userID, meta)
		if err != nil {
			return err
		}
		sealedToken, err := s.Tokens.Seal(issued.RefreshToken)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE sessions
			SET revoked_at = now(), replaced_by = $2, successor_refresh_sealed = $3
			WHERE id = $1
		`, sessionID, issued.sessionID, sealedToken); err != nil {
			return err
		}
		user, err := identity.LoadUser(ctx, tx, userID)
		if err != nil {
			return err
		}
		issued.User = user
		session = issued.Session
		return nil
	})
	if err != nil {
		return views.Session{}, err
	}
	if failure != nil {
		return views.Session{}, failure
	}
	return session, nil
}

func (s *Service) joinRecentRotation(ctx context.Context, tx pgx.Tx, userID uuid.UUID, revokedAt *time.Time, replacedBy *uuid.UUID, sealed *string) (views.Session, bool, error) {
	if replacedBy == nil || revokedAt == nil || sealed == nil || *sealed == "" {
		return views.Session{}, false, nil
	}
	if time.Since(*revokedAt) > refreshGrace {
		return views.Session{}, false, nil
	}
	var successorExpires time.Time
	var successorRevoked *time.Time
	var successorReplaced *uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT expires_at, revoked_at, replaced_by FROM sessions WHERE id = $1 FOR UPDATE
	`, *replacedBy).Scan(&successorExpires, &successorRevoked, &successorReplaced)
	if err != nil {
		return views.Session{}, false, nil
	}
	if successorRevoked != nil || successorReplaced != nil || time.Now().After(successorExpires) {
		return views.Session{}, false, nil
	}
	raw, err := s.Tokens.Open(*sealed)
	if err != nil {
		return views.Session{}, false, nil
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status); err != nil {
		return views.Session{}, false, err
	}
	if status == "deleted" || status == "locked" {
		return views.Session{}, false, nil
	}
	access, err := s.Tokens.IssueAccess(userID, *replacedBy)
	if err != nil {
		return views.Session{}, false, err
	}
	user, err := identity.LoadUser(ctx, tx, userID)
	if err != nil {
		return views.Session{}, false, err
	}
	return views.Session{
		User:         user,
		AccessToken:  access,
		RefreshToken: raw,
		ExpiresIn:    int(s.Tokens.AccessTTL().Seconds()),
		TokenType:    "Bearer",
	}, true, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string, meta Meta) error {
	if refreshToken == "" {
		return nil
	}
	return s.DB.WithFlags(ctx, nil, []string{"app.auth_lookup"}, func(tx pgx.Tx) error {
		var userID, sessionID uuid.UUID
		err := tx.QueryRow(ctx, `
			UPDATE sessions SET revoked_at = now()
			WHERE refresh_token_hash = $1 AND revoked_at IS NULL
			RETURNING id, user_id
		`, authn.HashRefresh(refreshToken)).Scan(&sessionID, &userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: &userID, Action: "auth.logout", ResourceType: "session", ResourceID: &sessionID,
			RequestID: meta.RequestID,
		})
	})
}

func (s *Service) LoadPrincipal(ctx context.Context, userID, sessionID uuid.UUID) (authn.Principal, error) {
	var principal authn.Principal
	err := s.DB.WithFlags(ctx, &userID, nil, func(tx pgx.Tx) error {
		var lockedUntil *time.Time
		err := tx.QueryRow(ctx, `
			SELECT status, is_demo, locked_until FROM users WHERE id = $1
		`, userID).Scan(&principal.Status, &principal.IsDemo, &lockedUntil)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidSession
		}
		if err != nil {
			return err
		}
		var revokedAt *time.Time
		var expires time.Time
		err = tx.QueryRow(ctx, `
			SELECT expires_at, revoked_at FROM sessions WHERE id = $1 AND user_id = $2
		`, sessionID, userID).Scan(&expires, &revokedAt)
		if errors.Is(err, pgx.ErrNoRows) || revokedAt != nil || time.Now().After(expires) {
			return errInvalidSession
		}
		if err != nil {
			return err
		}
		principal.UserID = userID
		principal.SessionID = sessionID
		principal.Locked = lockedUntil != nil && lockedUntil.After(time.Now())
		principal.GlobalRoles = map[string]struct{}{}
		principal.Permissions = map[string]struct{}{}
		principal.MemberRole = map[uuid.UUID]string{}
		rows, err := tx.Query(ctx, `
			SELECT r.code, ura.organization_id::text, p.code
			FROM user_role_assignments ura
			JOIN roles r ON r.id = ura.role_id
			JOIN role_permissions rp ON rp.role_id = r.id
			JOIN permissions p ON p.id = rp.permission_id
			WHERE ura.user_id = $1
		`, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var roleCode, permission string
			var orgID *string
			if err := rows.Scan(&roleCode, &orgID, &permission); err != nil {
				return err
			}
			if orgID == nil {
				principal.GlobalRoles[roleCode] = struct{}{}
				principal.Permissions[permission] = struct{}{}
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		memberRows, err := tx.Query(ctx, `
			SELECT organization_id::text, member_role
			FROM organization_members
			WHERE user_id = $1 AND status = 'active'
		`, userID)
		if err != nil {
			return err
		}
		defer memberRows.Close()
		for memberRows.Next() {
			var orgText, role string
			if err := memberRows.Scan(&orgText, &role); err != nil {
				return err
			}
			orgID, err := uuid.Parse(orgText)
			if err != nil {
				return err
			}
			principal.MemberRole[orgID] = role
		}
		return memberRows.Err()
	})
	return principal, err
}

type issuedSession struct {
	views.Session
	sessionID uuid.UUID
}

func (s *Service) issue(ctx context.Context, tx pgx.Tx, userID uuid.UUID, meta Meta) (issuedSession, error) {
	raw, hash, err := authn.NewRefreshToken()
	if err != nil {
		return issuedSession{}, err
	}
	var sessionID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO sessions (user_id, refresh_token_hash, user_agent, ip_address, expires_at)
		VALUES ($1, $2, $3, $4, now() + ($5 * interval '1 second'))
		RETURNING id
	`, userID, hash, meta.UserAgent, meta.IP, int(s.Tokens.RefreshTTL().Seconds())).Scan(&sessionID); err != nil {
		return issuedSession{}, err
	}
	access, err := s.Tokens.IssueAccess(userID, sessionID)
	if err != nil {
		return issuedSession{}, err
	}
	return issuedSession{
		sessionID: sessionID,
		Session: views.Session{
			AccessToken:  access,
			RefreshToken: raw,
			ExpiresIn:    int(s.Tokens.AccessTTL().Seconds()),
			TokenType:    "Bearer",
		},
	}, nil
}

func (s *Service) limit(ctx context.Context, key string, limit int, window time.Duration) error {
	if s.Limiter == nil {
		return nil
	}
	ok, err := s.Limiter.Allow(ctx, key, limit, window)
	if err != nil {
		return err
	}
	if !ok {
		return errRateLimited
	}
	return nil
}

func takeIdempotency(ctx context.Context, tx pgx.Tx, key, method, path, hash string) (*uuid.UUID, int, error) {
	if key == "" {
		return nil, 0, nil
	}
	if len(key) < 8 || len(key) > 200 {
		return nil, 0, httpx.New(400, "validation_error", "Idempotency-Key must be between 8 and 200 characters.")
	}
	var storedHash string
	var userID *uuid.UUID
	var status *int
	err := tx.QueryRow(ctx, `
		INSERT INTO idempotency_keys (key, method, path, request_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key, method, path) DO UPDATE SET key = idempotency_keys.key
		RETURNING request_hash, user_id, response_status
	`, key, method, path, hash).Scan(&storedHash, &userID, &status)
	if err != nil {
		return nil, 0, err
	}
	if storedHash != hash {
		return nil, 0, httpx.New(409, "idempotency_conflict", "This idempotency key was already used for a different request.")
	}
	if userID != nil {
		code := 201
		if status != nil {
			code = *status
		}
		return userID, code, nil
	}
	return nil, 0, nil
}

func finishIdempotency(ctx context.Context, tx pgx.Tx, key, method, path string, userID uuid.UUID, status int) error {
	if key == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE idempotency_keys SET user_id = $4, response_status = $5
		WHERE key = $1 AND method = $2 AND path = $3
	`, key, method, path, userID, status)
	return err
}

func hasFields(fields map[string]string) bool {
	for _, value := range fields {
		if value != "" {
			return true
		}
	}
	return false
}
