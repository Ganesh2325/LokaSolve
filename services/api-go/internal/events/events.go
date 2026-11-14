package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const StreamName = "lokasolve.domain"

type Event struct {
	ID      string
	Type    string
	Payload []byte
	RedisID string
}

func Enqueue(ctx context.Context, tx pgx.Tx, aggregateType string, aggregateID uuid.UUID, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (schema_version, aggregate_type, aggregate_id, aggregate_version, event_type, idempotency_key, payload)
		VALUES (1, $1, $2, 1, $3, $4, $5)
	`, aggregateType, aggregateID, eventType, uuid.NewString(), raw)
	return err
}

type RedisStream struct {
	Client *redis.Client
	Name   string
}

func (s RedisStream) Append(ctx context.Context, event Event) error {
	return s.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: s.Name,
		Values: map[string]any{
			"event_id":   event.ID,
			"event_type": event.Type,
			"payload":    string(event.Payload),
		},
	}).Err()
}

func (s RedisStream) EnsureGroup(ctx context.Context, group string) error {
	err := s.Client.XGroupCreateMkStream(ctx, s.Name, group, "0").Err()
	if err != nil && !isBusyGroup(err) {
		return err
	}
	return nil
}

func (s RedisStream) ReadNew(ctx context.Context, group, consumer string, count int64) ([]Event, error) {
	streams, err := s.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{s.Name, ">"},
		Count:    count,
		Block:    time.Millisecond,
	}).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return flatten(streams), nil
}

func (s RedisStream) Claim(ctx context.Context, group, consumer string, count int64) ([]Event, error) {
	messages, _, err := s.Client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   s.Name,
		Group:    group,
		Consumer: consumer,
		MinIdle:  0,
		Start:    "0-0",
		Count:    count,
	}).Result()
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(messages))
	for _, message := range messages {
		out = append(out, fromMessage(message))
	}
	return out, nil
}

func (s RedisStream) Ack(ctx context.Context, group string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	return s.Client.XAck(ctx, s.Name, group, ids...).Err()
}

// FirstTime is true when this event id has not been handled yet.
func (s RedisStream) FirstTime(ctx context.Context, eventID string) (bool, error) {
	return s.Client.SetNX(ctx, "lokasolve:processed:"+eventID, "1", 7*24*time.Hour).Result()
}

func flatten(streams []redis.XStream) []Event {
	var out []Event
	for _, stream := range streams {
		for _, message := range stream.Messages {
			out = append(out, fromMessage(message))
		}
	}
	return out
}

func fromMessage(message redis.XMessage) Event {
	event := Event{RedisID: message.ID}
	if value, ok := message.Values["event_id"].(string); ok {
		event.ID = value
	}
	if value, ok := message.Values["event_type"].(string); ok {
		event.Type = value
	}
	if value, ok := message.Values["payload"].(string); ok {
		event.Payload = []byte(value)
	}
	return event
}

func isBusyGroup(err error) bool {
	return err != nil && (err.Error() == "BUSYGROUP Consumer Group name already exists" ||
		len(err.Error()) >= 9 && err.Error()[:9] == "BUSYGROUP")
}
