package workflow

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func Record(ctx context.Context, tx pgx.Tx, actorID, issueID, recipientID uuid.UUID, kind, body, title string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO issue_events (id, issue_id, actor_user_id, kind, body)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.New(), issueID, actorID, kind, body); err != nil {
		return err
	}
	return Notify(ctx, tx, recipientID, issueID, title, body)
}

func Notify(ctx context.Context, tx pgx.Tx, userID, issueID uuid.UUID, title, body string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (id, user_id, issue_id, title, body)
		VALUES ($1, $2, $3, $4, $5)
	`, uuid.New(), userID, issueID, title, body); err != nil {
		return err
	}
	for _, channel := range []struct{ name, status string }{
		{"in_app", "recorded"},
		{"email", "not_configured"},
		{"sms", "not_configured"},
		{"whatsapp", "not_configured"},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO message_attempts (id, user_id, issue_id, channel, status, body)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, uuid.New(), userID, issueID, channel.name, channel.status, body); err != nil {
			return err
		}
	}
	return nil
}

func OpenReview(ctx context.Context, tx pgx.Tx, issueID uuid.UUID, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO review_items (id, issue_id, reason) VALUES ($1, $2, $3)
	`, uuid.New(), issueID, reason)
	return err
}
