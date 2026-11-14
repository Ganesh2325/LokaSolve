package workflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"lokasolve/services/api-go/internal/identity"
	"lokasolve/services/api-go/internal/issueclient"
	"lokasolve/services/api-go/internal/platform/audit"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/httpx"
)

type ReviewEvent struct {
	ID        string    `json:"id"`
	Decision  string    `json:"decision"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

func reviewer(principal authn.Principal) bool {
	return principal.Has("admin:users_read") || principal.Has("admin:roles_grant") || principal.Has("admin:audit_read")
}

func (s *Service) DecideReview(ctx context.Context, principal authn.Principal, id uuid.UUID, decision, note, department, severity string) error {
	if !reviewer(principal) {
		return httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	decision = strings.TrimSpace(decision)
	note = strings.TrimSpace(note)
	department = strings.TrimSpace(department)
	severity = strings.TrimSpace(severity)
	status, ok := map[string]string{
		"confirm_routing":       "resolved",
		"correct_routing":       "resolved",
		"request_information":   "needs_information",
		"escalate":              "escalated",
		"reject_classification": "rejected",
		"no_destination":        "no_destination",
	}[decision]
	if !ok {
		return httpx.Validation(map[string]string{"decision": "Choose a review decision."})
	}
	if note == "" {
		return httpx.Validation(map[string]string{"note": "Write the reason for this decision."})
	}
	if severity != "" && severity != "P0" && severity != "P1" && severity != "P2" && severity != "P3" && severity != "P4" {
		return httpx.Validation(map[string]string{"severity": "Use P0, P1, P2, P3, or P4."})
	}
	return s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		return decide(ctx, tx, s.Issue, principal, id, decision, status, note, department, severity)
	})
}

func decide(ctx context.Context, tx pgx.Tx, issues *issueclient.Client, principal authn.Principal, id uuid.UUID, decision, status, note, department, severity string) error {
	var issueID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT issue_id FROM review_items WHERE id = $1`, id).Scan(&issueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.New(404, "not_found", "That review is not available.")
	}
	if err != nil {
		return err
	}
	if severity != "" {
		if issues != nil {
			if err := issues.OverrideSeverity(ctx, issueID, principal.UserID, severity, note); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, `SELECT app_record_severity($1, $2, 'reviewer', $3, $4)`, issueID, severity, note, principal.UserID); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && (pgErr.Code == "42501" || pgErr.Code == "23514") {
				return httpx.New(403, "forbidden", "You do not have access to that action.")
			}
			return err
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE review_items
		SET status = $2, decision = $3, decision_note = $4, assigned_user_id = $5,
			suggested_department = COALESCE(NULLIF($6, ''), suggested_department),
			severity = COALESCE(NULLIF($7, ''), severity)
		WHERE id = $1
	`, id, status, decision, note, principal.UserID, department, severity)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.New(404, "not_found", "That review is not available.")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO review_events (id, review_id, actor_user_id, decision, note)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.New(), id, principal.UserID, decision, note); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{
		ActorID: &principal.UserID, ActorRole: "reviewer", Action: "review.decide",
		ResourceType: "review_item", ResourceID: &id,
		Metadata: map[string]any{"decision": decision, "issue_id": issueID.String(), "severity": severity},
	})
}

func (s *Service) ReviewEvents(ctx context.Context, principal authn.Principal, id uuid.UUID) ([]ReviewEvent, error) {
	if !reviewer(principal) {
		return nil, httpx.New(403, "forbidden", "You do not have access to that action.")
	}
	items := []ReviewEvent{}
	err := s.DB.WithFlags(ctx, &principal.UserID, identity.AccessFlags(principal), func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, decision, note, created_at
			FROM review_events WHERE review_id = $1 ORDER BY created_at
		`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ReviewEvent
			if err := rows.Scan(&item.ID, &item.Decision, &item.Note, &item.CreatedAt); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}
