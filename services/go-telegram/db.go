package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var pool *pgxpool.Pool

func initDB(ctx context.Context) error {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("DB_HOST", "127.0.0.1"),
		envOr("DB_PORT", "5432"),
		envOr("DB_USER", "flowise"),
		os.Getenv("DB_PASSWORD"),
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
		CREATE TABLE IF NOT EXISTS tg_bots (
			id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			name          TEXT        NOT NULL,
			token         TEXT        NOT NULL UNIQUE,
			chatflow_id   TEXT        NOT NULL,
			allow_user_ids TEXT       NOT NULL DEFAULT '',
			disable_upload BOOLEAN    NOT NULL DEFAULT FALSE,
			human_contact TEXT        NOT NULL DEFAULT '',
			active        BOOLEAN     NOT NULL DEFAULT TRUE,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

// BotRecord is a row from tg_bots.
type BotRecord struct {
	ID            string
	Name          string
	Token         string
	ChatflowID    string
	AllowUserIDs  string
	DisableUpload bool
	HumanContact  string
	Active        bool
	CreatedAt     time.Time
}

func dbListBots(ctx context.Context) ([]BotRecord, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, name, token, chatflow_id, allow_user_ids,
		       disable_upload, human_contact, active, created_at
		FROM tg_bots ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BotRecord
	for rows.Next() {
		var r BotRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Token, &r.ChatflowID,
			&r.AllowUserIDs, &r.DisableUpload, &r.HumanContact,
			&r.Active, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dbGetBot(ctx context.Context, id string) (*BotRecord, error) {
	r := &BotRecord{}
	err := pool.QueryRow(ctx, `
		SELECT id, name, token, chatflow_id, allow_user_ids,
		       disable_upload, human_contact, active, created_at
		FROM tg_bots WHERE id = $1
	`, id).Scan(&r.ID, &r.Name, &r.Token, &r.ChatflowID,
		&r.AllowUserIDs, &r.DisableUpload, &r.HumanContact,
		&r.Active, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func dbCreateBot(ctx context.Context, r *BotRecord) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO tg_bots (name, token, chatflow_id, allow_user_ids, disable_upload, human_contact, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, r.Name, r.Token, r.ChatflowID, r.AllowUserIDs, r.DisableUpload, r.HumanContact, r.Active).Scan(&id)
	return id, err
}

func dbUpdateBot(ctx context.Context, r *BotRecord) error {
	_, err := pool.Exec(ctx, `
		UPDATE tg_bots SET name=$1, chatflow_id=$2, allow_user_ids=$3,
		       disable_upload=$4, human_contact=$5, active=$6, updated_at=NOW()
		WHERE id=$7
	`, r.Name, r.ChatflowID, r.AllowUserIDs, r.DisableUpload,
		r.HumanContact, r.Active, r.ID)
	return err
}

func dbDeleteBot(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, `DELETE FROM tg_bots WHERE id = $1`, id)
	return err
}

func dbTokenExists(ctx context.Context, token string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tg_bots WHERE token=$1)`, token).Scan(&exists)
	return exists, err
}

// seedFromEnv inserts bots defined via legacy env vars if they don't exist yet.
func seedFromEnv(ctx context.Context) {
	seeds := []struct {
		name          string
		tokenKey      string
		chatflowKey   string
		allowUserIDs  string
		disableUpload bool
	}{
		{"Customer Bot", "TELEGRAM_BOT_TOKEN", "FLOWISE_CHATFLOW_CUSTOMER", "", false},
		{"Owner Bot", "OWNER_BOT_TOKEN", "FLOWISE_CHATFLOW_OWNER", os.Getenv("OWNER_TELEGRAM_IDS"), false},
		{"Salesman Bot", "SALESMAN_BOT_TOKEN", "FLOWISE_CHATFLOW_SALESMAN", "", true},
	}

	for _, s := range seeds {
		token := os.Getenv(s.tokenKey)
		chatflowID := os.Getenv(s.chatflowKey)
		if token == "" || chatflowID == "" {
			continue
		}
		exists, err := dbTokenExists(ctx, token)
		if err != nil || exists {
			continue
		}
		id, err := dbCreateBot(ctx, &BotRecord{
			Name:          s.name,
			Token:         token,
			ChatflowID:    chatflowID,
			AllowUserIDs:  s.allowUserIDs,
			DisableUpload: s.disableUpload,
			HumanContact:  os.Getenv("HUMAN_TELEGRAM"),
			Active:        true,
		})
		if err != nil {
			log.Printf("[seed] failed to insert %s: %v", s.name, err)
		} else {
			log.Printf("[seed] inserted %s → %s", s.name, id)
		}
	}
}
