package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func Connect(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		registerGoogleUUID(conn.TypeMap())
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	db.Pool.Close()
}

func (db *DB) Ping(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

func (db *DB) WithFlags(ctx context.Context, userID *uuid.UUID, flags []string, fn func(pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if userID != nil {
		if err := SetLocal(ctx, tx, "app.user_id", userID.String()); err != nil {
			return err
		}
	}
	for _, flag := range flags {
		if err := SetLocal(ctx, tx, flag, "on"); err != nil {
			return err
		}
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func SetLocal(ctx context.Context, tx pgx.Tx, key, value string) error {
	_, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", key, value)
	return err
}

func IsPrivileged(ctx context.Context, pool *pgxpool.Pool) (bool, string, error) {
	var name string
	var privileged bool
	err := pool.QueryRow(ctx, `
		SELECT current_user, rolsuper OR rolbypassrls
		FROM pg_roles
		WHERE rolname = current_user
	`).Scan(&name, &privileged)
	return privileged, name, err
}
