package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/platform/validate"
	"lokasolve/services/api-go/internal/views"
)

type OTPOutcome struct {
	Session *views.Session
	User    *views.User
}

func (s *Service) RequestOTP(ctx context.Context, userID *uuid.UUID, phone, purpose string, meta Meta) error {
	normalized, phoneErr := validate.Phone(phone)
	if phoneErr != "" || (purpose != "login" && purpose != "verify_phone") {
		fields := map[string]string{}
		if phoneErr != "" {
			fields["phone"] = phoneErr
		}
		if purpose != "login" && purpose != "verify_phone" {
			fields["purpose"] = "Choose login or verify_phone."
		}
		return httpx.Validation(fields)
	}
	if purpose == "verify_phone" && userID == nil {
		return errInvalidSession
	}
	if err := s.limit(ctx, "otp:"+normalized, 3, 10*time.Minute); err != nil {
		return err
	}
	flags := []string{"app.auth_lookup"}
	return s.DB.WithFlags(ctx, userID, flags, func(tx pgx.Tx) error {
		var target *uuid.UUID
		if purpose == "login" {
			var found uuid.UUID
			err := tx.QueryRow(ctx, `
				SELECT id FROM users
				WHERE phone_e164 = $1 AND phone_verified_at IS NOT NULL AND status <> 'deleted'
			`, normalized).Scan(&found)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			target = &found
		} else {
			target = userID
		}
		if target == nil {
			return nil
		}
		code, err := authn.NewCode()
		if err != nil {
			return err
		}
		var challengeID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO otp_challenges (destination, channel, code_hash, purpose, user_id, expires_at)
			VALUES ($1, 'sms', $2, $3, $4, now() + interval '5 minutes')
			RETURNING id
		`, normalized, authn.HashCode(s.Pepper, code), purpose, target).Scan(&challengeID); err != nil {
			return err
		}
		if s.OTP == nil {
			_, _ = tx.Exec(ctx, `DELETE FROM otp_challenges WHERE id = $1`, challengeID)
			return httpx.New(503, "otp_unavailable", "Text-message delivery is not configured.")
		}
		if err := s.OTP.Send(ctx, authn.Message{
			Channel: "sms", Destination: normalized, Code: code,
			Body: "Your LokaSolve verification code expires in 5 minutes.",
		}); err != nil {
			_, _ = tx.Exec(ctx, `DELETE FROM otp_challenges WHERE id = $1`, challengeID)
			return httpx.New(503, "otp_unavailable", "The verification code could not be sent.")
		}
		_ = database.SetLocal(ctx, tx, "app.user_id", target.String())
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: target, Action: "auth.otp_requested", ResourceType: "otp_challenge",
			ResourceID: &challengeID, RequestID: meta.RequestID,
			Metadata: map[string]any{"purpose": purpose, "channel": "sms"},
		})
	})
}

func (s *Service) VerifyOTP(ctx context.Context, userID *uuid.UUID, phone, purpose, code string, meta Meta) (OTPOutcome, error) {
	normalized, phoneErr := validate.Phone(phone)
	if phoneErr != "" || len(code) != 6 || (purpose != "login" && purpose != "verify_phone") {
		return OTPOutcome{}, errInvalidCode
	}
	if purpose == "verify_phone" && userID == nil {
		return OTPOutcome{}, errInvalidSession
	}
	var outcome OTPOutcome
	var failure error
	err := s.DB.WithFlags(ctx, userID, []string{"app.auth_lookup"}, func(tx pgx.Tx) error {
		var challengeID uuid.UUID
		var codeHash string
		var owner *uuid.UUID
		var expires time.Time
		var attempts int
		err := tx.QueryRow(ctx, `
			SELECT id, code_hash, user_id, expires_at, attempts
			FROM otp_challenges
			WHERE destination = $1 AND purpose = $2 AND consumed_at IS NULL
			ORDER BY created_at DESC
			LIMIT 1
		`, normalized, purpose).Scan(&challengeID, &codeHash, &owner, &expires, &attempts)
		if errors.Is(err, pgx.ErrNoRows) || time.Now().After(expires) || attempts >= 5 {
			failure = errInvalidCode
			return nil
		}
		if err != nil {
			return err
		}
		if userID != nil && (owner == nil || *owner != *userID) {
			failure = errInvalidCode
			return nil
		}
		if !authn.CodeMatches(s.Pepper, code, codeHash) {
			if _, err := tx.Exec(ctx, `UPDATE otp_challenges SET attempts = attempts + 1 WHERE id = $1`, challengeID); err != nil {
				return err
			}
			failure = errInvalidCode
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE otp_challenges SET consumed_at = now() WHERE id = $1`, challengeID); err != nil {
			return err
		}
		if purpose == "verify_phone" {
			var taken bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM users WHERE phone_e164 = $1 AND id <> $2)
			`, normalized, *userID).Scan(&taken); err != nil {
				return err
			}
			if taken {
				return httpx.New(409, "phone_taken", "That phone number is already verified on another account.")
			}
			if _, err := tx.Exec(ctx, `
				UPDATE users SET phone_e164 = $2, phone_verified_at = now() WHERE id = $1
			`, *userID, normalized); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO user_contacts (user_id, channel, value, verified_at, is_primary)
				VALUES ($1, 'sms', $2, now(), false)
				ON CONFLICT (channel, value) DO UPDATE
				SET verified_at = now(), user_id = EXCLUDED.user_id
			`, *userID, normalized); err != nil {
				return err
			}
			user, err := identity.LoadUser(ctx, tx, *userID)
			if err != nil {
				return err
			}
			if err := audit.Write(ctx, tx, audit.Entry{
				ActorID: userID, Action: "auth.phone_verified", ResourceType: "user", ResourceID: userID,
				RequestID: meta.RequestID, Metadata: map[string]any{"channel": "sms"},
			}); err != nil {
				return err
			}
			outcome.User = &user
			return nil
		}
		if owner == nil {
			failure = errInvalidCode
			return nil
		}
		issued, err := s.issue(ctx, tx, *owner, meta)
		if err != nil {
			return err
		}
		user, err := identity.LoadUser(ctx, tx, *owner)
		if err != nil {
			return err
		}
		issued.User = user
		copied := issued.Session
		outcome.Session = &copied
		return audit.Write(ctx, tx, audit.Entry{
			ActorID: owner, Action: "auth.login", ResourceType: "user", ResourceID: owner,
			RequestID: meta.RequestID, Metadata: map[string]any{"result": "otp"},
		})
	})
	if err != nil {
		return OTPOutcome{}, err
	}
	if failure != nil {
		return OTPOutcome{}, failure
	}
	return outcome, nil
}
