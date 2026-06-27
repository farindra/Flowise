package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

func initDB(ctx context.Context) error {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("DB_HOST", "127.0.0.1"),
		envOr("DB_PORT", "5432"),
		envOr("DB_USER", "flowise"),
		envOr("DB_PASSWORD", ""),
		envOr("DB_NAME", "flowise"),
	)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse db config: %w", err)
	}
	cfg.MaxConns = 5
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	if err := p.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}
	pool = p
	return nil
}

func migrateDB(ctx context.Context) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS wa_sessions (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name          TEXT NOT NULL,
			chatflow_id   TEXT NOT NULL,
			human_contact TEXT NOT NULL DEFAULT '',
			allow_phones  TEXT NOT NULL DEFAULT '',
			disable_upload BOOLEAN NOT NULL DEFAULT FALSE,
			active        BOOLEAN NOT NULL DEFAULT TRUE,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

type SessionRecord struct {
	ID            string
	Name          string
	ChatflowID    string
	HumanContact  string
	AllowPhones   string
	DisableUpload bool
	Active        bool
	CreatedAt     time.Time
}

func dbListSessions(ctx context.Context) ([]SessionRecord, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, chatflow_id, human_contact, allow_phones, disable_upload, active, created_at
		FROM wa_sessions ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.ChatflowID, &r.HumanContact, &r.AllowPhones, &r.DisableUpload, &r.Active, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dbGetSession(ctx context.Context, id string) (*SessionRecord, error) {
	var r SessionRecord
	err := pool.QueryRow(ctx, `
		SELECT id, name, chatflow_id, human_contact, allow_phones, disable_upload, active, created_at
		FROM wa_sessions WHERE id = $1
	`, id).Scan(&r.ID, &r.Name, &r.ChatflowID, &r.HumanContact, &r.AllowPhones, &r.DisableUpload, &r.Active, &r.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

func dbCreateSession(ctx context.Context, r *SessionRecord) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO wa_sessions (name, chatflow_id, human_contact, allow_phones, disable_upload, active)
		VALUES ($1, $2, $3, $4, $5, TRUE)
		RETURNING id
	`, r.Name, r.ChatflowID, r.HumanContact, r.AllowPhones, r.DisableUpload).Scan(&id)
	return id, err
}

func dbUpdateSession(ctx context.Context, r *SessionRecord) error {
	_, err := pool.Exec(ctx, `
		UPDATE wa_sessions SET name=$1, chatflow_id=$2, human_contact=$3, allow_phones=$4, disable_upload=$5, active=$6
		WHERE id=$7
	`, r.Name, r.ChatflowID, r.HumanContact, r.AllowPhones, r.DisableUpload, r.Active, r.ID)
	return err
}

func dbDeleteSession(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, `DELETE FROM wa_sessions WHERE id=$1`, id)
	return err
}
