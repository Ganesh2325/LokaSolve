package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"lokasolve/services/api-go/internal/events"
	"lokasolve/services/api-go/internal/platform/config"
	"lokasolve/services/api-go/internal/platform/database"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration", "error", err.Error())
		os.Exit(1)
	}
	if cfg.RedisURL == "" {
		slog.Error("REDIS_URL is required for the worker")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		slog.Error("redis", "error", err.Error())
		os.Exit(1)
	}
	rdb := redis.NewClient(opt)
	stream := events.RedisStream{Client: rdb, Name: events.StreamName}
	if err := stream.EnsureGroup(ctx, "domain-workers"); err != nil {
		slog.Error("stream group", "error", err.Error())
		os.Exit(1)
	}
	slog.Info("worker started")
	for ctx.Err() == nil {
		published, err := events.PublishBatch(ctx, db, stream, 50)
		if err != nil {
			slog.Error("publish", "error", err.Error())
		} else if published > 0 {
			slog.Info("published", "count", published)
		}
		if err := consume(ctx, stream, "worker-1"); err != nil {
			slog.Error("consume", "error", err.Error())
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func consume(ctx context.Context, stream events.RedisStream, consumer string) error {
	pending, err := stream.ReadNew(ctx, "domain-workers", consumer, 50)
	if err != nil {
		return err
	}
	claimed, err := stream.Claim(ctx, "domain-workers", consumer, 50)
	if err != nil {
		return err
	}
	batch := append(pending, claimed...)
	for _, event := range batch {
		first, err := stream.FirstTime(ctx, event.ID)
		if err != nil {
			return err
		}
		if first {
			slog.Info("event received", "event_id", event.ID, "event_type", event.Type)
		}
		if err := stream.Ack(ctx, "domain-workers", event.RedisID); err != nil {
			return err
		}
	}
	return nil
}
