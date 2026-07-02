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
		CREATE TABLE IF NOT EXISTS iot_zones (
			id          TEXT        PRIMARY KEY,
			name        TEXT        NOT NULL,
			description TEXT        NOT NULL DEFAULT '',
			thresholds  JSONB       NOT NULL DEFAULT '{}',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS iot_readings (
			id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			zone_id     TEXT        NOT NULL REFERENCES iot_zones(id),
			sensor_type TEXT        NOT NULL,
			value       NUMERIC     NOT NULL,
			unit        TEXT        NOT NULL DEFAULT '',
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS iot_alerts (
			id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			zone_id     TEXT        NOT NULL,
			zone_name   TEXT        NOT NULL DEFAULT '',
			sensor_type TEXT        NOT NULL,
			value       NUMERIC     NOT NULL,
			threshold   NUMERIC     NOT NULL,
			direction   TEXT        NOT NULL DEFAULT 'below',
			message     TEXT        NOT NULL,
			resolved    BOOLEAN     NOT NULL DEFAULT FALSE,
			sent_at     TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_readings_zone_time ON iot_readings(zone_id, recorded_at DESC);
		CREATE INDEX IF NOT EXISTS idx_alerts_open ON iot_alerts(resolved, created_at DESC);

		INSERT INTO iot_zones (id, name, description, thresholds) VALUES
			('zona-a', 'Zona A — Blok Premium', 'Kavling Premium, baris 1–20', '{"soil_humidity":{"min":40},"temperature":{"max":35},"air_humidity":{"min":50}}'),
			('zona-b', 'Zona B — Blok Keluarga', 'Kavling Keluarga, baris 21–60', '{"soil_humidity":{"min":40},"temperature":{"max":35},"air_humidity":{"min":50}}'),
			('zona-c', 'Zona C — Blok Standar', 'Kavling Standar, baris 61–120', '{"soil_humidity":{"min":40},"temperature":{"max":35},"air_humidity":{"min":50}}'),
			('taman-utama', 'Taman Utama', 'Area taman dan jalan utama', '{"soil_humidity":{"min":50},"temperature":{"max":33},"air_humidity":{"min":55}}'),
			('parkir', 'Area Parkir & Gerbang', 'Parkir, gerbang, pos jaga', '{"temperature":{"max":38}}')
		ON CONFLICT (id) DO NOTHING;
	`)
	return err
}

// ── Types ─────────────────────────────────────────────────────────────────────

type Zone struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Thresholds  map[string]any `json:"thresholds"`
	CreatedAt   time.Time      `json:"created_at"`
	// latest readings (joined)
	Latest      map[string]*Reading `json:"latest,omitempty"`
	Status      string              `json:"status"` // ok | warning | alert
	OpenAlerts  int                 `json:"open_alerts"`
}

type Reading struct {
	ID         string    `json:"id"`
	ZoneID     string    `json:"zone_id"`
	SensorType string    `json:"sensor_type"`
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Alert struct {
	ID         string     `json:"id"`
	ZoneID     string     `json:"zone_id"`
	ZoneName   string     `json:"zone_name"`
	SensorType string     `json:"sensor_type"`
	Value      float64    `json:"value"`
	Threshold  float64    `json:"threshold"`
	Direction  string     `json:"direction"`
	Message    string     `json:"message"`
	Resolved   bool       `json:"resolved"`
	SentAt     *time.Time `json:"sent_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// ── DB helpers ────────────────────────────────────────────────────────────────

func dbListZones(ctx context.Context) ([]Zone, error) {
	rows, err := pool.Query(ctx, `SELECT id, name, description, thresholds, created_at FROM iot_zones ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var zones []Zone
	for rows.Next() {
		var z Zone
		if err := rows.Scan(&z.ID, &z.Name, &z.Description, &z.Thresholds, &z.CreatedAt); err != nil {
			return nil, err
		}
		z.Status = "ok"
		zones = append(zones, z)
	}
	return zones, nil
}

func dbLatestReadings(ctx context.Context, zoneID string) (map[string]*Reading, error) {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT ON (sensor_type) id, zone_id, sensor_type, value, unit, recorded_at
		FROM iot_readings WHERE zone_id = $1
		ORDER BY sensor_type, recorded_at DESC`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]*Reading{}
	for rows.Next() {
		r := &Reading{}
		if err := rows.Scan(&r.ID, &r.ZoneID, &r.SensorType, &r.Value, &r.Unit, &r.RecordedAt); err != nil {
			return nil, err
		}
		m[r.SensorType] = r
	}
	return m, nil
}

func dbInsertReading(ctx context.Context, r *Reading) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO iot_readings (zone_id, sensor_type, value, unit, recorded_at) VALUES ($1,$2,$3,$4,$5)`,
		r.ZoneID, r.SensorType, r.Value, r.Unit, r.RecordedAt)
	return err
}

func dbInsertAlert(ctx context.Context, a *Alert) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO iot_alerts (zone_id, zone_name, sensor_type, value, threshold, direction, message)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		a.ZoneID, a.ZoneName, a.SensorType, a.Value, a.Threshold, a.Direction, a.Message).Scan(&id)
	return id, err
}

func dbMarkAlertSent(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, `UPDATE iot_alerts SET sent_at = NOW() WHERE id = $1`, id)
	return err
}

func dbResolveAlerts(ctx context.Context, zoneID, sensorType string) error {
	_, err := pool.Exec(ctx,
		`UPDATE iot_alerts SET resolved = TRUE WHERE zone_id=$1 AND sensor_type=$2 AND resolved=FALSE`,
		zoneID, sensorType)
	return err
}

func dbListAlerts(ctx context.Context, onlyOpen bool) ([]Alert, error) {
	q := `SELECT id, zone_id, zone_name, sensor_type, value, threshold, direction, message, resolved, sent_at, created_at
		  FROM iot_alerts`
	if onlyOpen {
		q += ` WHERE resolved = FALSE`
	}
	q += ` ORDER BY created_at DESC LIMIT 100`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var alerts []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.ZoneID, &a.ZoneName, &a.SensorType, &a.Value, &a.Threshold, &a.Direction, &a.Message, &a.Resolved, &a.SentAt, &a.CreatedAt); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

func dbCountOpenAlerts(ctx context.Context, zoneID string) int {
	var n int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM iot_alerts WHERE zone_id=$1 AND resolved=FALSE`, zoneID).Scan(&n)
	return n
}

func dbReadingsHistory(ctx context.Context, zoneID, sensorType string, hours int) ([]Reading, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, zone_id, sensor_type, value, unit, recorded_at
		FROM iot_readings
		WHERE zone_id=$1 AND sensor_type=$2 AND recorded_at > NOW() - ($3 * INTERVAL '1 hour')
		ORDER BY recorded_at ASC`, zoneID, sensorType, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var readings []Reading
	for rows.Next() {
		var r Reading
		if err := rows.Scan(&r.ID, &r.ZoneID, &r.SensorType, &r.Value, &r.Unit, &r.RecordedAt); err != nil {
			return nil, err
		}
		readings = append(readings, r)
	}
	return readings, nil
}
