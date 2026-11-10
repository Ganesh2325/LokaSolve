package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/config"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/ratelimit"
	"lokasolve/services/api-go/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration", "error", err.Error())
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.AutoMigrate {
		if err := migrate(ctx, cfg); err != nil {
			slog.Error("migration", "error", err.Error())
			os.Exit(1)
		}
	}
	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()
	privileged, role, err := database.IsPrivileged(ctx, db.Pool)
	if err != nil {
		slog.Error("database role", "error", err.Error())
		os.Exit(1)
	}
	if privileged && os.Getenv("ALLOW_PRIVILEGED_DB") != "true" {
		slog.Error("database role bypasses row-level security", "role", role)
		os.Exit(1)
	}
	var rdb *redis.Client
	var limiter ratelimit.Limiter = ratelimit.NewMemory()
	if cfg.RedisURL != "" {
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("redis", "error", err.Error())
			os.Exit(1)
		}
		rdb = redis.NewClient(opt)
		if err := rdb.Ping(ctx).Err(); err != nil {
			slog.Warn("redis unavailable, using in-memory rate limits", "error", err.Error())
			_ = rdb.Close()
			rdb = nil
		} else {
			limiter = ratelimit.Redis{Client: rdb}
		}
	}
	var sender authn.Sender = authn.LogSender{}
	if cfg.OTPDevExpose && cfg.Dev() {
		sender = authn.DevFileSender{Path: "tmp/otp-dev.txt"}
		slog.Warn("OTP codes are written to tmp/otp-dev.txt because OTP_DEV_EXPOSE is enabled")
	}
	app := server.New(cfg, db, rdb, sender, limiter)
	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: app.Handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()
	app.LogStartup(cfg.HTTPAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "error", err.Error())
		os.Exit(1)
	}
}

func migrate(ctx context.Context, cfg config.Config) error {
	url := cfg.MigrateDatabaseURL
	if url == "" {
		url = cfg.DatabaseURL
	}
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return database.Migrate(ctx, conn)
}
