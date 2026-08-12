package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ── Types ────────────────────────────────────────────────────────────────────

// Broadcast statuses.
const (
	BStatusDraft     = "draft"
	BStatusBuilding  = "building"
	BStatusReady     = "ready"
	BStatusScheduled = "scheduled"
	BStatusRunning   = "running"
	BStatusPaused    = "paused"
	BStatusDone      = "done"
	BStatusCancelled = "cancelled"
)

// Recipient statuses.
const (
	RStatusPending   = "pending"
	RStatusSending   = "sending"
	RStatusSent      = "sent"
	RStatusFailed    = "failed"
	RStatusSkipped   = "skipped"
	RStatusUncertain = "uncertain"
)

// AudienceSpec is the frozen snapshot of what the picker asked for.
type AudienceSpec struct {
	Customers *struct {
		Tiers    []string `json:"tiers"`
		Wilayahs []string `json:"wilayahs"`
	} `json:"customers,omitempty"`
	Upload []UploadRow `json:"upload,omitempty"`
	Chat   *struct {
		ChatflowIDs        []string `json:"chatflow_ids"`
		Days               int      `json:"days"`
		IncludePassthrough bool     `json:"include_passthrough"`
	} `json:"chat,omitempty"`
}

// UploadRow is one parsed row from an uploaded audience file.
type UploadRow struct {
	Name    string            `json:"name"`
	Phone   string            `json:"phone"`
	Wilayah string            `json:"wilayah"`
	Vars    map[string]string `json:"vars,omitempty"`
}

