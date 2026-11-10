package events

import (
	"context"

	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/platform/database"
)

type unpublished struct {
	id      string
	typ     string
	payload []byte
}

func PublishBatch(ctx context.Context, db *database.DB, stream RedisStream, limit int) (int, error) {
	published := 0
	err := db.WithFlags(ctx, nil, []string{"app.worker"}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, event_type, payload
			FROM outbox_events
			WHERE published_at IS NULL
			ORDER BY occurred_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		var batch []unpublished
		for rows.Next() {
			var row unpublished
			if err := rows.Scan(&row.id, &row.typ, &row.payload); err != nil {
				return err
			}
			batch = append(batch, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, row := range batch {
			err := stream.Append(ctx, Event{ID: row.id, Type: row.typ, Payload: row.payload})
			if err != nil {
				message := err.Error()
				if len(message) > 500 {
					message = message[:500]
				}
				if _, updateErr := tx.Exec(ctx, `
					UPDATE outbox_events SET attempts = attempts + 1, last_error = $2 WHERE id = $1
				`, row.id, message); updateErr != nil {
					return updateErr
				}
				continue
			}
			if _, err := tx.Exec(ctx, `
				UPDATE outbox_events SET published_at = now(), last_error = NULL WHERE id = $1
			`, row.id); err != nil {
				return err
			}
			published++
		}
		return nil
	})
	return published, err
}
