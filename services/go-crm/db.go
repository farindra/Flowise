package main

import (
	"context"
	"fmt"
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
	cfg.MaxConns = 10
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
		CREATE TABLE IF NOT EXISTS crm_leads (
			id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			name            TEXT        NOT NULL DEFAULT '',
			phone           TEXT        NOT NULL DEFAULT '',
			email           TEXT        NOT NULL DEFAULT '',
			source          TEXT        NOT NULL DEFAULT 'wa',
			stage           TEXT        NOT NULL DEFAULT 'new',
			score           INT         NOT NULL DEFAULT 0,
			urgency         TEXT        NOT NULL DEFAULT 'low',
			budget_range    TEXT        NOT NULL DEFAULT '',
			interest        TEXT        NOT NULL DEFAULT '',
			notes           TEXT        NOT NULL DEFAULT '',
			assigned_to     TEXT        NOT NULL DEFAULT '',
			last_contact_at TIMESTAMPTZ,
			follow_up_at    TIMESTAMPTZ,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS crm_kavlings (
			id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			code            TEXT        NOT NULL UNIQUE,
			type            TEXT        NOT NULL,
			zone            TEXT        NOT NULL DEFAULT '',
			area_m2         NUMERIC     NOT NULL DEFAULT 0,
			holes           INT         NOT NULL DEFAULT 1,
			price           BIGINT      NOT NULL DEFAULT 0,
			status          TEXT        NOT NULL DEFAULT 'available',
			buyer_id        UUID,
			notes           TEXT        NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS crm_buyers (
			id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			name            TEXT        NOT NULL,
			phone           TEXT        NOT NULL,
			email           TEXT        NOT NULL DEFAULT '',
			id_number       TEXT        NOT NULL DEFAULT '',
			address         TEXT        NOT NULL DEFAULT '',
			kavling_id      UUID,
			payment_type    TEXT        NOT NULL DEFAULT 'cash',
			total_price     BIGINT      NOT NULL DEFAULT 0,
			paid_amount     BIGINT      NOT NULL DEFAULT 0,
			next_due_date   TIMESTAMPTZ,
			deceased_name   TEXT        NOT NULL DEFAULT '',
			burial_date     TIMESTAMPTZ,
			anniversary_date TIMESTAMPTZ,
			notes           TEXT        NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS crm_notifications (
			id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			buyer_id        UUID,
			lead_id         UUID,
			channel         TEXT        NOT NULL DEFAULT 'wa',
			type            TEXT        NOT NULL,
			recipient_phone TEXT        NOT NULL,
			message         TEXT        NOT NULL,
			scheduled_at    TIMESTAMPTZ NOT NULL,
			sent_at         TIMESTAMPTZ,
			status          TEXT        NOT NULL DEFAULT 'pending',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_leads_stage ON crm_leads(stage);
		CREATE INDEX IF NOT EXISTS idx_leads_phone ON crm_leads(phone);
		CREATE INDEX IF NOT EXISTS idx_kavlings_status ON crm_kavlings(status);
		CREATE INDEX IF NOT EXISTS idx_notifs_pending ON crm_notifications(status, scheduled_at)
			WHERE status = 'pending';
	`)
	return err
}

// ── Lead types ───────────────────────────────────────────────────────────────

type Lead struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Phone         string     `json:"phone"`
	Email         string     `json:"email"`
	Source        string     `json:"source"`
	Stage         string     `json:"stage"`
	Score         int        `json:"score"`
	Urgency       string     `json:"urgency"`
	BudgetRange   string     `json:"budget_range"`
	Interest      string     `json:"interest"`
	Notes         string     `json:"notes"`
	AssignedTo    string     `json:"assigned_to"`
	LastContactAt *time.Time `json:"last_contact_at"`
	FollowUpAt    *time.Time `json:"follow_up_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func dbListLeads(ctx context.Context, stage string) ([]Lead, error) {
	query := `SELECT id, name, phone, email, source, stage, score, urgency,
		budget_range, interest, notes, assigned_to,
		last_contact_at, follow_up_at, created_at, updated_at
		FROM crm_leads`
	args := []any{}
	if stage != "" {
		query += " WHERE stage = $1"
		args = append(args, stage)
	}
	query += " ORDER BY created_at DESC LIMIT 100"
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Lead
	for rows.Next() {
		var r Lead
		if err := rows.Scan(&r.ID, &r.Name, &r.Phone, &r.Email, &r.Source,
			&r.Stage, &r.Score, &r.Urgency, &r.BudgetRange, &r.Interest,
			&r.Notes, &r.AssignedTo, &r.LastContactAt, &r.FollowUpAt,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dbGetLead(ctx context.Context, id string) (*Lead, error) {
	r := &Lead{}
	err := pool.QueryRow(ctx, `SELECT id, name, phone, email, source, stage, score, urgency,
		budget_range, interest, notes, assigned_to,
		last_contact_at, follow_up_at, created_at, updated_at
		FROM crm_leads WHERE id = $1`, id).Scan(
		&r.ID, &r.Name, &r.Phone, &r.Email, &r.Source, &r.Stage, &r.Score,
		&r.Urgency, &r.BudgetRange, &r.Interest, &r.Notes, &r.AssignedTo,
		&r.LastContactAt, &r.FollowUpAt, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func dbCreateLead(ctx context.Context, r *Lead) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `INSERT INTO crm_leads
		(name, phone, email, source, stage, score, urgency, budget_range, interest, notes, assigned_to)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		r.Name, r.Phone, r.Email, r.Source, r.Stage, r.Score, r.Urgency,
		r.BudgetRange, r.Interest, r.Notes, r.AssignedTo).Scan(&id)
	return id, err
}

func dbUpdateLead(ctx context.Context, r *Lead) error {
	_, err := pool.Exec(ctx, `UPDATE crm_leads SET
		name=$1, stage=$2, score=$3, urgency=$4, budget_range=$5,
		interest=$6, notes=$7, assigned_to=$8,
		last_contact_at=$9, follow_up_at=$10, updated_at=NOW()
		WHERE id=$11`,
		r.Name, r.Stage, r.Score, r.Urgency, r.BudgetRange,
		r.Interest, r.Notes, r.AssignedTo, r.LastContactAt, r.FollowUpAt, r.ID)
	return err
}

// ── Kavling types ─────────────────────────────────────────────────────────────

type Kavling struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	Type      string    `json:"type"`
	Zone      string    `json:"zone"`
	AreaM2    float64   `json:"area_m2"`
	Holes     int       `json:"holes"`
	Price     int64     `json:"price"`
	Status    string    `json:"status"`
	BuyerID   *string   `json:"buyer_id"`
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func dbListKavlings(ctx context.Context, status string) ([]Kavling, error) {
	query := `SELECT id, code, type, zone, area_m2, holes, price, status, buyer_id, notes, created_at, updated_at FROM crm_kavlings`
	args := []any{}
	if status != "" {
		query += " WHERE status = $1"
		args = append(args, status)
	}
	query += " ORDER BY code"
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Kavling
	for rows.Next() {
		var r Kavling
		if err := rows.Scan(&r.ID, &r.Code, &r.Type, &r.Zone, &r.AreaM2,
			&r.Holes, &r.Price, &r.Status, &r.BuyerID, &r.Notes,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dbGetKavling(ctx context.Context, id string) (*Kavling, error) {
	r := &Kavling{}
	err := pool.QueryRow(ctx, `SELECT id, code, type, zone, area_m2, holes, price, status, buyer_id, notes, created_at, updated_at FROM crm_kavlings WHERE id = $1 OR code = $1`, id).
		Scan(&r.ID, &r.Code, &r.Type, &r.Zone, &r.AreaM2, &r.Holes, &r.Price, &r.Status, &r.BuyerID, &r.Notes, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func dbUpdateKavling(ctx context.Context, r *Kavling) error {
	_, err := pool.Exec(ctx, `UPDATE crm_kavlings SET status=$1, buyer_id=$2, notes=$3, updated_at=NOW() WHERE id=$4`,
		r.Status, r.BuyerID, r.Notes, r.ID)
	return err
}

// ── Buyer types ───────────────────────────────────────────────────────────────

type Buyer struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Phone           string     `json:"phone"`
	Email           string     `json:"email"`
	IDNumber        string     `json:"id_number"`
	Address         string     `json:"address"`
	KavlingID       *string    `json:"kavling_id"`
	PaymentType     string     `json:"payment_type"`
	TotalPrice      int64      `json:"total_price"`
	PaidAmount      int64      `json:"paid_amount"`
	NextDueDate     *time.Time `json:"next_due_date"`
	DeceasedName    string     `json:"deceased_name"`
	BurialDate      *time.Time `json:"burial_date"`
	AnniversaryDate *time.Time `json:"anniversary_date"`
	Notes           string     `json:"notes"`
	CreatedAt       time.Time  `json:"created_at"`
}

func dbListBuyers(ctx context.Context) ([]Buyer, error) {
	rows, err := pool.Query(ctx, `SELECT id, name, phone, email, id_number, address,
		kavling_id, payment_type, total_price, paid_amount, next_due_date,
		deceased_name, burial_date, anniversary_date, notes, created_at
		FROM crm_buyers ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Buyer
	for rows.Next() {
		var r Buyer
		if err := rows.Scan(&r.ID, &r.Name, &r.Phone, &r.Email, &r.IDNumber,
			&r.Address, &r.KavlingID, &r.PaymentType, &r.TotalPrice,
			&r.PaidAmount, &r.NextDueDate, &r.DeceasedName, &r.BurialDate,
			&r.AnniversaryDate, &r.Notes, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dbGetBuyer(ctx context.Context, id string) (*Buyer, error) {
	r := &Buyer{}
	err := pool.QueryRow(ctx, `SELECT id, name, phone, email, id_number, address,
		kavling_id, payment_type, total_price, paid_amount, next_due_date,
		deceased_name, burial_date, anniversary_date, notes, created_at
		FROM crm_buyers WHERE id = $1`, id).
		Scan(&r.ID, &r.Name, &r.Phone, &r.Email, &r.IDNumber,
			&r.Address, &r.KavlingID, &r.PaymentType, &r.TotalPrice,
			&r.PaidAmount, &r.NextDueDate, &r.DeceasedName, &r.BurialDate,
			&r.AnniversaryDate, &r.Notes, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func dbCreateBuyer(ctx context.Context, r *Buyer) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `INSERT INTO crm_buyers
		(name, phone, email, id_number, address, kavling_id, payment_type,
		total_price, paid_amount, next_due_date, deceased_name, burial_date, anniversary_date, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
		r.Name, r.Phone, r.Email, r.IDNumber, r.Address, r.KavlingID,
		r.PaymentType, r.TotalPrice, r.PaidAmount, r.NextDueDate,
		r.DeceasedName, r.BurialDate, r.AnniversaryDate, r.Notes).Scan(&id)
	return id, err
}

// ── Notification types ────────────────────────────────────────────────────────

type Notification struct {
	ID             string     `json:"id"`
	BuyerID        *string    `json:"buyer_id"`
	LeadID         *string    `json:"lead_id"`
	Channel        string     `json:"channel"`
	Type           string     `json:"type"`
	RecipientPhone string     `json:"recipient_phone"`
	Message        string     `json:"message"`
	ScheduledAt    time.Time  `json:"scheduled_at"`
	SentAt         *time.Time `json:"sent_at"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
}

func dbListPendingNotifs(ctx context.Context) ([]Notification, error) {
	rows, err := pool.Query(ctx, `SELECT id, buyer_id, lead_id, channel, type,
		recipient_phone, message, scheduled_at, sent_at, status, created_at
		FROM crm_notifications
		WHERE status = 'pending' AND scheduled_at <= NOW()
		ORDER BY scheduled_at LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var r Notification
		if err := rows.Scan(&r.ID, &r.BuyerID, &r.LeadID, &r.Channel, &r.Type,
			&r.RecipientPhone, &r.Message, &r.ScheduledAt, &r.SentAt,
			&r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func dbCreateNotif(ctx context.Context, r *Notification) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `INSERT INTO crm_notifications
		(buyer_id, lead_id, channel, type, recipient_phone, message, scheduled_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		r.BuyerID, r.LeadID, r.Channel, r.Type,
		r.RecipientPhone, r.Message, r.ScheduledAt).Scan(&id)
	return id, err
}

func dbMarkNotifSent(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, `UPDATE crm_notifications SET status='sent', sent_at=NOW() WHERE id=$1`, id)
	return err
}