type Broadcast struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	Provider         string          `json:"provider"`
	SenderID         string          `json:"sender_id"`
	MessageText      string          `json:"message_text"`
	MediaPath        string          `json:"media_path"`
	MediaMime        string          `json:"media_mime"`
	MediaMode        string          `json:"media_mode"`
	Audience         AudienceSpec    `json:"audience"`
	Throttle         map[string]any  `json:"throttle"`
	IncludeBlacklist bool            `json:"include_blacklist"`
	AllPhones        bool            `json:"all_phones"`
	DryRun           bool            `json:"dry_run"`
	ScheduledAt      *time.Time      `json:"scheduled_at"`
	StartedAt        *time.Time       `json:"started_at"`
	FinishedAt       *time.Time      `json:"finished_at"`
	Total            int             `json:"total"`
	LastError        string          `json:"last_error"`
	CreatedBy        string          `json:"created_by"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
	Counts           map[string]int  `json:"counts,omitempty"`
}

type Recipient struct {
	ID            int64             `json:"id"`
	BroadcastID   string            `json:"broadcast_id"`
	Phone         string            `json:"phone"`
	Name          string            `json:"name"`
	Wilayah       string            `json:"wilayah"`
	Tier          string            `json:"tier"`
	CustomerID    *string           `json:"customer_id"`
	Source        string            `json:"source"`
	Vars          map[string]string `json:"vars"`
	Status        string            `json:"status"`
	SkipReason    string            `json:"skip_reason"`
	Attempts      int               `json:"attempts"`
	NextAttemptAt time.Time         `json:"next_attempt_at"`
	LastError     string            `json:"last_error"`
	ProviderMsgID string            `json:"provider_msg_id"`
	SentAt        *time.Time        `json:"sent_at"`
	CreatedAt     time.Time         `json:"created_at"`
}

// ── Broadcast CRUD ───────────────────────────────────────────────────────────

const broadcastCols = `id, name, status, provider, sender_id, message_text, media_path, media_mime,
	media_mode, audience, throttle, include_blacklist, all_phones, dry_run, scheduled_at,
	started_at, finished_at, total, last_error, created_by, created_at, updated_at`

func scanBroadcast(row interface{ Scan(...any) error }) (*Broadcast, error) {
	b := &Broadcast{}
	var audRaw, thrRaw []byte
	err := row.Scan(&b.ID, &b.Name, &b.Status, &b.Provider, &b.SenderID, &b.MessageText,
		&b.MediaPath, &b.MediaMime, &b.MediaMode, &audRaw, &thrRaw, &b.IncludeBlacklist,
		&b.AllPhones, &b.DryRun, &b.ScheduledAt, &b.StartedAt, &b.FinishedAt, &b.Total,
		&b.LastError, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(audRaw) > 0 {
		_ = json.Unmarshal(audRaw, &b.Audience)
	}
	if len(thrRaw) > 0 {
		_ = json.Unmarshal(thrRaw, &b.Throttle)
	}
	if b.Throttle == nil {
		b.Throttle = map[string]any{}
	}
	return b, nil
}

func dbListBroadcasts(ctx context.Context, q, status string, page, limit int) ([]Broadcast, int, error) {
	var conds []string
	var args []any
	if q != "" {
		args = append(args, "%"+q+"%")
		conds = append(conds, fmt.Sprintf("name ILIKE $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_broadcasts`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sql := `SELECT ` + broadcastCols + ` FROM crm_broadcasts` + where + ` ORDER BY created_at DESC`
	if limit > 0 {
		if page < 1 {
			page = 1
		}
		sql += fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, (page-1)*limit)
	}
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Broadcast{}
	for rows.Next() {
		b, err := scanBroadcast(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	// Attach computed counters. Derived on read rather than denormalized —
	// stored counters drift whenever a crash lands between send and update.
	for i := range out {
		counts, err := dbRecipientCounts(ctx, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Counts = counts
	}
	return out, total, nil
}

func dbGetBroadcast(ctx context.Context, id string) (*Broadcast, error) {
	row := pool.QueryRow(ctx, `SELECT `+broadcastCols+` FROM crm_broadcasts WHERE id=$1`, id)
	b, err := scanBroadcast(row)
	if err != nil {
		return nil, err
	}
	counts, err := dbRecipientCounts(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	b.Counts = counts
	return b, nil
}

func dbCreateBroadcast(ctx context.Context, b *Broadcast) (string, error) {
	aud, _ := json.Marshal(b.Audience)
	thr, _ := json.Marshal(b.Throttle)
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO crm_broadcasts (name, status, provider, sender_id, message_text, media_path,
			media_mime, media_mode, audience, throttle, include_blacklist, all_phones, dry_run,
			scheduled_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		b.Name, b.Status, b.Provider, b.SenderID, b.MessageText, b.MediaPath, b.MediaMime,
		b.MediaMode, aud, thr, b.IncludeBlacklist, b.AllPhones, b.DryRun, b.ScheduledAt,
		b.CreatedBy).Scan(&id)
	return id, err
}

func dbUpdateBroadcast(ctx context.Context, b *Broadcast) error {
	aud, _ := json.Marshal(b.Audience)
	thr, _ := json.Marshal(b.Throttle)
	_, err := pool.Exec(ctx, `
		UPDATE crm_broadcasts SET name=$1, provider=$2, sender_id=$3, message_text=$4,
			media_path=$5, media_mime=$6, media_mode=$7, audience=$8, throttle=$9,
			include_blacklist=$10, all_phones=$11, dry_run=$12, scheduled_at=$13, updated_at=NOW()
		WHERE id=$14`,
		b.Name, b.Provider, b.SenderID, b.MessageText, b.MediaPath, b.MediaMime, b.MediaMode,
		aud, thr, b.IncludeBlacklist, b.AllPhones, b.DryRun, b.ScheduledAt, b.ID)
	return err
}

func dbSetBroadcastStatus(ctx context.Context, id, status string) error {
	_, err := pool.Exec(ctx,
		`UPDATE crm_broadcasts SET status=$1, updated_at=NOW() WHERE id=$2`, status, id)
	return err
}

func dbSetBroadcastError(ctx context.Context, id, msg string) error {
	_, err := pool.Exec(ctx,
		`UPDATE crm_broadcasts SET last_error=$1, updated_at=NOW() WHERE id=$2`, msg, id)
	return err
}

func dbStartBroadcast(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, `
		UPDATE crm_broadcasts
		SET status='running', started_at=COALESCE(started_at, NOW()), last_error='', updated_at=NOW()
		WHERE id=$1`, id)
	return err
}

func dbFinishBroadcast(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, `
		UPDATE crm_broadcasts SET status='done', finished_at=NOW(), updated_at=NOW() WHERE id=$1`, id)
	return err
}

func dbDeleteBroadcast(ctx context.Context, id string) error {
	_, err := pool.Exec(ctx, `DELETE FROM crm_broadcasts WHERE id=$1`, id)
	return err
}

// dbBroadcastStatus reads just the status — used by the worker's per-message
// re-check, so pause/cancel takes effect within one message.
func dbBroadcastStatus(ctx context.Context, id string) (string, error) {
	var s string
	err := pool.QueryRow(ctx, `SELECT status FROM crm_broadcasts WHERE id=$1`, id).Scan(&s)
	return s, err
}

// ── Recipients ───────────────────────────────────────────────────────────────

func dbRecipientCounts(ctx context.Context, broadcastID string) (map[string]int, error) {
	rows, err := pool.Query(ctx,
		`SELECT status, COUNT(*) FROM crm_broadcast_recipients WHERE broadcast_id=$1 GROUP BY status`,
		broadcastID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// dbReplaceRecipients swaps in a freshly built audience atomically, so a failed
// build can never leave a half-populated list attached to a campaign.
func dbReplaceRecipients(ctx context.Context, broadcastID string, rs []*Recipient) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`DELETE FROM crm_broadcast_recipients WHERE broadcast_id=$1`, broadcastID); err != nil {
		return err
	}

	for _, r := range rs {
		vars, _ := json.Marshal(r.Vars)
		if _, err := tx.Exec(ctx, `
			INSERT INTO crm_broadcast_recipients
				(broadcast_id, phone, name, wilayah, tier, customer_id, source, vars, status, skip_reason)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (broadcast_id, phone) DO NOTHING`,
			broadcastID, r.Phone, r.Name, r.Wilayah, r.Tier, r.CustomerID, r.Source, vars,
			r.Status, r.SkipReason); err != nil {
			return err
		}
	}

	// `total` counts only rows that will actually be attempted.
	if _, err := tx.Exec(ctx, `
		UPDATE crm_broadcasts SET total = (
			SELECT COUNT(*) FROM crm_broadcast_recipients
			WHERE broadcast_id=$1 AND status='pending'
		), updated_at=NOW() WHERE id=$1`, broadcastID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// dbClaimNextRecipient atomically reserves one pending recipient. attempts is
// incremented at claim time, before the send, so a crash mid-send still burns
// the attempt and a crash-loop cannot resend the same row forever.
// FOR UPDATE SKIP LOCKED means even two go-crm processes cannot double-claim.
func dbClaimNextRecipient(ctx context.Context, broadcastID string) (*Recipient, error) {
	row := pool.QueryRow(ctx, `
		UPDATE crm_broadcast_recipients r
		SET status='sending', attempts=attempts+1, claimed_at=NOW()
		WHERE r.id = (
			SELECT id FROM crm_broadcast_recipients
			WHERE broadcast_id=$1 AND status='pending' AND next_attempt_at <= NOW()
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING r.id, r.phone, r.name, r.wilayah, r.tier, r.source, r.vars, r.attempts`,
		broadcastID)

	r := &Recipient{BroadcastID: broadcastID}
	var varsRaw []byte
	err := row.Scan(&r.ID, &r.Phone, &r.Name, &r.Wilayah, &r.Tier, &r.Source, &varsRaw, &r.Attempts)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(varsRaw) > 0 {
		_ = json.Unmarshal(varsRaw, &r.Vars)
	}
	if r.Vars == nil {
		r.Vars = map[string]string{}
	}
	return r, nil
}

func dbMarkRecipientSent(ctx context.Context, id int64, msgID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE crm_broadcast_recipients
		SET status='sent', sent_at=NOW(), provider_msg_id=$1, last_error=''
		WHERE id=$2`, msgID, id)
	return err
}

func dbMarkRecipientFailed(ctx context.Context, id int64, errMsg string) error {
	_, err := pool.Exec(ctx,
		`UPDATE crm_broadcast_recipients SET status='failed', last_error=$1 WHERE id=$2`, errMsg, id)
	return err
}

func dbMarkRecipientRetry(ctx context.Context, id int64, errMsg string, retryAt time.Time) error {
	_, err := pool.Exec(ctx, `
		UPDATE crm_broadcast_recipients
		SET status='pending', last_error=$1, next_attempt_at=$2 WHERE id=$3`, errMsg, retryAt, id)
	return err
}

func dbMarkRecipientSkipped(ctx context.Context, id int64, reason string) error {
	_, err := pool.Exec(ctx,
		`UPDATE crm_broadcast_recipients SET status='skipped', skip_reason=$1 WHERE id=$2`, reason, id)
	return err
}

// dbPendingRecipientCount reports how much work is left, including rows waiting
// on a retry backoff.
func dbPendingRecipientCount(ctx context.Context, broadcastID string) (int, error) {
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_broadcast_recipients
		WHERE broadcast_id=$1 AND status IN ('pending','sending')`, broadcastID).Scan(&n)
	return n, err
}

func dbListRecipients(ctx context.Context, broadcastID, status, q string, page, limit int) ([]Recipient, int, error) {
	conds := []string{"broadcast_id = $1"}
	args := []any{broadcastID}
	if status != "" {
		args = append(args, status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if q != "" {
		args = append(args, "%"+q+"%")
		conds = append(conds, fmt.Sprintf("(phone ILIKE $%d OR name ILIKE $%d)", len(args), len(args)))
	}
	where := " WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crm_broadcast_recipients`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sql := `SELECT id, phone, name, wilayah, tier, source, status, skip_reason, attempts,
		last_error, provider_msg_id, sent_at, created_at
		FROM crm_broadcast_recipients` + where + ` ORDER BY id`
	if limit > 0 {
		if page < 1 {
			page = 1
		}
		sql += fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, (page-1)*limit)
	}
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := []Recipient{}
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.ID, &r.Phone, &r.Name, &r.Wilayah, &r.Tier, &r.Source, &r.Status,
			&r.SkipReason, &r.Attempts, &r.LastError, &r.ProviderMsgID, &r.SentAt, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// dbRetryRecipients moves failed/uncertain rows back to pending. `uncertain` is
// only ever retried through here — never automatically — because the send
// outcome for those rows is genuinely unknown.
func dbRetryRecipients(ctx context.Context, broadcastID, scope string) (int64, error) {
	if scope != RStatusFailed && scope != RStatusUncertain {
		return 0, fmt.Errorf("scope harus 'failed' atau 'uncertain'")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE crm_broadcast_recipients
		SET status='pending', attempts=0, next_attempt_at=NOW(), last_error=''
		WHERE broadcast_id=$1 AND status=$2`, broadcastID, scope)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func dbCancelPendingRecipients(ctx context.Context, broadcastID string) error {
	_, err := pool.Exec(ctx, `
		UPDATE crm_broadcast_recipients SET status='skipped', skip_reason='cancelled'
		WHERE broadcast_id=$1 AND status='pending'`, broadcastID)
	return err
}

// dbReconcileSendingRecipients runs once at startup. A row left in 'sending'
// means the process died between the provider call and the status write — the
// message may or may not have arrived. Marking these 'uncertain' (never
// auto-retried) makes a human decide, rather than silently duplicating or
// silently dropping.
func dbReconcileSendingRecipients(ctx context.Context) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE crm_broadcast_recipients
		SET status='uncertain',
		    last_error='service restart saat pengiriman — status tidak diketahui'
		WHERE status='sending'`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ── Send-time guards ─────────────────────────────────────────────────────────

func dbIsOptedOut(ctx context.Context, phone string) (bool, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM crm_broadcast_optouts WHERE phone=$1`, phone).Scan(&n)
	return n > 0, err
}

// dbRecentlyContacted reports whether the number received any broadcast within
// the cooldown window, across all campaigns.
func dbRecentlyContacted(ctx context.Context, phone string, days int, exceptBroadcast string) (bool, error) {
	if days <= 0 {
		return false, nil
	}
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_broadcast_recipients
		WHERE phone=$1 AND status='sent' AND broadcast_id <> $2
		  AND sent_at >= NOW() - ($3 || ' days')::interval`,
		phone, exceptBroadcast, fmt.Sprint(days)).Scan(&n)
	return n > 0, err
}

// dbSentTodayBySender counts messages already sent today by the given sender,
// across every campaign — a per-campaign cap would not compose.
func dbSentTodayBySender(ctx context.Context, senderID string, loc *time.Location) (int, error) {
	now := time.Now().In(loc)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM crm_broadcast_recipients r
		JOIN crm_broadcasts b ON b.id = r.broadcast_id
		WHERE b.sender_id=$1 AND r.status='sent' AND r.sent_at >= $2`,
		senderID, startOfDay).Scan(&n)
	return n, err
}

func dbAddOptOut(ctx context.Context, phone, reason, note string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO crm_broadcast_optouts (phone, reason, note) VALUES ($1,$2,$3)
		ON CONFLICT (phone) DO UPDATE SET reason=EXCLUDED.reason, note=EXCLUDED.note`,
		phone, reason, note)
	return err
}

func dbRemoveOptOut(ctx context.Context, phone string) error {
	_, err := pool.Exec(ctx, `DELETE FROM crm_broadcast_optouts WHERE phone=$1`, phone)
	return err
}

func dbListOptOuts(ctx context.Context) ([]map[string]any, error) {
	rows, err := pool.Query(ctx,
		`SELECT phone, reason, note, created_at FROM crm_broadcast_optouts ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var phone, reason, note string
		var created time.Time
		if err := rows.Scan(&phone, &reason, &note, &created); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"phone": phone, "reason": reason, "note": note, "created_at": created,
		})
	}
	return out, rows.Err()
}

