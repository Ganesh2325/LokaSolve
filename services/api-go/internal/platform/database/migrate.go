package database

import (
	"context"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"lokasolve/services/api-go/migrations"
)

var versionPattern = regexp.MustCompile(`^[0-9A-Za-z_.-]+$`)

func Migrate(ctx context.Context, conn *pgx.Conn) error {
	if err := execSimple(ctx, conn, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		if !versionPattern.MatchString(version) {
			return fmt.Errorf("unsafe migration version %q", version)
		}
		var applied bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		if err := applyMigration(ctx, conn, version, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", version, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, conn *pgx.Conn, version, sql string) error {
	if err := execSimple(ctx, conn, "BEGIN"); err != nil {
		return err
	}
	if err := execSimple(ctx, conn, sql); err != nil {
		_ = execSimple(ctx, conn, "ROLLBACK")
		return err
	}
	insert := fmt.Sprintf("INSERT INTO schema_migrations (version) VALUES ('%s')", version)
	if err := execSimple(ctx, conn, insert); err != nil {
		_ = execSimple(ctx, conn, "ROLLBACK")
		return err
	}
	return execSimple(ctx, conn, "COMMIT")
}

func execSimple(ctx context.Context, conn *pgx.Conn, sql string) error {
	result := conn.PgConn().Exec(ctx, sql)
	_, err := result.ReadAll()
	closeErr := result.Close()
	if err != nil {
		return err
	}
	return closeErr
}
