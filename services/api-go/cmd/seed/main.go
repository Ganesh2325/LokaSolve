package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/internal/platform/authn"
	"lokasolve/services/api-go/internal/platform/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if os.Getenv("SEED_DEMO_DATA") != "true" {
		slog.Info("SEED_DEMO_DATA is not true; nothing to do")
		return
	}
	if os.Getenv("ENV") == "production" && os.Getenv("SEED_DEMO_CONFIRM") != "yes" {
		slog.Error("refusing to seed demo accounts in production without SEED_DEMO_CONFIRM=yes")
		os.Exit(1)
	}
	password := os.Getenv("DEMO_USER_PASSWORD")
	if len(password) < 10 {
		slog.Error("DEMO_USER_PASSWORD must be at least 10 characters")
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration", "error", err.Error())
		os.Exit(1)
	}
	url := cfg.MigrateDatabaseURL
	if url == "" {
		url = cfg.DatabaseURL
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		slog.Error("database", "error", err.Error())
		os.Exit(1)
	}
	defer conn.Close(ctx)
	hash, err := authn.HashPassword(password)
	if err != nil {
		slog.Error("password", "error", err.Error())
		os.Exit(1)
	}
	adminEmail := os.Getenv("DEMO_ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = "demo.admin@lokasolve.test"
	}
	accounts := []struct {
		email string
		name  string
		role  string
	}{
		{adminEmail, "Demo Platform Admin", "super_admin"},
		{"demo.resident@lokasolve.test", "Demo Resident", "resident"},
		{"demo.provider@lokasolve.test", "Demo Provider", "provider"},
		{"demo.org@lokasolve.test", "Demo Organization Admin", "org_admin"},
	}
	for _, account := range accounts {
		if err := insertDemo(ctx, conn, account.email, account.name, account.role, hash); err != nil {
			slog.Error("seed", "email", account.email, "error", err.Error())
			os.Exit(1)
		}
	}
	fmt.Println("Demo accounts are labeled is_demo=true. The password was read from the environment and was not printed.")
}

func insertDemo(ctx context.Context, conn *pgx.Conn, email, name, role, hash string) error {
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1)`, email).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, is_demo) VALUES ($1, $2, true) RETURNING id::text
	`, email, hash).Scan(&userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_profiles (user_id, display_name) VALUES ($1, $2)
	`, userID, name); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_role_assignments (user_id, role_id, organization_id)
		SELECT $1, id, NULL FROM roles WHERE code = 'resident'
	`, userID); err != nil {
		return err
	}
	if role == "super_admin" || role == "provider" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_role_assignments (user_id, role_id, organization_id)
			SELECT $1, id, NULL FROM roles WHERE code = $2
		`, userID, role); err != nil {
			return err
		}
	}
	if role == "provider" || role == "org_admin" {
		orgName := "Demo Electrical Services"
		orgType := "service_provider"
		if role == "org_admin" {
			orgName = "Demo Ward Office"
			orgType = "municipality"
		}
		var orgID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO organizations (name, org_type, is_demo) VALUES ($1, $2, true) RETURNING id::text
		`, orgName, orgType).Scan(&orgID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization_members (organization_id, user_id, member_role) VALUES ($1, $2, 'owner')
		`, orgID, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_role_assignments (user_id, role_id, organization_id)
			SELECT $1, roles.id, $2::uuid FROM roles WHERE code = 'org_admin'
		`, userID, orgID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