// dbOptedOutSet returns which of the given phones are opted out, in one query.
func dbOptedOutSet(ctx context.Context, phones []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(phones) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx,
		`SELECT phone FROM crm_broadcast_optouts WHERE phone = ANY($1)`, phones)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

// dbRecentlyContactedSet batches the cooldown lookup for audience building.
func dbRecentlyContactedSet(ctx context.Context, phones []string, days int) (map[string]bool, error) {
	out := map[string]bool{}
	if len(phones) == 0 || days <= 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT phone FROM crm_broadcast_recipients
		WHERE phone = ANY($1) AND status='sent'
		  AND sent_at >= NOW() - ($2 || ' days')::interval`, phones, fmt.Sprint(days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

// ── Audience sources ─────────────────────────────────────────────────────────

// dbCustomersForBroadcast selects customers by tier and wilayah. Wilayah is
// matched case-folded because the stored values are inconsistent (MEDAN/Medan,
// jakarta/Jakarta) and an exact match would silently under-target.
//
// Note it deliberately does NOT filter out blacklisted customers: that
// exclusion belongs to the audience pipeline so it can be counted and shown.
// 62 of 83 customers are blacklisted, so dropping them here would make an
// all-tiers campaign look inexplicably tiny with nothing to explain it.
func dbCustomersForBroadcast(ctx context.Context, tiers, wilayahs []string) ([]Customer, error) {
	conds := []string{}
	args := []any{}
	if len(tiers) > 0 {
		args = append(args, tiers)
		conds = append(conds, fmt.Sprintf("tier = ANY($%d)", len(args)))
	}
	if len(wilayahs) > 0 {
		lowered := make([]string, len(wilayahs))
		for i, w := range wilayahs {
			lowered[i] = strings.ToLower(strings.TrimSpace(w))
		}
		args = append(args, lowered)
		conds = append(conds, fmt.Sprintf("lower(wilayah) = ANY($%d)", len(args)))
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	rows, err := pool.Query(ctx,
		`SELECT `+customerScanCols+` FROM crm_customers`+where+` ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Customer{}
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// dbWilayahCounts feeds the audience picker. Grouped case-folded so the
// MEDAN/Medan split shows as one option with the correct total.
func dbWilayahCounts(ctx context.Context) ([]map[string]any, error) {
	rows, err := pool.Query(ctx, `
		SELECT initcap(lower(wilayah)) AS w, lower(wilayah) AS key, COUNT(*)
		FROM crm_customers WHERE wilayah <> ''
		GROUP BY 1, 2 ORDER BY 3 DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var label, key string
		var n int
		if err := rows.Scan(&label, &key, &n); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"label": label, "value": key, "count": n})
	}
	return out, rows.Err()
}

// dbBlacklistedPhoneSet reports which of the given phones belong to a
// blacklisted customer. Needed because upload and chat-sourced audiences carry
// no tier of their own, and 62 of 83 customers are blacklisted — filtering only
// the customers source would let most of them back in through another door.
func dbBlacklistedPhoneSet(ctx context.Context, phones []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(phones) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT p FROM crm_customers c, unnest(c.phone) p
		WHERE c.tier='blacklist' AND p = ANY($1)`, phones)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

// dbCustomerByPhoneSet enriches upload/chat-sourced recipients with the curated
// name, wilayah and tier we already hold.
func dbCustomerByPhoneSet(ctx context.Context, phones []string) (map[string]*Customer, error) {
	out := map[string]*Customer{}
	if len(phones) == 0 {
		return out, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT p, c.id, c.name, c.wilayah, c.tier
		FROM crm_customers c, unnest(c.phone) p WHERE p = ANY($1)`, phones)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		c := &Customer{}
		if err := rows.Scan(&p, &c.ID, &c.Name, &c.Wilayah, &c.Tier); err != nil {
			return nil, err
		}
		out[p] = c
	}
	return out, rows.Err()
}

// dbChatIDsForChatflows pulls distinct conversation identifiers from Flowise's
// chat_message table (go-crm shares the same database). These are WhatsApp LIDs
// rather than phone numbers, so they still need resolving via go-wa.
func dbChatIDsForChatflows(ctx context.Context, chatflowIDs []string, days int) ([]string, error) {
	if len(chatflowIDs) == 0 {
		return nil, nil
	}
	if days <= 0 {
		days = 90
	}
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT "chatId" FROM chat_message
		WHERE chatflowid = ANY($1)
		  AND "createdDate" >= NOW() - ($2 || ' days')::interval
		  AND "chatId" ~ '^[0-9]{8,20}$'`, chatflowIDs, fmt.Sprint(days))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// dbWASessionsForChatflows finds which WhatsApp sessions serve the given
// chatflows. Required for LID resolution: the LID map is per-session SQLite, so
// an ID from one session will not resolve against another.
func dbWASessionsForChatflows(ctx context.Context, chatflowIDs []string) ([]string, error) {
	if len(chatflowIDs) == 0 {
		return nil, nil
	}
	rows, err := pool.Query(ctx,
		`SELECT id FROM wa_sessions WHERE chatflow_id = ANY($1) AND active = TRUE`, chatflowIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// dbWASessionChatflows lists sessions with their chatflow, for the picker.
func dbWASessionChatflows(ctx context.Context) ([]map[string]any, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, name, chatflow_id FROM wa_sessions WHERE active = TRUE ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, chatflow string
		if err := rows.Scan(&id, &name, &chatflow); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "chatflow_id": chatflow})
	}
	return out, rows.Err()
}

// dbDueBroadcasts returns campaigns the worker should act on.
func dbDueBroadcasts(ctx context.Context) ([]Broadcast, error) {
	rows, err := pool.Query(ctx, `
		SELECT `+broadcastCols+` FROM crm_broadcasts
		WHERE status='running'
		   OR (status='scheduled' AND scheduled_at IS NOT NULL AND scheduled_at <= NOW())
		ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Broadcast{}
	for rows.Next() {
		b, err := scanBroadcast(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}
