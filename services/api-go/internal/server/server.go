package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
	"lokasolve/services/api-go/internal/admin"
	"lokasolve/services/api-go/internal/auth"
	"lokasolve/services/api-go/internal/authority"
	"lokasolve/services/api-go/internal/identityclient"
	"lokasolve/services/api-go/internal/identitymode"
	"lokasolve/services/api-go/internal/issuebridge"
	"lokasolve/services/api-go/internal/issueclient"
	"lokasolve/services/api-go/internal/issuemode"
	"lokasolve/services/api-go/internal/issues"
	"lokasolve/services/api-go/internal/organizations"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/config"
	"lokasolve/services/api-go/internal/platform/database"
	"lokasolve/services/api-go/internal/platform/httpx"
	"lokasolve/services/api-go/internal/platform/ratelimit"
	"lokasolve/services/api-go/internal/taxonomy"
	"lokasolve/services/api-go/internal/users"
	"lokasolve/services/api-go/internal/workflow"
)

type App struct {
	Handler http.Handler
	DB      *database.DB
	Redis   *redis.Client
	Auth    *auth.Service
}

func New(cfg config.Config, db *database.DB, rdb *redis.Client, sender authn.Sender, limiter ratelimit.Limiter) *App {
	if sender == nil {
		sender = authn.LogSender{}
	}
	if limiter == nil {
		limiter = ratelimit.NewMemory()
	}
	if cfg.IdentityWritesClosed || cfg.IdentityBaseURL != "" {
		identitymode.WritesClosed = true
	}
	if cfg.IssueWritesClosed || cfg.IssueBaseURL != "" {
		issuemode.WritesClosed = true
	}
	var remote *identityclient.Client
	if cfg.IdentityBaseURL != "" {
		remote = identityclient.New(cfg.IdentityBaseURL, cfg.IdentityServiceToken)
	}
	var issuesRemote *issueclient.Client
	if cfg.IssueBaseURL != "" {
		issuesRemote = issueclient.New(cfg.IssueBaseURL, cfg.IssueServiceToken)
	}
	authService := &auth.Service{
		DB: db, Tokens: authn.NewTokenManager(cfg.AuthTokenSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL),
		OTP: sender, Pepper: cfg.OTPPepper, Limiter: limiter,
		LockoutThreshold: cfg.LockoutThreshold, LockoutFor: cfg.LockoutDuration,
		Remote: remote,
	}
	userService := &users.Service{DB: db}
	orgService := &organizations.Service{DB: db}
	adminService := &admin.Service{DB: db}
	authHandler := auth.Handler{Service: authService, CookieSecure: cfg.CookieSecure, RefreshTTL: int(cfg.RefreshTokenTTL.Seconds())}

	router := chi.NewRouter()
	router.Use(httpx.RequestIDs)
	router.Use(httpx.SecurityHeaders)
	router.Use(httpx.Recover)
	router.Use(httpx.AccessLog)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Idempotency-Key", "X-LokaSolve-Client", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	router.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "database": "down"})
			return
		}
		body := map[string]string{"status": "ready", "database": "up", "redis": "not_configured"}
		if rdb != nil {
			if err := rdb.Ping(ctx).Err(); err != nil {
				body["redis"] = "down"
			} else {
				body["redis"] = "up"
			}
		}
		httpx.WriteJSON(w, http.StatusOK, body)
	})
	router.Route("/api/v1", func(r chi.Router) {
		r.Use(httpx.MaxBody(6 << 20))
		authHandler.Routes(r)
		r.Group(func(r chi.Router) {
			r.Use(authService.Authenticate)
			users.Handler{Service: userService}.Routes(r)
			issues.Handler{Service: &issues.Service{DB: db}}.Routes(r)
			taxonomy.Handler{Service: &taxonomy.Service{DB: db}}.Routes(r)
			workflow.Handler{Service: &workflow.Service{DB: db, Issue: issuesRemote}}.Routes(r)
			authority.Handler{Service: &authority.Service{DB: db, Issue: issuesRemote}}.Routes(r)
			organizations.Handler{Service: orgService}.Routes(r)
			admin.Handler{Service: adminService}.Routes(r)
			r.Post("/me/phone/otp/request", authHandler.RequestPhoneOTP)
			r.Post("/me/phone/otp/verify", authHandler.VerifyPhoneOTP)
		})
	})
	issuebridge.Handler{DB: db, Token: cfg.IssueServiceToken}.Mount(router)
	return &App{Handler: router, DB: db, Redis: rdb, Auth: authService}
}

func (a *App) LogStartup(addr string) {
	slog.Info("api listening", "addr", addr)
}
